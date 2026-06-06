package planner

import (
	"context"
	"testing"
	"time"

	"github.com/qrjhamron/feast/pkg/nav/goal"
	"github.com/qrjhamron/feast/pkg/nav/move"
	"github.com/qrjhamron/feast/pkg/world"
)

// flatChunk fills y=63 with stone across one chunk and records the surface.
func flatChunkWorld() (*world.World, *world.Chunk) {
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			ch.SetBlock(x, 63, z, world.BlockState{ID: 1, Name: "stone", Solid: true})
			ch.SurfaceY[z*world.ChunkWidth+x] = 63
		}
	}
	w.AddChunk(ch)
	return w, ch
}

// TestPlannerContextCancel verifies an already-cancelled context yields a
// non-found status promptly rather than running the full search budget.
func TestPlannerContextCancel(t *testing.T) {
	InvalidateCache()
	w, _ := flatChunkWorld()
	g := goal.NewGoalBlock(14, 64, 14)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before planning

	start := time.Now()
	res := Plan(ctx, 1, 64, 1, g, w)
	if elapsed := time.Since(start); elapsed > 200*time.Millisecond {
		t.Fatalf("cancelled plan took too long: %s", elapsed)
	}
	if res.Status == PlanFound {
		t.Fatalf("expected non-found status for cancelled context, got %s", res.Status)
	}
}

// TestPlannerNarrowCorridor routes through a 1-wide walled corridor: the only
// valid path is the corridor itself, exercising tight neighbor rejection.
func TestPlannerNarrowCorridor(t *testing.T) {
	InvalidateCache()
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			ch.SetBlock(x, 63, z, world.BlockState{ID: 1, Name: "stone", Solid: true})
			ch.SurfaceY[z*world.ChunkWidth+x] = 63
		}
	}
	// Two parallel walls forming a 1-wide corridor along x at z=8 (walls at z=7,z=9).
	for x := 2; x <= 12; x++ {
		for _, z := range []int{7, 9} {
			ch.SetBlock(x, 64, z, world.BlockState{ID: 1, Name: "stone", Solid: true})
			ch.SetBlock(x, 65, z, world.BlockState{ID: 1, Name: "stone", Solid: true})
		}
	}
	w.AddChunk(ch)

	g := goal.NewGoalBlock(12, 64, 8)
	res := Plan(context.Background(), 2, 64, 8, g, w)
	if res.Status != PlanFound || len(res.Path) == 0 {
		t.Fatalf("expected corridor path found, got %s len=%d", res.Status, len(res.Path))
	}
	pos := [3]int{2, 64, 8}
	for _, m := range expandForTest(res.Path, pos) {
		if m[2] != 8 {
			t.Fatalf("corridor path left z=8 lane at %v (would clip a wall)", m)
		}
	}
}

// TestPlannerStairsOneBlockUp ensures the planner can ascend a single-block
// step (via the jump primitive) to a raised goal.
func TestPlannerStairsOneBlockUp(t *testing.T) {
	InvalidateCache()
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			ch.SetBlock(x, 63, z, world.BlockState{ID: 1, Name: "stone", Solid: true})
			ch.SurfaceY[z*world.ChunkWidth+x] = 63
		}
	}
	// A raised step: blocks at y=64 for x>=5 form a one-block-high shelf.
	for x := 5; x < 16; x++ {
		for z := 0; z < 16; z++ {
			ch.SetBlock(x, 64, z, world.BlockState{ID: 1, Name: "stone", Solid: true})
			ch.SurfaceY[z*world.ChunkWidth+x] = 64
		}
	}
	w.AddChunk(ch)

	g := goal.NewGoalBlock(8, 65, 8) // standing on top of the shelf
	res := Plan(context.Background(), 1, 64, 8, g, w)
	if res.Status != PlanFound || len(res.Path) == 0 {
		t.Fatalf("expected stairs-up path found, got %s len=%d", res.Status, len(res.Path))
	}
	pos := [3]int{1, 64, 8}
	for _, m := range res.Path {
		pos = m.Destination(pos)
	}
	if !g.Satisfied(pos[0], pos[1], pos[2]) {
		t.Fatalf("stairs path did not reach raised goal, ended at %v", pos)
	}
}

