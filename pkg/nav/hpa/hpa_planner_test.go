package hpa

import (
	"context"
	"errors"
	"testing"

	"github.com/user/feastgo/pkg/nav/goal"
	"github.com/user/feastgo/pkg/nav/planner"
	"github.com/user/feastgo/pkg/world"
)

func TestHPAPlanner(t *testing.T) {
	w := world.NewWorld()
	m := NewClusterManager(nil)
	g := NewAbstractGraph()
	builder := NewGraphBuilder(w, g, m)

	// Create 3 chunks in a line (0,0), (1,0), (2,0)
	for cx := 0; cx < 3; cx++ {
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

	builder.RebuildDirty()

	p := NewHPAPlanner(w, g, m)
	start := [3]int{0, 61, 8}
	end := [3]int{47, 61, 8}
	goalDef := goal.NewGoalBlock(end[0], end[1], end[2])

	res := p.Plan(start, end, goalDef)

	if res.Status != planner.PlanFound {
		t.Fatalf("expected path found, got %v", res.Status)
	}

	if len(res.AbstractPath) == 0 {
		t.Fatalf("expected abstract path")
	}

	// Refine first segment
	segment := res.Refiner.NextSegment()
	if len(segment) == 0 {
		t.Fatalf("expected concrete segment moves")
	}

	if res.Refiner.IsComplete() {
		t.Fatalf("refiner should not be complete after 1 segment")
	}
}

func TestHPAPlanner_NoPath(t *testing.T) {
	w := world.NewWorld()
	m := NewClusterManager(nil)
	g := NewAbstractGraph()
	builder := NewGraphBuilder(w, g, m)

	// Create 2 disconnected chunks (0,0) and (2,0)
	chunk0 := world.NewChunk(0, 0)
	chunk2 := world.NewChunk(2, 0)
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			chunk0.SetBlock(x, 60, z, world.BlockState{Name: "stone", Solid: true})
			chunk0.SetBlock(x, 61, z, world.BlockState{Name: "air", Solid: false})
			chunk0.SetBlock(x, 62, z, world.BlockState{Name: "air", Solid: false})

			chunk2.SetBlock(x, 60, z, world.BlockState{Name: "stone", Solid: true})
			chunk2.SetBlock(x, 61, z, world.BlockState{Name: "air", Solid: false})
			chunk2.SetBlock(x, 62, z, world.BlockState{Name: "air", Solid: false})
		}
	}
	w.AddChunk(chunk0)
	w.AddChunk(chunk2)
	m.GetOrCreate(0, 0)
	m.GetOrCreate(2, 0)

	builder.RebuildDirty()

	p := NewHPAPlanner(w, g, m)
	start := [3]int{0, 61, 8}
	end := [3]int{47, 61, 8}
	goalDef := goal.NewGoalBlock(end[0], end[1], end[2])

	res := p.Plan(start, end, goalDef)

	if res.Status != planner.PlanNoPath {
		t.Fatalf("expected no path, got %v", res.Status)
	}
}

func TestHPAPlanner_NegativeChunkCoordinates(t *testing.T) {
	w := world.NewWorld()
	m := NewClusterManager(nil)
	g := NewAbstractGraph()
	builder := NewGraphBuilder(w, g, m)

	for cx := -13; cx <= -11; cx++ {
		chunk := world.NewChunk(cx, -1)
		for x := 0; x < 16; x++ {
			for z := 0; z < 16; z++ {
				chunk.SetBlock(x, 63, z, world.BlockState{Name: "stone", Solid: true})
				chunk.SetBlock(x, 64, z, world.BlockState{Name: "air", Solid: false})
				chunk.SetBlock(x, 65, z, world.BlockState{Name: "air", Solid: false})
			}
		}
		w.AddChunk(chunk)
		m.GetOrCreate(cx, -1)
	}

	builder.RebuildDirty()

	p := NewHPAPlanner(w, g, m)
	start := [3]int{-208, 64, -8}
	end := [3]int{-161, 64, -8}
	res := p.Plan(start, end, goal.NewGoalBlock(end[0], end[1], end[2]))

	if res.Status != planner.PlanFound {
		t.Fatalf("expected path found across negative chunks, got %v", res.Status)
	}
	if len(res.AbstractPath) < 2 {
		t.Fatalf("expected abstract path across negative chunks, got %d nodes", len(res.AbstractPath))
	}
	if segment := res.Refiner.NextSegment(); len(segment) == 0 {
		t.Fatalf("expected refined segment across negative chunks")
	}
}

func TestHPAPlanner_PlanWithContextCanceled(t *testing.T) {
	w := world.NewWorld()
	m := NewClusterManager(nil)
	g := NewAbstractGraph()
	p := NewHPAPlanner(w, g, m)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res := p.PlanWithContext(ctx, [3]int{0, 64, 0}, [3]int{16, 64, 0}, goal.NewGoalBlock(16, 64, 0))
	if res.Status != planner.PlanNoPath {
		t.Fatalf("expected no path on canceled context, got %v", res.Status)
	}
	if !errors.Is(res.Err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", res.Err)
	}
}
