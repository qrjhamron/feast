package hpa

import (
	"context"
	"sync"
	"time"

	"github.com/qrjhamron/feast/pkg/nav/goal"
	"github.com/qrjhamron/feast/pkg/nav/planner"
	"github.com/qrjhamron/feast/pkg/world"
)

const intraClusterEdgeTimeout = 3 * time.Second

type EdgeType int

const (
	EdgeIntra EdgeType = iota
	EdgeInter
)

type AbstractNode struct {
	ID           uint32
	Pos          [3]int
	ClusterCoord ClusterCoord
	TempNode     bool
}

type AbstractEdge struct {
	From     uint32
	To       uint32
	Cost     float64
	EdgeType EdgeType
}

type AbstractGraph struct {
	nodes     []AbstractNode
	edges     [][]AbstractEdge
	nodeIndex map[[3]int]uint32
	mu        sync.RWMutex
}

func NewAbstractGraph() *AbstractGraph {
	return &AbstractGraph{
		nodes:     make([]AbstractNode, 0),
		edges:     make([][]AbstractEdge, 0),
		nodeIndex: make(map[[3]int]uint32),
	}
}

func (g *AbstractGraph) AddNode(pos [3]int, cluster ClusterCoord) uint32 {
	g.mu.Lock()
	defer g.mu.Unlock()

	if id, ok := g.nodeIndex[pos]; ok {
		return id
	}

	id := uint32(len(g.nodes))
	g.nodes = append(g.nodes, AbstractNode{
		ID:           id,
		Pos:          pos,
		ClusterCoord: cluster,
		TempNode:     false,
	})
	g.edges = append(g.edges, nil)
	g.nodeIndex[pos] = id
	return id
}

// AddTempNode appends a temporary node without entering the permanent position index.
func (g *AbstractGraph) AddTempNode(pos [3]int, cluster ClusterCoord) uint32 {
	g.mu.Lock()
	defer g.mu.Unlock()
	id := uint32(len(g.nodes))
	g.nodes = append(g.nodes, AbstractNode{
		ID:           id,
		Pos:          pos,
		ClusterCoord: cluster,
		TempNode:     true,
	})
	g.edges = append(g.edges, nil)
	return id
}

// FindNodeByPos resolves a permanent abstract node by block position.
func (g *AbstractGraph) FindNodeByPos(pos [3]int) (uint32, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	id, ok := g.nodeIndex[pos]
	return id, ok
}

func (g *AbstractGraph) AddEdge(from, to uint32, cost float64, edgeType EdgeType) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.edges[from] = append(g.edges[from], AbstractEdge{
		From:     from,
		To:       to,
		Cost:     cost,
		EdgeType: edgeType,
	})
}

func (g *AbstractGraph) RemoveClusterNodes(cluster ClusterCoord) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Find nodes to remove
	var nodesToRemove []uint32
	for i, n := range g.nodes {
		if n.ClusterCoord == cluster {
			nodesToRemove = append(nodesToRemove, uint32(i))
			delete(g.nodeIndex, n.Pos)
			// we can't easily compact nodes without invalidating IDs.
			// Instead of full removal and ID shifts, we can just clear their edges.
			// Or we can leave them disconnected. For a dynamic graph, we might want a freelist
			// or just to clear edges pointing to/from them.
			// For now, let's just clear edges to/from these nodes to "remove" them logically.
			g.edges[i] = nil
		}
	}

	// Remove incoming edges
	for i := range g.edges {
		var newEdges []AbstractEdge
		for _, e := range g.edges[i] {
			keep := true
			for _, r := range nodesToRemove {
				if e.To == r {
					keep = false
					break
				}
			}
			if keep {
				newEdges = append(newEdges, e)
			}
		}
		g.edges[i] = newEdges
	}
}

