package move

import (
	"math"
	"testing"

	"github.com/qrjhamron/feast/pkg/world"
)

func newTestWorld() (*world.World, *world.Chunk) {
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
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

func setSolid(ch *world.Chunk, x, y, z int) {
	ch.SetBlock(x, y, z, world.BlockState{Name: "stone", Solid: true})
}

func setPassable(ch *world.Chunk, x, y, z int) {
	ch.SetBlock(x, y, z, world.BlockState{Name: "air", Solid: false})
}

func TestMoveWalk(t *testing.T) {
	w, ch := newTestWorld()
	from := [3]int{5, 10, 5}

	setSolid(ch, 5, 9, 5)
	setSolid(ch, 6, 9, 5)

	walk := MoveWalk{Dx: 1, Dz: 0}

	if cost := walk.Cost(w, from); cost != 1.0 {
		t.Errorf("expected cost 1.0 for valid walk, got %f", cost)
	}

	setSolid(ch, 6, 10, 5)
	if cost := walk.Cost(w, from); cost != math.Inf(1) {
		t.Errorf("expected Inf for blocked destination, got %f", cost)
	}
	setPassable(ch, 6, 10, 5)

	setSolid(ch, 6, 11, 5)
	if cost := walk.Cost(w, from); cost != math.Inf(1) {
		t.Errorf("expected Inf for blocked head room at destination, got %f", cost)
	}
	setPassable(ch, 6, 11, 5)

	setSolid(ch, 5, 11, 5)
	if cost := walk.Cost(w, from); cost != math.Inf(1) {
		t.Errorf("expected Inf for blocked head room at from, got %f", cost)
	}
	setPassable(ch, 5, 11, 5)

	setPassable(ch, 6, 9, 5)
	if cost := walk.Cost(w, from); cost != math.Inf(1) {
		t.Errorf("expected Inf for no floor at destination, got %f", cost)
	}
}

func TestMoveJump(t *testing.T) {
	w, ch := newTestWorld()
	from := [3]int{5, 10, 5}

	setSolid(ch, 5, 9, 5)
	setSolid(ch, 6, 10, 5)

	jump := MoveJump{Dx: 1, Dz: 0}

	if cost := jump.Cost(w, from); cost != 2.0 {
		t.Errorf("expected cost 2.0 for valid jump, got %f", cost)
	}

	setSolid(ch, 5, 12, 5)
	if cost := jump.Cost(w, from); cost != math.Inf(1) {
		t.Errorf("expected Inf when headroom at from blocked, got %f", cost)
	}
	setPassable(ch, 5, 12, 5)

	setSolid(ch, 6, 12, 5)
	if cost := jump.Cost(w, from); cost != math.Inf(1) {
		t.Errorf("expected Inf when headroom at dest blocked, got %f", cost)
	}
	setPassable(ch, 6, 12, 5)

	setSolid(ch, 6, 11, 5)
	if cost := jump.Cost(w, from); cost != math.Inf(1) {
		t.Errorf("expected Inf when dest is blocked, got %f", cost)
	}
	setPassable(ch, 6, 11, 5)

	setPassable(ch, 6, 10, 5)
	if cost := jump.Cost(w, from); cost != math.Inf(1) {
		t.Errorf("expected Inf when dest floor is missing, got %f", cost)
	}
}

func TestMoveFall(t *testing.T) {
	w, ch := newTestWorld()
	from := [3]int{5, 10, 5}

	fall := MoveFall{Dx: 1, Dy: -2, Dz: 0}

	setSolid(ch, 5, 9, 5)
	setSolid(ch, 6, 7, 5)

	if cost := fall.Cost(w, from); cost != 1.0 {
		t.Errorf("expected cost 1.0 for valid fall, got %f", cost)
	}

	fall4 := MoveFall{Dx: 1, Dy: -4, Dz: 0}
	setPassable(ch, 6, 7, 5)
	setSolid(ch, 6, 5, 5)
	if cost := fall4.Cost(w, from); cost != 3.0 {
		t.Errorf("expected cost 3.0 for 4-block fall (includes damage penalty), got %f", cost)
	}
	setSolid(ch, 6, 7, 5)

	fall0 := MoveFall{Dx: 1, Dy: 0, Dz: 0}
	if cost := fall0.Cost(w, from); cost != math.Inf(1) {
		t.Errorf("expected Inf for drop >= 0 blocks, got %f", cost)
	}

	setSolid(ch, 6, 9, 5)
	if cost := fall.Cost(w, from); cost != math.Inf(1) {
		t.Errorf("expected Inf when drop path blocked, got %f", cost)
	}
	setPassable(ch, 6, 9, 5)

	setSolid(ch, 6, 11, 5)
	if cost := fall.Cost(w, from); cost != math.Inf(1) {
		t.Errorf("expected Inf when head room stepping in is blocked, got %f", cost)
	}
	setPassable(ch, 6, 11, 5)

	setPassable(ch, 6, 7, 5)
	if cost := fall.Cost(w, from); cost != math.Inf(1) {
		t.Errorf("expected Inf when dest floor missing, got %f", cost)
	}
}

func TestMoveWalkDiagonal(t *testing.T) {
	w, ch := newTestWorld()
	from := [3]int{5, 10, 5}

	// Ensure floors exist under all involved squares.
	setSolid(ch, 5, 9, 5)
	setSolid(ch, 6, 9, 6)
	setSolid(ch, 6, 9, 5)
	setSolid(ch, 5, 9, 6)

	diag := MoveWalkDiagonal{Dx: 1, Dz: 1}
	if cost := diag.Cost(w, from); cost != math.Sqrt2 {
		t.Errorf("expected cost sqrt(2) for valid diagonal walk, got %f", cost)
	}
	if yaw := diag.DesiredYaw(from); yaw == 0 {
		t.Errorf("expected non-zero yaw for diagonal movement")
	}

	// Block one adjacent cardinal cell to trigger corner-cutting rejection.
	setSolid(ch, 6, 10, 5)
	if cost := diag.Cost(w, from); cost != math.Inf(1) {
		t.Errorf("expected Inf for corner cutting, got %f", cost)
	}
}

func TestMoveParkour(t *testing.T) {
	w, ch := newTestWorld()
	from := [3]int{5, 10, 5}

	setSolid(ch, 5, 9, 5)
	setSolid(ch, 7, 9, 5) // landing floor

	parkour := MoveParkour{Dx: 1, Dz: 0}
	if cost := parkour.Cost(w, from); cost != 4.0 {
		t.Errorf("expected cost 4.0 for valid parkour, got %f", cost)
	}

	// Block mid-air headroom.
	setSolid(ch, 6, 11, 5)
	if cost := parkour.Cost(w, from); cost != math.Inf(1) {
		t.Errorf("expected Inf when mid headroom blocked, got %f", cost)
	}
}

func TestMoveClimb(t *testing.T) {
	w, ch := newTestWorld()
	from := [3]int{5, 10, 5}

	ch.SetBlock(5, 10, 5, world.BlockState{ID: 0x41B, Name: "minecraft:ladder"})
	setPassable(ch, 5, 11, 5)
	setPassable(ch, 5, 12, 5)

	climb := MoveClimb{}
	if cost := climb.Cost(w, from); math.Abs(cost-(1.0/0.1176)) > 1e-9 {
		t.Fatalf("expected climb cost %f, got %f", 1.0/0.1176, cost)
	}

	ch.SetBlock(5, 10, 5, world.BlockState{Name: "stone", Solid: true})
	if cost := climb.Cost(w, from); !math.IsInf(cost, 1) {
		t.Fatalf("expected Inf without climbable block, got %f", cost)
	}
}

func TestMoveSwim(t *testing.T) {
	w, ch := newTestWorld()
	from := [3]int{5, 10, 5}

	ch.SetBlock(5, 10, 5, world.BlockState{Name: "minecraft:water"})
	ch.SetBlock(6, 10, 5, world.BlockState{Name: "minecraft:water"})
	setPassable(ch, 6, 11, 5)

	swim := MoveSwim{Dx: 1, Dz: 0}
	if cost := swim.Cost(w, from); cost != 4.0 {
		t.Fatalf("expected swim cost 4.0, got %f", cost)
	}

	ch.SetBlock(6, 10, 5, world.BlockState{Name: "minecraft:air"})
	if cost := swim.Cost(w, from); !math.IsInf(cost, 1) {
		t.Fatalf("expected Inf when destination is not water, got %f", cost)
	}
}

func TestMoveSwimUp(t *testing.T) {
	w, ch := newTestWorld()
	from := [3]int{5, 10, 5}

	ch.SetBlock(5, 10, 5, world.BlockState{Name: "minecraft:water"})
	// Destination is one block up at (6,11,5) with solid bank under it.
	setSolid(ch, 6, 10, 5)
	setPassable(ch, 6, 11, 5)
	setPassable(ch, 6, 12, 5)

	swimUp := MoveSwimUp{Dx: 1, Dz: 0}
	if cost := swimUp.Cost(w, from); cost != 3.0 {
		t.Fatalf("expected swim-up cost 3.0, got %f", cost)
	}

	// No solid bank below destination should invalidate the move.
	setPassable(ch, 6, 10, 5)
	if cost := swimUp.Cost(w, from); !math.IsInf(cost, 1) {
		t.Fatalf("expected Inf when bank is missing, got %f", cost)
	}
}
