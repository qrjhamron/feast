package planner

import (
	"math"
	"testing"

	"github.com/qrjhamron/feast/pkg/nav/move"
	"github.com/qrjhamron/feast/pkg/world"
)

// TestSmoothPath_DoesNotClipThroughWall verifies that smoothing a path of
// >=3 walks followed by a direction change does not merge across the turn,
// which would result in a MoveWalkLine clipping through diagonal blocks.
func TestSmoothPath_DoesNotClipThroughWall(t *testing.T) {
	// Path: 3 east, then 1 north. Smoothing must NOT merge all 4 into one line.
	start := [3]int{0, 64, 0}
	path := []move.Movement{
		move.MoveWalk{Dx: 1, Dz: 0},
		move.MoveWalk{Dx: 1, Dz: 0},
		move.MoveWalk{Dx: 1, Dz: 0},
		move.MoveWalk{Dx: 0, Dz: 1},
	}

	out := smoothPath(start, path)
	// The first 3 walks should be merged into a line, and the 4th remains.
	if len(out) != 2 {
		t.Fatalf("expected 2 movements, got %d: %#v", len(out), out)
	}
	line, ok := out[0].(move.MoveWalkLine)
	if !ok {
		t.Fatalf("expected MoveWalkLine first, got %T", out[0])
	}
	if line.Steps != 3 || line.Dx != 1 || line.Dz != 0 {
		t.Fatalf("unexpected line: %+v", line)
	}
}

// TestSmoothPath_DiagonalLineRevalidatesEachStep confirms MoveWalkDiagonalLine
// has infinite cost when any single step is blocked (meaning smoothing output
// is still world-validated).
func TestSmoothPath_DiagonalLineRevalidatesEachStep(t *testing.T) {
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	// floor for all
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			ch.SetBlock(x, 63, z, world.BlockState{ID: 1, Name: "stone", Solid: true})
		}
	}
	// Place wall blocking adj-cardinal of step 2 (corner clip at x=2,z=1)
	ch.SetBlock(2, 64, 1, world.BlockState{ID: 1, Name: "stone", Solid: true})
	w.AddChunk(ch)

	line := move.MoveWalkDiagonalLine{Dx: 1, Dz: 1, Steps: 3}
	from := [3]int{0, 64, 0}
	cost := line.Cost(w, from)
	if !math.IsInf(cost, 1) {
		t.Fatalf("expected Inf for diagonal line with blocked step, got %f", cost)
	}
}