// RemoveTempNodes removes only temporary nodes and edges connected to them.
func (g *AbstractGraph) RemoveTempNodes(ids ...uint32) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if len(ids) == 0 {
		return
	}

	tempSet := make(map[uint32]struct{}, len(ids))
	for _, id := range ids {
		if int(id) >= len(g.nodes) {
			continue
		}
		if !g.nodes[id].TempNode {
			continue
		}
		tempSet[id] = struct{}{}
		g.edges[id] = nil
	}
	if len(tempSet) == 0 {
		return
	}

	for i := range g.edges {
		if _, isTemp := tempSet[uint32(i)]; isTemp {
			continue
		}
		out := g.edges[i][:0]
		for _, e := range g.edges[i] {
			if _, drop := tempSet[e.To]; drop {
				continue
			}
			out = append(out, e)
		}
		g.edges[i] = out
	}
}

func (g *AbstractGraph) GetNeighbors(nodeID uint32) []AbstractEdge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if int(nodeID) >= len(g.edges) {
		return nil
	}
	// return a copy to prevent data races
	res := make([]AbstractEdge, len(g.edges[nodeID]))
	copy(res, g.edges[nodeID])
	return res
}

func (g *AbstractGraph) Node(id uint32) (AbstractNode, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	if int(id) >= len(g.nodes) {
		return AbstractNode{}, false
	}
	return g.nodes[id], true
}

// Stats returns current abstract graph node and edge counts.
func (g *AbstractGraph) Stats() (nodes int, edges int) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	nodes = len(g.nodes)
	for _, e := range g.edges {
		edges += len(e)
	}
	return nodes, edges
}

type IntraClusterBuilder struct {
	Planner    *planner.Planner
	Optimistic bool
}

func NewIntraClusterBuilder() *IntraClusterBuilder {
	return &IntraClusterBuilder{
		Planner: planner.NewPlanner(),
	}
}

func (b *IntraClusterBuilder) BuildIntraEdges(w *world.World, cluster *Cluster, entrances []EntranceNode, g *AbstractGraph) {
	_ = b.BuildIntraEdgesWithContext(context.Background(), w, cluster, entrances, g)
}

func (b *IntraClusterBuilder) BuildIntraEdgesWithContext(ctx context.Context, w *world.World, cluster *Cluster, entrances []EntranceNode, g *AbstractGraph) error {
	if ctx == nil {
		ctx = context.Background()
	}
	// For each pair of entrance nodes in same cluster:
	for i := 0; i < len(entrances); i++ {
		for j := i + 1; j < len(entrances); j++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			start := entrances[i]
			end := entrances[j]
			id1 := g.AddNode(start.pos, cluster.coord)
			id2 := g.AddNode(end.pos, cluster.coord)

			if b.Optimistic {
				cost := manhattanAbstract(start.pos, end.pos)
				g.AddEdge(id1, id2, cost, EdgeIntra)
				g.AddEdge(id2, id1, cost, EdgeIntra)
				continue
			}

			// Run A*
			segCtx, cancel := context.WithTimeout(ctx, intraClusterEdgeTimeout)
			res := b.Planner.PlanWithExclusions(segCtx, start.pos[0], start.pos[1], start.pos[2], goal.NewGoalBlock(end.pos[0], end.pos[1], end.pos[2]), w, nil)
			cancel()

			if res.Status == planner.PlanFound {
				cost := float64(0)
				pos := start.pos
				for _, m := range res.Path {
					cost += m.Cost(w, pos)
					pos = m.Destination(pos)
				}

				g.AddEdge(id1, id2, cost, EdgeIntra)
				g.AddEdge(id2, id1, cost, EdgeIntra)
			}
		}
	}
	return nil
}

type GraphBuilder struct {
	graph        *AbstractGraph
	clusters     *ClusterManager
	world        *world.World
	scanner      *TransitionScanner
	intraBuilder *IntraClusterBuilder
}

func NewGraphBuilder(w *world.World, g *AbstractGraph, m *ClusterManager) *GraphBuilder {
	return &GraphBuilder{
		graph:        g,
		clusters:     m,
		world:        w,
		scanner:      &TransitionScanner{},
		intraBuilder: NewIntraClusterBuilder(),
	}
}

func (b *GraphBuilder) SetOptimisticIntraEdges(enabled bool) {
	b.intraBuilder.Optimistic = enabled
}

