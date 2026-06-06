package move

import (
	"math"
	"testing"

	"github.com/qrjhamron/feast/pkg/world"
)

// TestWalkRejectsUnloadedTarget ensures an unloaded destination chunk is never
// treated as a free path. The single test chunk is at (0,0); a step toward x=16
// crosses into the unloaded chunk (1,0), where IsPassable returns false.
func TestWalkRejectsUnloadedTarget(t *testing.T) {
	w, ch := newTestWorld()
	from := [3]int{15, 10, 5}
	setSolid(ch, 15, 9, 5) // valid floor under the origin (still chunk 0)

	walk := MoveWalk{Dx: 1, Dz: 0} // dest (16,10,5) is in unloaded chunk (1,0)
	if cost := walk.Cost(w, from); !math.IsInf(cost, 1) {
		t.Fatalf("expected Inf walking into unloaded chunk, got %f", cost)
	}
}

// TestJumpRequiresHeadroom / TestJumpRejectsTwoBlockWall: a wall two blocks tall
// cannot be surmounted by a single jump (which only gains one block of height),
// while a one-block step up is accepted.
func TestJumpRejectsTwoBlockWall(t *testing.T) {
	w, ch := newTestWorld()
	from := [3]int{5, 10, 5}
	setSolid(ch, 5, 9, 5)

	jump := MoveJump{Dx: 1, Dz: 0} // dest (6,11,5)

	// One-block step up: only (6,10,5) is solid -> standable at (6,11,5).
	setSolid(ch, 6, 10, 5)
	if cost := jump.Cost(w, from); math.IsInf(cost, 1) {
		t.Fatalf("expected valid one-block step up, got Inf")
	}

	// Two-block wall: (6,10,5) AND (6,11,5) solid -> destination feet blocked.
	setSolid(ch, 6, 11, 5)
	if cost := jump.Cost(w, from); !math.IsInf(cost, 1) {
		t.Fatalf("expected Inf for two-block wall, got %f", cost)
	}
}

// TestParkourRejectsSolidArc ensures parkour cannot pass through a solid block
// occupying the jump arc (mid feet height), which would be clipping through a
// wall rather than clearing a gap.
func TestParkourRejectsSolidArc(t *testing.T) {
	w, ch := newTestWorld()
	from := [3]int{5, 10, 5}
	setSolid(ch, 5, 9, 5)
	setSolid(ch, 7, 9, 5) // landing floor

	parkour := MoveParkour{Dx: 1, Dz: 0}
	if cost := parkour.Cost(w, from); cost != 4.0 {
		t.Fatalf("expected valid parkour cost 4.0, got %f", cost)
	}

	// Solid block in the arc at feet height (mid cell) must reject the jump.
	setSolid(ch, 6, 10, 5)
	if cost := parkour.Cost(w, from); !math.IsInf(cost, 1) {
		t.Fatalf("expected Inf when a solid block fills the jump arc, got %f", cost)
	}
}

// TestMoveReplaceableBlockHandling documents that replaceable, non-collidable
// plants (empty bounding box) are walked through normally rather than treated
// as obstacles.
func TestMoveReplaceableBlockHandling(t *testing.T) {
	w, ch := newTestWorld()
	from := [3]int{5, 10, 5}
	setSolid(ch, 5, 9, 5)
	setSolid(ch, 6, 9, 5)

	// Sanity: the world must consider these plants passable; otherwise the test
	// is meaningless on this registry.
	for _, name := range []string{"minecraft:short_grass", "minecraft:tall_grass", "minecraft:fern"} {
		ch.SetBlock(6, 10, 5, world.BlockState{Name: name})
		if !w.IsPassable(6, 10, 5) {
			t.Fatalf("expected %s to be passable in the registry", name)
		}
		walk := MoveWalk{Dx: 1, Dz: 0}
		if cost := walk.Cost(w, from); cost != 1.0 {
			t.Fatalf("expected walk through %s to cost 1.0, got %f", name, cost)
		}
	}
}

// TestWalkReorderPreservesSemantics guards the Cost-check reordering: the water
// gate must still reject walking out of a water column even though it now runs
// after the geometry checks.
func TestWalkReorderPreservesSemantics(t *testing.T) {
	w, ch := newTestWorld()
	from := [3]int{5, 10, 5}
	setSolid(ch, 5, 9, 5)
	setSolid(ch, 6, 9, 5)
	// Origin feet in water: geometry is otherwise valid, water gate must reject.
	ch.SetBlock(5, 10, 5, world.BlockState{Name: "minecraft:water"})

	walk := MoveWalk{Dx: 1, Dz: 0}
	if cost := walk.Cost(w, from); !math.IsInf(cost, 1) {
		t.Fatalf("expected Inf walking while standing in water, got %f", cost)
	}
}
