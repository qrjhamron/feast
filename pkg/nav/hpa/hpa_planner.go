package hpa

import (
	"container/heap"
	"context"
	"sync"
	"time"

	"github.com/qrjhamron/feast/pkg/nav/goal"
	"github.com/qrjhamron/feast/pkg/nav/move"
	"github.com/qrjhamron/feast/pkg/nav/planner"
	"github.com/qrjhamron/feast/pkg/world"
)

type HPAPlanResult struct {
	Status             planner.PlanStatus
	AbstractPath       []AbstractNode
	Refiner            *LazyRefiner
	Err                error
	StartConnected     bool
	GoalConnected      bool
	StartComponentSize int
	GoalComponentSize  int
	SameComponent      bool
}

type HPAPlanner struct {
	graph        *AbstractGraph
	clusters     *ClusterManager
	localPlanner *planner.Planner
	world        *world.World
}

func NewHPAPlanner(w *world.World, g *AbstractGraph, c *ClusterManager) *HPAPlanner {
	return &HPAPlanner{
		graph:        g,
		clusters:     c,
		localPlanner: planner.NewPlanner(),
		world:        w,
	}
}

type astarNode struct {
	id      uint32
	g, h, f float64
	parent  *astarNode
	index   int
}

type astarQueue []*astarNode

func (pq astarQueue) Len() int           { return len(pq) }
func (pq astarQueue) Less(i, j int) bool { return pq[i].f < pq[j].f }
func (pq astarQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}
func (pq *astarQueue) Push(x interface{}) {
	n := len(*pq)
	item := x.(*astarNode)
	item.index = n
	*pq = append(*pq, item)
}
func (pq *astarQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	item := old[n-1]
	item.index = -1
	*pq = old[0 : n-1]
	return item
}

func manhattanAbstract(a, b [3]int) float64 {
	return float64(abs(a[0]-b[0]) + abs(a[1]-b[1]) + abs(a[2]-b[2]))
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func (p *HPAPlanner) Plan(start, end [3]int, goalDef goal.Goal) HPAPlanResult {
	return p.PlanWithContext(context.Background(), start, end, goalDef)
}

func (p *HPAPlanner) PlanWithContext(ctx context.Context, start, end [3]int, goalDef goal.Goal) HPAPlanResult {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return HPAPlanResult{Status: planner.PlanNoPath, Err: err}
	}
	startClusterCoord := ClusterCoord{X: blockToChunkCoord(start[0]), Z: blockToChunkCoord(start[2])}
	goalClusterCoord := ClusterCoord{X: blockToChunkCoord(end[0]), Z: blockToChunkCoord(end[2])}

	startNodeID := p.graph.AddTempNode(start, startClusterCoord)
	goalNodeID := p.graph.AddTempNode(end, goalClusterCoord)

	// cleanup temp nodes after plan
	defer func() {
		p.graph.RemoveTempNodes(startNodeID, goalNodeID)
	}()

	startCluster := p.clusters.GetOrCreate(startClusterCoord.X, startClusterCoord.Z)
	goalCluster := p.clusters.GetOrCreate(goalClusterCoord.X, goalClusterCoord.Z)

	// Connect start to startCluster entrances
	for _, e := range startCluster.entrances {
		if err := ctx.Err(); err != nil {
			return HPAPlanResult{Status: planner.PlanNoPath, Err: err}
		}
		segCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		res := p.localPlanner.Plan(segCtx, start[0], start[1], start[2], goal.NewGoalBlock(e.pos[0], e.pos[1], e.pos[2]), p.world)
		cancel()
		if res.Status == planner.PlanFound {
			cost := float64(len(res.Path))
			if eID, ok := p.graph.FindNodeByPos(e.pos); ok {
				p.graph.AddEdge(startNodeID, eID, cost, EdgeIntra)
			}
		}
	}

	// Connect goalCluster entrances to goal
	for _, e := range goalCluster.entrances {
		if err := ctx.Err(); err != nil {
			return HPAPlanResult{Status: planner.PlanNoPath, Err: err}
		}
		segCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		res := p.localPlanner.Plan(segCtx, e.pos[0], e.pos[1], e.pos[2], goal.NewGoalBlock(end[0], end[1], end[2]), p.world)
		cancel()
		if res.Status == planner.PlanFound {
			cost := float64(len(res.Path))
			if eID, ok := p.graph.FindNodeByPos(e.pos); ok {
				p.graph.AddEdge(eID, goalNodeID, cost, EdgeIntra)
			}
		}
	}

	p.graph.mu.RLock()
	startConnected := len(p.graph.edges[startNodeID]) > 0
	goalConnected := false
	for u := 0; u < len(p.graph.edges); u++ {
		for _, edge := range p.graph.edges[u] {
			if edge.To == goalNodeID {
				goalConnected = true
				break
			}
		}
		if goalConnected {
			break
		}
	}
	p.graph.mu.RUnlock()

	startCompSize, goalCompSize, sameComp := computeComponents(p.graph, startNodeID, goalNodeID)

	// Abstract A*
	pq := make(astarQueue, 0)
	heap.Init(&pq)

	startState := &astarNode{
		id: startNodeID,
		g:  0,
		h:  manhattanAbstract(start, end),
		f:  0,
	}
	startState.f = startState.g + startState.h
	heap.Push(&pq, startState)

	open := make(map[uint32]*astarNode)
	open[startNodeID] = startState
	closed := make(map[uint32]bool)

	var goalState *astarNode

	for pq.Len() > 0 {
		if err := ctx.Err(); err != nil {
			return HPAPlanResult{
				Status:             planner.PlanNoPath,
				Err:                err,
				StartConnected:     startConnected,
				GoalConnected:      goalConnected,
				StartComponentSize: startCompSize,
				GoalComponentSize:  goalCompSize,
				SameComponent:      sameComp,
			}
		}
		curr := heap.Pop(&pq).(*astarNode)
		delete(open, curr.id)

		if curr.id == goalNodeID {
			goalState = curr
			break
		}

		closed[curr.id] = true
		neighbors := p.graph.GetNeighbors(curr.id)

		for _, edge := range neighbors {
			if closed[edge.To] {
				continue
			}

			tentativeG := curr.g + edge.Cost
			neighbor, inOpen := open[edge.To]

			if inOpen && tentativeG >= neighbor.g {
				continue
			}

			if !inOpen {
				nData, _ := p.graph.Node(edge.To)
				neighbor = &astarNode{
					id: edge.To,
					h:  manhattanAbstract(nData.Pos, end),
				}
			}

			neighbor.parent = curr
			neighbor.g = tentativeG
			neighbor.f = tentativeG + neighbor.h

			if !inOpen {
				open[edge.To] = neighbor
				heap.Push(&pq, neighbor)
			} else {
				heap.Fix(&pq, neighbor.index)
			}
		}
	}

	if goalState == nil {
		return HPAPlanResult{
			Status:             planner.PlanNoPath,
			StartConnected:     startConnected,
			GoalConnected:      goalConnected,
			StartComponentSize: startCompSize,
			GoalComponentSize:  goalCompSize,
			SameComponent:      sameComp,
		}
	}

	var path []AbstractNode
	curr := goalState
	for curr != nil {
		n, _ := p.graph.Node(curr.id)
		path = append(path, n)
		curr = curr.parent
	}

	// reverse
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}

	refiner := &LazyRefiner{
		Path:         path,
		currentIndex: 0,
		localPlanner: p.localPlanner,
		world:        p.world,
		ctx:          ctx,
	}

	return HPAPlanResult{
		Status:             planner.PlanFound,
		AbstractPath:       path,
		Refiner:            refiner,
		StartConnected:     startConnected,
		GoalConnected:      goalConnected,
		StartComponentSize: startCompSize,
		GoalComponentSize:  goalCompSize,
		SameComponent:      sameComp,
	}
}

