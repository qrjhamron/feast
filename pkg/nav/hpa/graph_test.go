package hpa

import (
	"fmt"
	"testing"

	"github.com/qrjhamron/feast/pkg/world"
)

func TestGraphBuilder(t *testing.T) {
	w := world.NewWorld()
	m := NewClusterManager(nil)
	g := NewAbstractGraph()
	builder := NewGraphBuilder(w, g, m)

	// Create 3x3 chunks
	for cx := 0; cx < 3; cx++ {
		for cz := 0; cz < 3; cz++ {
			chunk := world.NewChunk(cx, cz)
			for x := 0; x < 16; x++ {
				for z := 0; z < 16; z++ {
					chunk.SetBlock(x, 60, z, world.BlockState{Name: "stone", Solid: true})
					chunk.SetBlock(x, 61, z, world.BlockState{Name: "air", Solid: false})
					chunk.SetBlock(x, 62, z, world.BlockState{Name: "air", Solid: false})
				}
			}
			w.AddChunk(chunk)
			m.GetOrCreate(cx, cz)
		}
	}

	builder.RebuildDirty()

	g.mu.RLock()
	nodesCount := len(g.nodeIndex)
	g.mu.RUnlock()

	if nodesCount == 0 {
		t.Fatalf("expected nodes in graph, got 0")
	}

	if nodesCount != 24 {
		var poses []string
		for pos := range g.nodeIndex {
			poses = append(poses, fmt.Sprintf("%v", pos))
		}
		t.Errorf("expected 24 active nodes, got %d: %v", nodesCount, poses)
	}

	// Check edges
	g.mu.RLock()
	edgesCount := 0
	for _, edges := range g.edges {
		edgesCount += len(edges)
	}
	g.mu.RUnlock()

	if edgesCount == 0 {
		t.Fatalf("expected edges in graph, got 0")
	}
}

func TestGraphBuilderOptimisticIntraEdges(t *testing.T) {
	w := world.NewWorld()
	m := NewClusterManager(nil)
	g := NewAbstractGraph()
	builder := NewGraphBuilder(w, g, m)
	builder.SetOptimisticIntraEdges(true)

	for cx := 0; cx < 2; cx++ {
		chunk := world.NewChunk(cx, 0)
		for x := 0; x < 16; x++ {
			for z := 0; z < 16; z++ {
				chunk.SetBlock(x, 60, z, world.BlockState{Name: "stone", Solid: true})
				chunk.SetBlock(x, 61, z, world.BlockState{Name: "air", Solid: false})
				chunk.SetBlock(x, 62, z, world.BlockState{Name: "air", Solid: false})
			}
		}
		w.AddChunk(chunk)
		m.GetOrCreate(cx, 0)
	}

	if err := builder.RebuildDirtyWithContext(nil); err != nil {
		t.Fatalf("rebuild: %v", err)
	}
	_, edges := g.Stats()
	if edges == 0 {
		t.Fatalf("expected optimistic graph edges")
	}
}