func (b *GraphBuilder) BuildCluster(c *Cluster) {
	_ = b.BuildClusterWithContext(context.Background(), c)
}

func (b *GraphBuilder) BuildClusterWithContext(ctx context.Context, c *Cluster) error {
	return b.buildCluster(ctx, c, true)
}

func (b *GraphBuilder) buildCluster(ctx context.Context, c *Cluster, refreshNeighbors bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !c.dirty {
		return nil
	}

	// remove old nodes logically
	b.graph.RemoveClusterNodes(c.coord)

	// find entrances with all loaded neighbors
	dirs := []ClusterCoord{
		{X: c.coord.X + 1, Z: c.coord.Z},
		{X: c.coord.X - 1, Z: c.coord.Z},
		{X: c.coord.X, Z: c.coord.Z + 1},
		{X: c.coord.X, Z: c.coord.Z - 1},
	}

	c.entrances = nil
	for _, nCoord := range dirs {
		if err := ctx.Err(); err != nil {
			return err
		}
		if neighbor, ok := b.clusters.clusters.Load(nCoord); ok {
			nCluster := neighbor.(*Cluster)
			ents := b.scanner.ScanBoundary(b.world, c, nCluster)
			c.entrances = append(c.entrances, ents...)
			removeBoundaryEntrances(nCluster, c.coord)

			// add inter edges
			for _, e := range ents {
				id1 := b.graph.AddNode(e.pos, c.coord)
				id2 := b.graph.AddNode(e.neighborPos, nCluster.coord)
				b.graph.AddEdge(id1, id2, 1.0, EdgeInter)
				b.graph.AddEdge(id2, id1, 1.0, EdgeInter)

				reverse := EntranceNode{
					pos:          e.neighborPos,
					clusterCoord: nCluster.coord,
					neighborPos:  e.pos,
					direction:    e.direction,
				}
				nCluster.entrances = appendUniqueEntrance(nCluster.entrances, reverse)
			}
		}
	}

	if err := b.intraBuilder.BuildIntraEdgesWithContext(ctx, b.world, c, c.entrances, b.graph); err != nil {
		return err
	}
	c.dirty = false

	if refreshNeighbors {
		for _, nCoord := range dirs {
			if err := ctx.Err(); err != nil {
				return err
			}
			if neighbor, ok := b.clusters.clusters.Load(nCoord); ok {
				nCluster := neighbor.(*Cluster)
				nCluster.dirty = true
				if err := b.buildCluster(ctx, nCluster, false); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (b *GraphBuilder) RebuildDirty() {
	_ = b.RebuildDirtyWithContext(context.Background())
}

func (b *GraphBuilder) RebuildDirtyWithContext(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var dirty []*Cluster
	b.clusters.clusters.Range(func(key, value interface{}) bool {
		c := value.(*Cluster)
		if c.dirty {
			dirty = append(dirty, c)
		}
		return true
	})

	for _, c := range dirty {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := b.BuildClusterWithContext(ctx, c); err != nil {
			return err
		}
	}
	return nil
}

func appendUniqueEntrance(existing []EntranceNode, e EntranceNode) []EntranceNode {
	for _, cur := range existing {
		if cur.pos == e.pos && cur.neighborPos == e.neighborPos {
			return existing
		}
	}
	return append(existing, e)
}

func removeBoundaryEntrances(c *Cluster, neighbor ClusterCoord) {
	filtered := c.entrances[:0]
	for _, e := range c.entrances {
		nx := blockToChunkCoord(e.neighborPos[0])
		nz := blockToChunkCoord(e.neighborPos[2])
		if nx == neighbor.X && nz == neighbor.Z {
			continue
		}
		filtered = append(filtered, e)
	}
	c.entrances = filtered
}

// RebuildAll marks all clusters dirty and rebuilds them.
func (b *GraphBuilder) RebuildAll() {
	_ = b.RebuildAllWithContext(context.Background())
}

func (b *GraphBuilder) RebuildAllWithContext(ctx context.Context) error {
	b.clusters.ForEach(func(c *Cluster) {
		c.dirty = true
	})
	return b.RebuildDirtyWithContext(ctx)
}
