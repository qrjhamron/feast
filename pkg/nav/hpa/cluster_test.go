package hpa

import (
	"testing"

	"github.com/user/feastgo/pkg/world"
)

func TestClusterManager(t *testing.T) {
	m := NewClusterManager(nil)
	c := m.GetOrCreate(0, 0)
	if c.coord.X != 0 || c.coord.Z != 0 {
		t.Errorf("expected 0,0 got %v", c.coord)
	}
	if !c.dirty {
		t.Errorf("expected dirty")
	}

	c.dirty = false
	m.Invalidate(0, 0) // invalidates 0,0 and -1,0 and 0,-1
	if !c.dirty {
		t.Errorf("expected dirty after invalidate")
	}
}

func TestTransitionScanner(t *testing.T) {
	w := world.NewWorld()
	chunkA := world.NewChunk(0, 0)
	chunkB := world.NewChunk(1, 0)

	// Create a flat surface across A and B at y=60
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			chunkA.SetBlock(x, 60, z, world.BlockState{Name: "stone", Solid: true})
			chunkA.SetBlock(x, 61, z, world.BlockState{Name: "air", Solid: false})
			chunkA.SetBlock(x, 62, z, world.BlockState{Name: "air", Solid: false})

			chunkB.SetBlock(x, 60, z, world.BlockState{Name: "stone", Solid: true})
			chunkB.SetBlock(x, 61, z, world.BlockState{Name: "air", Solid: false})
			chunkB.SetBlock(x, 62, z, world.BlockState{Name: "air", Solid: false})
		}
	}
	w.AddChunk(chunkA)
	w.AddChunk(chunkB)

	m := NewClusterManager(nil)
	cA := m.GetOrCreate(0, 0)
	cB := m.GetOrCreate(1, 0)

	scanner := &TransitionScanner{}
	entrances := scanner.ScanBoundary(w, cA, cB)

	if len(entrances) != 1 {
		t.Fatalf("expected 1 entrance, got %d", len(entrances))
	}
	e := entrances[0]
	// midpoint along Z is (0+15)/2 = 7
	if e.pos[0] != 15 || e.pos[1] != 61 || e.pos[2] != 7 {
		t.Errorf("unexpected entrance pos: %v", e.pos)
	}
}
