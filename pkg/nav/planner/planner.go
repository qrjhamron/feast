package planner

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"sync"
	"time"

	"github.com/user/feastgo/pkg/nav/goal"
	"github.com/user/feastgo/pkg/nav/move"
	"github.com/user/feastgo/pkg/world"
)

type Node struct {
	pos      [3]int
	g        float64
	h        float64
	f        float64
	parent   *Node
	movement move.Movement
	index    int
}

type PlanStatus int

const (
	PlanFound PlanStatus = iota
	PlanPartial
	PlanNoPath
	PlanTimeout
	PlanCancelled
)

func (s PlanStatus) String() string {
	switch s {
	case PlanFound:
		return "found"
	case PlanPartial:
		return "partial"
	case PlanNoPath:
		return "no_path"
	case PlanTimeout:
		return "timeout"
	case PlanCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

type PlanResult struct {
	Status PlanStatus
	Path   []move.Movement
}

type cacheKey struct {
	start [3]int
	goal  string
}

type cacheEntry struct {
	at  time.Time
	seq uint64
	gen uint64
}

type PlannerOptions struct {
	AvoidEntities bool
	Pose          string
}

type Planner struct {
	cache     map[cacheKey]PlanResult
	cacheMu   sync.Mutex
	cacheMax  int
	cacheMeta map[cacheKey]cacheEntry
	cacheGen  uint64
	cacheSeq  uint64
	options   PlannerOptions
}

func (p *Planner) SetOptions(opts PlannerOptions) {
	p.options = opts
}

// DebugLogs enables verbose planner logging. It should remain false in
// production runs because it materially affects planner performance.
var DebugLogs = false

func debugf(format string, args ...any) {
	if !DebugLogs {
		return
	}
	log.Printf(format, args...)
}

var nodePool = sync.Pool{
	New: func() any { return new(Node) },
}

type PriorityQueue []*Node

func (pq PriorityQueue) Len() int           { return len(pq) }
func (pq PriorityQueue) Less(i, j int) bool { return pq[i].f < pq[j].f }
func (pq PriorityQueue) Swap(i, j int) {
	pq[i], pq[j] = pq[j], pq[i]
	pq[i].index = i
	pq[j].index = j
}
func (pq *PriorityQueue) Push(x interface{}) {
	n := len(*pq)
	node := x.(*Node)
	node.index = n
	*pq = append(*pq, node)
}
func (pq *PriorityQueue) Pop() interface{} {
	old := *pq
	n := len(old)
	node := old[n-1]
	node.index = -1
	*pq = old[0 : n-1]
	return node
}

var allMovements []move.Movement

var (
	defaultPlanner = NewPlanner()
	cacheWindow    = 500 * time.Millisecond
)

func init() {
	dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for _, d := range dirs {
		allMovements = append(allMovements, move.MoveWalk{Dx: d[0], Dz: d[1]})
		allMovements = append(allMovements, move.MoveSwim{Dx: d[0], Dz: d[1]})
		allMovements = append(allMovements, move.MoveSwimUp{Dx: d[0], Dz: d[1]})
		allMovements = append(allMovements, move.MoveParkour{Dx: d[0], Dz: d[1]})
		allMovements = append(allMovements, move.MoveJump{Dx: d[0], Dz: d[1]})
		for dy := -1; dy >= -8; dy-- {
			allMovements = append(allMovements, move.MoveFall{Dx: d[0], Dy: dy, Dz: d[1]})
		}
	}
	allMovements = append(allMovements, move.MoveClimb{})

	diags := [][2]int{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}}
	for _, d := range diags {
		allMovements = append(allMovements, move.MoveWalkDiagonal{Dx: d[0], Dz: d[1]})
	}
}

func NewPlanner() *Planner {
	return &Planner{
		cache:     make(map[cacheKey]PlanResult),
		cacheMeta: make(map[cacheKey]cacheEntry),
		cacheMax:  64,
	}
}

// InvalidateCache invalidates cached planning results.
func InvalidateCache() {
	defaultPlanner.InvalidateCache()
}

// InvalidateCache invalidates this planner's cache generation.
func (p *Planner) InvalidateCache() {
	p.cacheMu.Lock()
	p.cacheGen++
	p.cacheMu.Unlock()
}

// PlanWithExclusions computes a path while skipping any destination node that
// matches one of the excluded block coordinates.
func PlanWithExclusions(ctx context.Context, startX, startY, startZ int, g goal.Goal, w *world.World, excluded [][3]int) PlanResult {
	return defaultPlanner.PlanWithExclusions(ctx, startX, startY, startZ, g, w, excluded)
}

func (p *Planner) PlanWithExclusions(ctx context.Context, startX, startY, startZ int, g goal.Goal, w *world.World, excluded [][3]int) PlanResult {
	excludedSet := make(map[[3]int]struct{}, len(excluded))
	for _, pos := range excluded {
		excludedSet[pos] = struct{}{}
	}
	return p.planInternal(ctx, startX, startY, startZ, g, w, excludedSet)
}

func Plan(ctx context.Context, startX, startY, startZ int, g goal.Goal, w *world.World) PlanResult {
	return defaultPlanner.Plan(ctx, startX, startY, startZ, g, w)
}

func (p *Planner) Plan(ctx context.Context, startX, startY, startZ int, g goal.Goal, w *world.World) PlanResult {
	return p.planInternal(ctx, startX, startY, startZ, g, w, nil)
}

func (p *Planner) planInternal(ctx context.Context, startX, startY, startZ int, g goal.Goal, w *world.World, excluded map[[3]int]struct{}) PlanResult {
	if ctx == nil {
		ctx = context.Background()
	}
	startY = normalizeStartYToSurface(startX, startY, startZ, w)
	startPos := [3]int{startX, startY, startZ}
	planStart := time.Now()
	goalX, goalZ := plannerGoalXZ(g)
	chunks := 0
	if w != nil {
		chunks = w.ChunkCount()
	}
	debugf("planner.Plan start=(%d,%d,%d) goal=(%d,%d) world_chunks=%d", startX, startY, startZ, goalX, goalZ, chunks)

	logResult := func(status PlanStatus, path []move.Movement) {
		debugf("planner.Plan result=status=%s steps=%d elapsed=%dms", status.String(), len(path), time.Since(planStart).Milliseconds())
	}

	// Check if timeout is set, if not use adaptive budget by start-to-goal distance.
	_, ok := ctx.Deadline()
	if !ok {
		manhattan := estimateManhattanDistance(g, startPos)
		timeout := adaptiveTimeout(manhattan)
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	cacheKey := buildCacheKey(startPos, g, excluded)
	if excluded == nil {
		if hit, ok := p.getCachedPlan(cacheKey); ok {
			return hit
		}
	}

	if dl, hasDL := ctx.Deadline(); hasDL {
		debugf("planner.Plan enter start=%v goal=%T deadline=%s", startPos, g, dl.UTC().Format(time.RFC3339Nano))
	} else {
		debugf("planner.Plan enter start=%v goal=%T deadline=none", startPos, g)
	}
	startH := g.Heuristic(startX, startY, startZ) * 0.5
	allocated := make([]*Node, 0, 1024)
	allocNode := func() *Node {
		n := nodePool.Get().(*Node)
		*n = Node{}
		allocated = append(allocated, n)
		return n
	}
	defer func() {
		for _, n := range allocated {
			*n = Node{}
			nodePool.Put(n)
		}
	}()

	startNode := allocNode()
	startNode.pos = startPos
	startNode.h = startH
	startNode.f = startH

	pq := make(PriorityQueue, 0)
	heap.Init(&pq)
	heap.Push(&pq, startNode)

	closed := make(map[[3]int]struct{})
	openMap := make(map[[3]int]*Node)
	openMap[startNode.pos] = startNode

	// bestNode is the closest reachable fallback used when we time out, exhaust
	// the frontier, or hit maxNodes.
	var bestNode *Node
	bestH := math.Inf(1)
	nodesExpanded := 0
	maxNodes := maxNodesForDistance(estimateManhattanDistance(g, startPos))
	maxSteps := 200

	for pq.Len() > 0 {
		select {
		case <-ctx.Done():
			path := constructPath(bestNode)
			status := PlanCancelled
			reason := "context_canceled"
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				status = PlanTimeout
				reason = "timeout"
			}
			debugf("planner.Plan exit reason=%s nodesExpanded=%d pathLen=%d", reason, nodesExpanded, len(path))
			res := validatePath(startPos, g, w, PlanResult{Status: status, Path: path})
			logResult(res.Status, res.Path)
			return res
		default:
		}

		if nodesExpanded >= maxNodes {
			path := constructPath(bestNode)
			reason := "max_nodes"
			debugf("planner.Plan exit reason=%s nodesExpanded=%d pathLen=%d", reason, nodesExpanded, len(path))
			res := validatePath(startPos, g, w, PlanResult{Status: PlanPartial, Path: path})
			logResult(res.Status, res.Path)
			return res
		}

		current := heap.Pop(&pq).(*Node)
		delete(openMap, current.pos)

		if g.Satisfied(current.pos[0], current.pos[1], current.pos[2]) {
			path := constructPath(current)
			path = smoothPath(startPos, path)
			if len(path) > maxSteps {
				path = path[:maxSteps]
			}
			debugf("planner.Plan exit reason=goal_found nodesExpanded=%d pathLen=%d goalPos=%v", nodesExpanded, len(path), current.pos)
			res := validatePath(startPos, g, w, PlanResult{Status: PlanFound, Path: path})
			logResult(res.Status, res.Path)
			if excluded == nil {
				p.setCachedPlan(cacheKey, res)
			}
			return res
		}

		closed[current.pos] = struct{}{}
		nodesExpanded++

		for _, m := range allMovements {
			cost := m.Cost(w, current.pos)
			if math.IsInf(cost, 1) {
				continue
			}

			dest := m.Destination(current.pos)
			if dest[1] < -64 || dest[1] > 319 {
				continue
			}
			if p.options.AvoidEntities && w != nil {
				height := 1.8
				switch p.options.Pose {
				case "sneaking":
					height = 1.5
				case "swimming", "crawling":
					height = 0.6
				}
				botAABB := world.AABB{
					MinX: float64(dest[0]) + 0.2,
					MinY: float64(dest[1]),
					MinZ: float64(dest[2]) + 0.2,
					MaxX: float64(dest[0]) + 0.8,
					MaxY: float64(dest[1]) + height,
					MaxZ: float64(dest[2]) + 0.8,
				}
				if w.IsEntityBlocking(botAABB) {
					continue
				}
			}
			if _, isExcluded := excluded[dest]; isExcluded {
				continue
			}
			if _, ok := closed[dest]; ok {
				continue
			}

			tentativeG := current.g + cost
			neighbor, inOpen := openMap[dest]

			if inOpen && tentativeG >= neighbor.g {
				continue
			}

			h := g.Heuristic(dest[0], dest[1], dest[2]) * 0.5

			if !inOpen {
				neighbor = allocNode()
				neighbor.pos = dest
			}

			neighbor.parent = current
			neighbor.movement = m
			neighbor.g = tentativeG
			neighbor.h = h
			neighbor.f = tentativeG + h

			if neighbor.parent != nil && h < bestH {
				bestNode = neighbor
				bestH = h
			}

			if !inOpen {
				openMap[dest] = neighbor
				heap.Push(&pq, neighbor)
			} else {
				heap.Fix(&pq, neighbor.index)
			}
		}
	}

	// Queue is empty, goal not reached
	path := constructPath(bestNode)
	path = smoothPath(startPos, path)
	if len(path) > maxSteps {
		path = path[:maxSteps]
	}
	reason := "open_empty"
	debugf("planner.Plan exit reason=%s nodesExpanded=%d pathLen=%d", reason, nodesExpanded, len(path))
	status := PlanNoPath
	if len(path) > 0 {
		status = PlanPartial
	}
	res := validatePath(startPos, g, w, PlanResult{Status: status, Path: path})
	logResult(res.Status, res.Path)
	if excluded == nil {
		p.setCachedPlan(cacheKey, res)
	}
	return res
}

func plannerGoalXZ(g goal.Goal) (x, z int) {
	switch v := g.(type) {
	case *goal.GoalBlock:
		return v.X, v.Z
	case *goal.GoalXZ:
		return v.X, v.Z
	default:
		return 0, 0
	}
}

func constructPath(n *Node) []move.Movement {
	var path []move.Movement
	curr := n
	for curr != nil && curr.movement != nil {
		path = append(path, curr.movement)
		curr = curr.parent
	}
	// Reverse the path
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path
}

func smoothPath(startPos [3]int, path []move.Movement) []move.Movement {
	if len(path) < 3 {
		return path
	}

	out := make([]move.Movement, 0, len(path))
	pos := startPos

	i := 0
	for i < len(path) {
		switch m := path[i].(type) {
		case move.MoveWalk:
			steps := 1
			for j := i + 1; j < len(path); j++ {
				nm, ok := path[j].(move.MoveWalk)
				if !ok || nm.Dx != m.Dx || nm.Dz != m.Dz {
					break
				}
				steps++
			}
			if steps >= 3 {
				out = append(out, move.MoveWalkLine{Dx: m.Dx, Dz: m.Dz, Steps: steps})
				for k := 0; k < steps; k++ {
					pos = m.Destination(pos)
				}
				i += steps
				continue
			}

		case move.MoveWalkDiagonal:
			steps := 1
			for j := i + 1; j < len(path); j++ {
				nm, ok := path[j].(move.MoveWalkDiagonal)
				if !ok || nm.Dx != m.Dx || nm.Dz != m.Dz {
					break
				}
				steps++
			}
			if steps >= 3 {
				out = append(out, move.MoveWalkDiagonalLine{Dx: m.Dx, Dz: m.Dz, Steps: steps})
				for k := 0; k < steps; k++ {
					pos = m.Destination(pos)
				}
				i += steps
				continue
			}
		}

		out = append(out, path[i])
		pos = path[i].Destination(pos)
		i++
	}

	return out
}

func validatePath(startPos [3]int, g goal.Goal, w *world.World, res PlanResult) PlanResult {
	if len(res.Path) == 0 {
		if res.Status == PlanFound {
			res.Status = PlanNoPath
		}
		return res
	}

	pos := startPos
	valid := 0
	for ; valid < len(res.Path); valid++ {
		m := res.Path[valid]
		if math.IsInf(m.Cost(w, pos), 1) {
			break
		}
		pos = m.Destination(pos)
	}

	if valid == len(res.Path) {
		if res.Status == PlanFound && !g.Satisfied(pos[0], pos[1], pos[2]) {
			res.Status = PlanPartial
		}
		return res
	}

	res.Path = res.Path[:valid]
	if len(res.Path) == 0 {
		res.Status = PlanNoPath
	} else if res.Status == PlanFound {
		res.Status = PlanPartial
	}
	return res
}

func normalizeStartYToSurface(startX, startY, startZ int, w *world.World) int {
	if w == nil {
		return startY
	}
	surfaceY := w.GetSurfaceY(startX, startZ)
	if surfaceY <= world.MinY-1 {
		return startY
	}
	standY := surfaceY + 1
	if !isStandableAt(w, startX, standY, startZ) {
		return startY
	}
	return standY
}

func isStandableAt(w *world.World, x, y, z int) bool {
	return w.IsPassable(x, y, z) &&
		w.IsPassable(x, y+1, z) &&
		!w.IsPassable(x, y-1, z)
}

func estimateManhattanDistance(g goal.Goal, start [3]int) int {
	if me, ok := g.(goal.ManhattanEstimator); ok {
		return me.ManhattanDistance(start[0], start[1], start[2])
	}
	h := g.Heuristic(start[0], start[1], start[2])
	if h <= 0 {
		return 0
	}
	return int(math.Ceil(h))
}

func adaptiveTimeout(distance int) time.Duration {
	switch {
	case distance < 20:
		return 800 * time.Millisecond
	case distance <= 100:
		return 2 * time.Second
	default:
		return 1000 * time.Millisecond
	}
}

func maxNodesForDistance(distance int) int {
	if distance <= 100 {
		return 10000
	}
	return 50000
}

func buildCacheKey(start [3]int, g goal.Goal, excluded map[[3]int]struct{}) cacheKey {
	exSig := ""
	if len(excluded) > 0 {
		exSig = fmt.Sprintf("|ex=%d", len(excluded))
	}
	return cacheKey{
		start: start,
		goal:  fmt.Sprintf("%T:%v%s", g, g, exSig),
	}
}

func (p *Planner) getCachedPlan(key cacheKey) (PlanResult, bool) {
	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()
	meta, ok := p.cacheMeta[key]
	if !ok || meta.gen != p.cacheGen || time.Since(meta.at) > cacheWindow {
		return PlanResult{}, false
	}
	res, ok := p.cache[key]
	if !ok {
		return PlanResult{}, false
	}
	return PlanResult{Status: res.Status, Path: append([]move.Movement(nil), res.Path...)}, true
}

func (p *Planner) setCachedPlan(key cacheKey, res PlanResult) {
	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()

	p.cacheSeq++
	p.cache[key] = PlanResult{Status: res.Status, Path: append([]move.Movement(nil), res.Path...)}
	p.cacheMeta[key] = cacheEntry{
		at:  time.Now(),
		seq: p.cacheSeq,
		gen: p.cacheGen,
	}
	if len(p.cache) <= p.cacheMax {
		return
	}

	var (
		oldestKey  cacheKey
		oldestSeq  uint64
		haveOldest bool
	)
	for k, meta := range p.cacheMeta {
		if !haveOldest || meta.seq < oldestSeq {
			oldestSeq = meta.seq
			oldestKey = k
			haveOldest = true
		}
	}
	if haveOldest && len(p.cache) > p.cacheMax {
		delete(p.cache, oldestKey)
		delete(p.cacheMeta, oldestKey)
	}
}