type LazyRefiner struct {
	mu           sync.RWMutex
	Path         []AbstractNode
	currentIndex int
	localPlanner *planner.Planner
	world        *world.World
	ctx          context.Context
	needsReplan  bool
}

func (r *LazyRefiner) NextSegment() []move.Movement {
	if r.IsComplete() {
		return nil
	}

	r.mu.Lock()
	start := r.Path[r.currentIndex]
	end := r.Path[r.currentIndex+1]
	r.mu.Unlock()

	ctx := r.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	res := r.localPlanner.Plan(ctx, start.Pos[0], start.Pos[1], start.Pos[2], goal.NewGoalBlock(end.Pos[0], end.Pos[1], end.Pos[2]), r.world)
	if res.Status == planner.PlanFound {
		r.mu.Lock()
		r.currentIndex++
		r.mu.Unlock()
		return res.Path
	}

	return nil
}

func (r *LazyRefiner) IsComplete() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.currentIndex >= len(r.Path)-1
}

func (r *LazyRefiner) CurrentIndex() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.currentIndex
}

func (r *LazyRefiner) NeedsReplan() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.needsReplan
}

func (r *LazyRefiner) SetNeedsReplan(val bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.needsReplan = val
}

func (r *LazyRefiner) Invalidate(blockPos [3]int) {
	if r.IsComplete() {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check if blockPos is in current cluster
	currNode := r.Path[r.currentIndex]
	clusterCoord := currNode.ClusterCoord

	bChunkX := blockToChunkCoord(blockPos[0])
	bChunkZ := blockToChunkCoord(blockPos[2])

	if clusterCoord.X == bChunkX && clusterCoord.Z == bChunkZ {
		// Mark for replan
		r.needsReplan = true
	}
}

// computeComponents finds the component size and whether start and goal are in the same component.
func computeComponents(g *AbstractGraph, startID, goalID uint32) (startSize int, goalSize int, same bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	n := len(g.nodes)
	if int(startID) >= n || int(goalID) >= n {
		return 0, 0, false
	}

	// Build undirected adjacency list
	adj := make([][]uint32, n)
	for u := 0; u < n; u++ {
		for _, edge := range g.edges[u] {
			v := edge.To
			if int(v) >= n {
				continue
			}
			adj[u] = append(adj[u], v)
			adj[v] = append(adj[v], uint32(u))
		}
	}

	// BFS helper
	bfs := func(root uint32) map[uint32]bool {
		visited := make(map[uint32]bool)
		visited[root] = true
		queue := []uint32{root}
		for len(queue) > 0 {
			curr := queue[0]
			queue = queue[1:]
			for _, neighbor := range adj[curr] {
				if !visited[neighbor] {
					visited[neighbor] = true
					queue = append(queue, neighbor)
				}
			}
		}
		return visited
	}

	startVisited := bfs(startID)
	if startVisited[goalID] {
		size := len(startVisited)
		return size, size, true
	}

	goalVisited := bfs(goalID)
	return len(startVisited), len(goalVisited), false
}