// TestPlannerStartNodePrefersStandableFeet locks in the feet-Y semantics fix:
// a valid (low) feet position must NOT be snapped up to the terrain surface,
// which would otherwise plan an impossible route from above the bot.
func TestPlannerStartNodePrefersStandableFeet(t *testing.T) {
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	// Tall surface column (cache reports y=120) with a real shelf block there ...
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			ch.SetBlock(x, 120, z, world.BlockState{ID: 1, Name: "stone", Solid: true})
			ch.SurfaceY[z*world.ChunkWidth+x] = 120
		}
	}
	// ... but a real, standable cave floor far below at y=10 (feet at y=11).
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			ch.SetBlock(x, 10, z, world.BlockState{ID: 1, Name: "stone", Solid: true})
		}
	}
	w.AddChunk(ch)

	if got := normalizeStartYToSurface(4, 11, 4, w); got != 11 {
		t.Fatalf("standable cave feet Y must be preserved, got %d want 11", got)
	}
	// A detached/sentinel start that is NOT standable should still fall back to
	// the surface so callers passing a bogus Y recover.
	if got := normalizeStartYToSurface(4, 250, 4, w); got != 121 {
		t.Fatalf("non-standable detached start should snap to surface+1=121, got %d", got)
	}
}

// TestPlannerCacheKeyTracksMovingEntity guards the cache-signature correctness
// fix: a GoalNear whose entity has moved must not return a stale cached path.
func TestPlannerCacheKeyTracksMovingEntity(t *testing.T) {
	w, _ := flatChunkWorld()
	e := &world.Entity{X: 5, Y: 64, Z: 5}
	g := goal.NewGoalNear(e, 1.0)

	k1 := buildCacheKey([3]int{1, 64, 1}, g, nil)
	e.X, e.Z = 12, 12 // entity moved
	k2 := buildCacheKey([3]int{1, 64, 1}, g, nil)
	if k1 == k2 {
		t.Fatalf("cache key must change when the GoalNear entity moves (k=%q)", k1.goal)
	}
	_ = w
}

// TestPlannerTieBreakKeepsOptimalLength confirms the f-tie-break did not make
// A* return longer-than-optimal paths on open terrain.
func TestPlannerTieBreakKeepsOptimalLength(t *testing.T) {
	InvalidateCache()
	w, _ := flatChunkWorld()
	g := goal.NewGoalBlock(11, 64, 1)
	res := Plan(context.Background(), 1, 64, 1, g, w)
	if res.Status != PlanFound {
		t.Fatalf("expected found, got %s", res.Status)
	}
	// Optimal is 10 cardinal steps east; smoothing collapses them into one
	// MoveWalkLine of 10 steps. Either way the expanded step count must be 10.
	pos := [3]int{1, 64, 1}
	steps := expandForTest(res.Path, pos)
	if len(steps) != 10 {
		t.Fatalf("expected optimal 10-step path, got %d steps", len(steps))
	}
}

// expandForTest returns the visited block coordinates of a (possibly smoothed)
// path, expanding line movements into individual steps.
func expandForTest(path []move.Movement, start [3]int) [][3]int {
	var out [][3]int
	pos := start
	for _, m := range path {
		switch lm := m.(type) {
		case move.MoveWalkLine:
			for i := 0; i < lm.Steps; i++ {
				pos = [3]int{pos[0] + lm.Dx, pos[1], pos[2] + lm.Dz}
				out = append(out, pos)
			}
		case move.MoveWalkDiagonalLine:
			for i := 0; i < lm.Steps; i++ {
				pos = [3]int{pos[0] + lm.Dx, pos[1], pos[2] + lm.Dz}
				out = append(out, pos)
			}
		default:
			pos = m.Destination(pos)
			out = append(out, pos)
		}
	}
	return out
}
