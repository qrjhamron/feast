package executor

import (
	"context"
	"testing"

	"github.com/qrjhamron/feast/pkg/nav/goal"
	"github.com/qrjhamron/feast/pkg/nav/move"
	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/world"
)

// TestMovementPacketsUseGroundFlagCorrectly verifies that a flat walk across
// solid ground reports on_ground=true on every position packet (feet rest on
// the block top), which is what keeps Paper from issuing movement corrections.
func TestMovementPacketsUseGroundFlagCorrectly(t *testing.T) {
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	// Solid floor at y=63 spanning the walk lane.
	for x := 0; x <= 4; x++ {
		ch.SetBlock(x, 63, 0, world.BlockState{Name: "stone", Solid: true})
	}
	w.AddChunk(ch)

	client := &mockClient{x: 0.5, y: 64.0, z: 0.5}
	g := goal.NewGoalBlock(3, 64, 0)
	path := []move.Movement{
		move.MoveWalk{Dx: 1, Dz: 0},
		move.MoveWalk{Dx: 1, Dz: 0},
		move.MoveWalk{Dx: 1, Dz: 0},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5e9)
	defer cancel()
	_ = Execute(ctx, client, w, g, path)

	client.mu.Lock()
	defer client.mu.Unlock()
	if client.posPktCount == 0 {
		t.Fatal("expected at least one position packet")
	}
	for i, p := range client.packets {
		pp, ok := p.(*protocol.PlayServerboundSetPlayerPositionAndRotationPacket)
		if !ok {
			continue
		}
		// Feet stay at integer y=64 on a flat lane → must be on_ground.
		if !pp.OnGround {
			t.Fatalf("packet %d at y=%.3f reported on_ground=false on flat ground", i, pp.Y)
		}
	}
}
