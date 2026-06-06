package executor

import (
	"context"
	"testing"
	"time"

	"github.com/qrjhamron/feast/pkg/nav/goal"
	"github.com/qrjhamron/feast/pkg/nav/move"
	"github.com/qrjhamron/feast/pkg/world"
)

// TestNoopStepLoggedAsNoop verifies a segment that starts already at the goal
// finishes successfully without sending packets and is flagged as a no-op.
func TestNoopStepLoggedAsNoop(t *testing.T) {
	client := &statsClient{}
	w := world.NewWorld()
	// Goal already satisfied at start.
	g := mockGoal{satisfied: true}

	err := Execute(context.Background(), client, w, g, []move.Movement{move.MoveWalk{Dx: 1, Dz: 0}})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !client.stats.Noop {
		t.Fatal("expected Noop=true for an already-satisfied segment")
	}
	if client.stats.PacketsSent != 0 {
		t.Fatalf("expected no packets for a no-op, got %d", client.stats.PacketsSent)
	}
}

// TestMoveStepReachedDoesNotMeanRouteReached verifies that consuming a path
// without satisfying the goal reports step success (Reached) but a false
// GoalSatisfied — the route is not actually reached.
func TestMoveStepReachedDoesNotMeanRouteReached(t *testing.T) {
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	for x := 0; x <= 3; x++ {
		ch.SetBlock(x, 63, 0, world.BlockState{Name: "stone", Solid: true})
	}
	w.AddChunk(ch)

	client := &statsClient{}
	client.x, client.y, client.z = 0.5, 64.0, 0.5
	// A concrete goal far from where the short path ends, so it is never met.
	g := goal.NewGoalBlock(50, 64, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = Execute(ctx, client, w, g, []move.Movement{move.MoveWalk{Dx: 1, Dz: 0}})

	if client.stats.GoalSatisfied {
		t.Fatal("goal must not be reported satisfied when the bot is far from goal")
	}
}
