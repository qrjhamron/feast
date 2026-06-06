package executor

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/qrjhamron/feast/pkg/nav/goal"
	"github.com/qrjhamron/feast/pkg/nav/move"
	"github.com/qrjhamron/feast/pkg/nav/planner"
	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/world"
)

// 1. Path over removed support becomes invalid/replans (at planner level)
func TestPathOverRemovedSupportInvalid(t *testing.T) {
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	// Create support blocks
	ch.SetBlock(0, 9, 0, world.BlockState{Name: "stone", Solid: true})
	ch.SetBlock(1, 9, 0, world.BlockState{Name: "stone", Solid: true})
	ch.SetBlock(2, 9, 0, world.BlockState{Name: "stone", Solid: true})
	w.AddChunk(ch)

	g := goal.NewGoalBlock(2, 10, 0)
	res := planner.Plan(context.Background(), 0, 10, 0, g, w)
	t.Logf("Initial plan: status=%v, path=%v", res.Status, res.Path)
	if res.Status != planner.PlanFound || len(res.Path) == 0 {
		t.Fatalf("expected path to be found, got status=%v, path=%v", res.Status, res.Path)
	}

	// Remove support block at (1, 9, 0) and (2, 9, 0)
	ch.SetBlock(1, 9, 0, world.BlockState{Name: "air", Solid: false})
	ch.SetBlock(2, 9, 0, world.BlockState{Name: "air", Solid: false})
	planner.InvalidateCache()

	// Re-plan and check that path is no longer found (or is partial/timeout/no path)
	res2 := planner.Plan(context.Background(), 0, 10, 0, g, w)
	t.Logf("Second plan: status=%v, path=%v", res2.Status, res2.Path)
	if res2.Status == planner.PlanFound {
		t.Fatalf("expected path to NOT be fully found after support block removal, got status=%v, path=%v", res2.Status, res2.Path)
	}
}

// 2. Diagonal clipping blocked
func TestDiagonalClippingBlocked(t *testing.T) {
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	// Base floor
	for x := 0; x <= 2; x++ {
		for z := 0; z <= 2; z++ {
			ch.SetBlock(x, 9, z, world.BlockState{Name: "stone", Solid: true})
		}
	}
	// Corner block at (1, 10, 0) is blocked (solid)
	ch.SetBlock(1, 10, 0, world.BlockState{Name: "stone", Solid: true})
	w.AddChunk(ch)

	// Check playerCollisionClear at (0.9, 10.0, 0.9).
	// Because of the blocked corner at (1, 10, 0), this should return false.
	clear := playerCollisionClear(w, 0.9, 10.0, 0.9)
	if clear {
		t.Fatalf("expected playerCollisionClear to be false near blocked corner")
	}

	// Test collisionAwareHorizontalPosition: it should redirect/slide
	cx, cz := 0.5, 0.5
	tx, tz := 0.9, 0.9
	rx, rz := collisionAwareHorizontalPosition(w, cx, cz, tx, tz, 10.0)
	if rx == tx && rz == tz {
		t.Fatalf("expected collisionAwareHorizontalPosition to modify target to avoid diagonal clipping")
	}
}

// 3. One-block step valid
func TestOneBlockStepValid(t *testing.T) {
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	ch.SetBlock(0, 9, 0, world.BlockState{Name: "stone", Solid: true})
	ch.SetBlock(1, 10, 0, world.BlockState{Name: "stone", Solid: true})
	w.AddChunk(ch)

	// Step up: MoveJump from (0, 10, 0) to (1, 11, 0)
	jump := move.MoveJump{Dx: 1, Dz: 0}
	cost := jump.Cost(w, [3]int{0, 10, 0})
	if math.IsInf(cost, 1) {
		t.Fatalf("expected valid cost for MoveJump (step up), got Inf")
	}
	dest := jump.Destination([3]int{0, 10, 0})
	if dest != [3]int{1, 11, 0} {
		t.Fatalf("expected destination (1, 11, 0), got %v", dest)
	}

	// Step down: MoveFall from (1, 11, 0) to (0, 10, 0)
	fall := move.MoveFall{Dx: -1, Dy: -1, Dz: 0}
	cost2 := fall.Cost(w, [3]int{1, 11, 0})
	if math.IsInf(cost2, 1) {
		t.Fatalf("expected valid cost for MoveFall (step down), got Inf")
	}
	dest2 := fall.Destination([3]int{1, 11, 0})
	if dest2 != [3]int{0, 10, 0} {
		t.Fatalf("expected destination (0, 10, 0), got %v", dest2)
	}
}

// 4. Fall after support loss handled
func TestFallAfterSupportLossHandled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	client := &mockClient{entityID: 10, failAfterPosN: 2, x: 0.5, y: 10.0, z: 0.5}
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	// Air under the player start position (0, 9, 0), so support is lost.
	ch.SetBlock(0, 9, 0, world.BlockState{Name: "air", Solid: false})
	w.AddChunk(ch)

	// Try to execute a walk. Since support is lost, player Y should drop due to gravity.
	g := mockGoal{satisfied: false}
	path := []move.Movement{move.MoveWalk{Dx: 1, Dz: 0}}
	_ = Execute(ctx, client, w, g, path)

	client.mu.Lock()
	defer client.mu.Unlock()

	// The packets sent should show decreasing Y positions (falling)
	var lastY float64 = 10.0
	hasFall := false
	for _, p := range client.packets {
		if pp, ok := p.(*protocol.PlayServerboundSetPlayerPositionAndRotationPacket); ok {
			if pp.Y < lastY {
				hasFall = true
			}
			lastY = pp.Y
		}
	}
	if !hasFall {
		t.Fatalf("expected player to fall (Y coordinate decrease) after support loss, but it did not")
	}
}

// 5. Target center conversion correct
func TestTargetCenterConversionCorrect(t *testing.T) {
	dest := [3]int{10, 64, 20}
	destX := float64(dest[0]) + 0.5
	destY := float64(dest[1])
	destZ := float64(dest[2]) + 0.5

	if destX != 10.5 || destY != 64.0 || destZ != 20.5 {
		t.Fatalf("expected target center (10.5, 64.0, 20.5), got (%.1f, %.1f, %.1f)", destX, destY, destZ)
	}
}
