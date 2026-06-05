package hpa

import (
	"testing"
	"time"

	"github.com/qrjhamron/feast/pkg/state"
	"github.com/qrjhamron/feast/pkg/world"
)

func TestGraphUpdater(t *testing.T) {
	w := world.NewWorld()
	bus := state.NewEventBus()
	m := NewClusterManager(bus)
	g := NewAbstractGraph()
	b := NewGraphBuilder(w, g, m)

	chunk := world.NewChunk(0, 0)
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			chunk.SetBlock(x, 60, z, world.BlockState{Name: "stone", Solid: true})
			chunk.SetBlock(x, 61, z, world.BlockState{Name: "air", Solid: false})
			chunk.SetBlock(x, 62, z, world.BlockState{Name: "air", Solid: false})
		}
	}
	w.AddChunk(chunk)
	m.GetOrCreate(0, 0)

	u := NewGraphUpdater(w, g, m, b, bus)
	u.Start()
	defer u.Stop()

	// Initial build
	b.RebuildDirty()

	// create fake refiner
	refiner := &LazyRefiner{
		Path: []AbstractNode{
			{ClusterCoord: ClusterCoord{X: 0, Z: 0}},
			{ClusterCoord: ClusterCoord{X: 1, Z: 0}},
		},
		currentIndex: 0,
	}
	u.SetActiveRefiner(refiner)

	// Send block update
	u.NotifyUpdate([3]int{5, 60, 5})

	// Wait for async rebuild
	time.Sleep(100 * time.Millisecond)

	// Check if refiner was invalidated
	if !refiner.NeedsReplan() {
		t.Fatalf("expected refiner to be marked NeedsReplan")
	}

	// Check if cluster was rebuilt
	c := m.GetOrCreate(0, 0)
	if c.dirty {
		t.Fatalf("expected cluster to be clean after rebuild")
	}
}
