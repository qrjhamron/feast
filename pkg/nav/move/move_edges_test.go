package move

import (
	"math"
	"testing"

	"github.com/qrjhamron/feast/pkg/world"
)

// negWorld builds a fully-air chunk at negative chunk coordinates (-1,-1),
// covering world blocks x,z in [-16,-1].
func negWorld() (*world.World, *world.Chunk) {
	w := world.NewWorld()
	ch := world.NewChunk(-1, -1)
	air := world.BlockState{Name: "air"}
	for y := 0; y < 20; y++ {
		for x := 0; x < 16; x++ {
			for z := 0; z < 16; z++ {
				ch.SetBlock(x, y, z, air)
			}
		}
	}
	w.AddChunk(ch)
	return w, ch
}

// setSolidAbs sets a solid block at absolute world coordinates using the chunk
// local setter (chunk is at (-1,-1) => local = abs + 16).
func setSolidAbs(ch *world.Chunk, x, y, z int) {
	ch.SetBlock(x+16, y, z+16, world.BlockState{Name: "stone", Solid: true})
}

func TestMoveWalkNegativeCoordinates(t *testing.T) {
	w, ch := negWorld()
	from := [3]int{-5, 10, -5}

	setSolidAbs(ch, -5, 9, -5)
	setSolidAbs(ch, -6, 9, -5)

	walk := MoveWalk{Dx: -1, Dz: 0}
	if cost := walk.Cost(w, from); cost != 1.0 {
		t.Fatalf("expected cost 1.0 for valid walk at negative coords, got %f", cost)
	}
	if dest := walk.Destination(from); dest != [3]int{-6, 10, -5} {
		t.Fatalf("unexpected destination at negative coords: %v", dest)
	}
}

func TestMoveWalkLavaBlocked(t *testing.T) {
	w, ch := newTestWorld()
	from := [3]int{5, 10, 5}

	setSolid(ch, 5, 9, 5)
	// Destination feet block is lava: not passable, so walk must be rejected.
	ch.SetBlock(6, 10, 5, world.BlockState{Name: "minecraft:lava"})
	ch.SetBlock(6, 9, 5, world.BlockState{Name: "stone", Solid: true})

	walk := MoveWalk{Dx: 1, Dz: 0}
	if cost := walk.Cost(w, from); !math.IsInf(cost, 1) {
		t.Fatalf("expected Inf walking into lava, got %f", cost)
	}
}

func TestMoveFallExcessiveHeightBlocked(t *testing.T) {
	w, ch := newTestWorld()
	from := [3]int{5, 18, 5}
	setSolid(ch, 5, 17, 5)

	// Falls deeper than -16 are not modeled and must be rejected outright.
	fall := MoveFall{Dx: 1, Dy: -17, Dz: 0}
	if cost := fall.Cost(w, from); !math.IsInf(cost, 1) {
		t.Fatalf("expected Inf for fall deeper than 16 blocks, got %f", cost)
	}
}

func TestMoveFallLavaLandingBlocked(t *testing.T) {
	w, ch := newTestWorld()
	from := [3]int{5, 10, 5}
	setSolid(ch, 5, 9, 5)

	// Landing column floor is lava (not solid => IsPassable(lava)==false means
	// "floor present"); but the landing feet block itself is lava and not
	// passable, so the drop path check rejects it.
	ch.SetBlock(6, 7, 5, world.BlockState{Name: "minecraft:lava"})

	fall := MoveFall{Dx: 1, Dy: -3, Dz: 0}
	if cost := fall.Cost(w, from); !math.IsInf(cost, 1) {
		t.Fatalf("expected Inf when drop path passes through lava, got %f", cost)
	}
}

func TestMoveSwimRejectsLava(t *testing.T) {
	w, ch := newTestWorld()
	from := [3]int{5, 10, 5}

	// Lava is not water: swim must not treat it as swimmable.
	ch.SetBlock(5, 10, 5, world.BlockState{Name: "minecraft:lava"})
	ch.SetBlock(6, 10, 5, world.BlockState{Name: "minecraft:lava"})

	swim := MoveSwim{Dx: 1, Dz: 0}
	if cost := swim.Cost(w, from); !math.IsInf(cost, 1) {
		t.Fatalf("expected Inf swimming through lava, got %f", cost)
	}
}
