package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qrjhamron/feast/pkg/nav/goal"
	"github.com/qrjhamron/feast/pkg/nav/move"
	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/world"
)

// correctionClient bumps its position-sync sequence once after a given number
// of position packets, simulating a server-side movement correction without
// teleporting the bot away (so the movement still completes). It captures the
// final MovementStats via TrackMovementStats.
type correctionClient struct {
	mockClient
	seq          uint64
	correctAfter int
	corrected    bool
	stats        MovementStats
}

func (c *correctionClient) PositionSyncSeq() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.seq
}

func (c *correctionClient) TrackMovementStats(s MovementStats) { c.stats = s }

func (c *correctionClient) WritePacket(p protocol.Packet) error {
	err := c.mockClient.WritePacket(p)
	c.mu.Lock()
	if !c.corrected && c.posPktCount >= c.correctAfter {
		c.corrected = true
		c.seq++ // server acknowledges a correction
	}
	c.mu.Unlock()
	return err
}

func flatGroundWorld(maxX int) *world.World {
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	for x := 0; x <= maxX; x++ {
		for z := 0; z <= 2; z++ {
			ch.SetBlock(x, 63, z, world.BlockState{Name: "stone", Solid: true})
		}
	}
	w.AddChunk(ch)
	return w
}

func TestExecutorAdjacentStepFlat(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client := &mockClient{x: 0.5, y: 64, z: 0.5}
	w := flatGroundWorld(3)

	stats, err := executeAdjacentStep(ctx, client, w, [3]int{1, 64, 0}, MovementOptions{})
	if err != nil {
		t.Fatalf("adjacent step failed: %v", err)
	}
	if !stats.Reached || stats.Noop {
		t.Fatalf("expected reached non-noop, got %+v", stats)
	}
	if stats.PacketsSent == 0 {
		t.Fatal("expected position packets to be sent")
	}
	if stats.FinalDistance > 0.35 {
		t.Fatalf("expected to finish within 0.35 of the target, got %.3f", stats.FinalDistance)
	}
}

func TestExecutorAdjacentStepNoop(t *testing.T) {
	client := &mockClient{x: 5.5, y: 64, z: 5.5}
	w := flatGroundWorld(8)
	stats, err := executeAdjacentStep(context.Background(), client, w, [3]int{5, 64, 5}, MovementOptions{})
	if err != nil {
		t.Fatalf("noop step errored: %v", err)
	}
	if !stats.Noop || !stats.Reached || stats.PacketsSent != 0 {
		t.Fatalf("expected zero-packet noop, got %+v", stats)
	}
}

func TestExecutorStepUpOneBlock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client := &mockClient{x: 0.5, y: 64, z: 0.5}
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	ch.SetBlock(0, 63, 0, world.BlockState{Name: "stone", Solid: true}) // start floor
	ch.SetBlock(1, 63, 0, world.BlockState{Name: "stone", Solid: true})
	ch.SetBlock(1, 64, 0, world.BlockState{Name: "stone", Solid: true}) // one-block step up
	w.AddChunk(ch)

	stats, err := executeAdjacentStep(ctx, client, w, [3]int{1, 65, 0}, MovementOptions{})
	if err != nil {
		t.Fatalf("step up failed: %v", err)
	}
	if !stats.Reached || stats.PacketsSent == 0 {
		t.Fatalf("expected reached step-up with packets, got %+v", stats)
	}
}

func TestExecutorStepDownOneBlock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client := &mockClient{x: 0.5, y: 64, z: 0.5}
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	ch.SetBlock(0, 63, 0, world.BlockState{Name: "stone", Solid: true}) // start floor
	ch.SetBlock(1, 62, 0, world.BlockState{Name: "stone", Solid: true}) // one-block step down floor
	w.AddChunk(ch)

	stats, err := executeAdjacentStep(ctx, client, w, [3]int{1, 63, 0}, MovementOptions{})
	if err != nil {
		t.Fatalf("step down failed: %v", err)
	}
	if !stats.Reached || stats.PacketsSent == 0 {
		t.Fatalf("expected reached step-down with packets, got %+v", stats)
	}
}

func TestExecutorRejectsTwoBlockStepUp(t *testing.T) {
	client := &mockClient{x: 0.5, y: 64, z: 0.5}
	w := flatGroundWorld(3)

	stats, err := executeAdjacentStep(context.Background(), client, w, [3]int{1, 66, 0}, MovementOptions{})
	if err == nil {
		t.Fatal("expected error for a non-adjacent (two-block) step up")
	}
	if !errors.Is(err, errMovementStuck) {
		t.Fatalf("expected errMovementStuck, got %v", err)
	}
	if stats.PacketsSent != 0 {
		t.Fatalf("non-adjacent target must be rejected before sending packets, got %d", stats.PacketsSent)
	}
}

func TestExecutorAdjacentStepContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	client := &mockClient{x: 0.5, y: 64, z: 0.5}
	w := flatGroundWorld(3)

	_, err := executeAdjacentStep(ctx, client, w, [3]int{1, 64, 0}, MovementOptions{})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

// TestExecutorOnGroundFlagWhileFalling verifies on_ground is reported false on
// every packet while the bot is descending through air (fractional Y), which is
// what prevents Paper from issuing movement corrections.
func TestExecutorOnGroundFlagWhileFalling(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client := &mockClient{entityID: 7, x: 0.5, y: 20, z: 0.5, failAfterPosN: 4}
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	// Chunk loaded but entirely air under the bot -> genuine free fall.
	ch.SetBlock(0, 10, 0, world.BlockState{Name: "air"})
	w.AddChunk(ch)

	_ = Execute(ctx, client, w, mockGoal{satisfied: false}, []move.Movement{move.MoveWalk{Dx: 1, Dz: 0}})

	client.mu.Lock()
	defer client.mu.Unlock()
	var sawPacket bool
	for _, p := range client.packets {
		pp, ok := p.(*protocol.PlayServerboundSetPlayerPositionAndRotationPacket)
		if !ok {
			continue
		}
		sawPacket = true
		if pp.OnGround {
			t.Fatalf("on_ground must be false while falling, got true at y=%.3f", pp.Y)
		}
	}
	if !sawPacket {
		t.Fatal("expected at least one position packet")
	}
}

// TestExecutorCorrectionCounts verifies that a server position-sync bump during
// movement is counted as a correction in the stats.
func TestExecutorCorrectionCounts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client := &correctionClient{
		mockClient:   mockClient{entityID: 9, x: 0.5, y: 64, z: 0.5},
		correctAfter: 3,
	}
	w := flatGroundWorld(4)

	// A high-cost move guarantees enough ticks for the correction to register.
	_ = Execute(ctx, client, w, mockGoal{satisfied: false}, []move.Movement{highCostMove{cost: 8}})

	if client.stats.CorrectionsSeen < 1 {
		t.Fatalf("expected at least one correction counted, got %d", client.stats.CorrectionsSeen)
	}
	if client.stats.ServerPositionsSeen < 1 {
		t.Fatalf("expected server positions seen >= 1, got %d", client.stats.ServerPositionsSeen)
	}
}

// TestExecutorStatsNotStaleAfterFailure ensures a failed segment does not leave
// Reached=true and records a StuckReason — the stats must reflect the failure.
func TestExecutorStatsNotStaleAfterFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Frozen position -> never advances -> stuck failure.
	client := &statsClient{mockClient: mockClient{entityID: 11, x: 0.5, y: 64, z: 0.5, freezePosN: 999}}
	w := flatGroundWorld(3)

	err := Execute(ctx, client, w, mockGoal{satisfied: false}, []move.Movement{move.MoveWalk{Dx: 1, Dz: 0}})
	if err == nil {
		t.Fatal("expected a stuck failure")
	}
	if client.stats.Reached {
		t.Fatal("stats.Reached must be false after a failure (not stale)")
	}
	if client.stats.StuckReason == "" {
		t.Fatal("expected a non-empty StuckReason after failure")
	}
	if !errors.Is(err, errMovementStuck) {
		t.Fatalf("expected errMovementStuck, got %v", err)
	}
}

// TestExecutorSentinelClassification confirms ExecuteWithResult classifies the
// outcome via sentinel identity rather than fragile substring matching.
func TestExecutorSentinelClassification(t *testing.T) {
	w := flatGroundWorld(3)

	// No path -> MoveNoPath + errMovementNoPath.
	res, err := ExecuteWithResult(context.Background(), &mockClient{}, w, mockGoal{satisfied: false}, nil)
	if !errors.Is(err, errMovementNoPath) || res.Reason != MoveNoPath {
		t.Fatalf("expected no-path sentinel, got reason=%v err=%v", res.Reason, err)
	}

	// Stuck -> MoveStuck.
	stuckClient := &statsClient{mockClient: mockClient{x: 0.5, y: 64, z: 0.5, freezePosN: 999}}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	res, err = ExecuteWithResult(ctx, stuckClient, w, mockGoal{satisfied: false}, []move.Movement{move.MoveWalk{Dx: 1, Dz: 0}})
	if res.Reason != MoveStuck || !errors.Is(err, errMovementStuck) {
		t.Fatalf("expected stuck classification, got reason=%v err=%v", res.Reason, err)
	}

	// Timeout -> MoveTimeout.
	tctx, tcancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer tcancel()
	res, err = ExecuteWithResult(tctx, &mockClient{x: 0.5, y: 64, z: 0.5}, w, mockGoal{satisfied: false}, []move.Movement{mockMove{}})
	if res.Reason != MoveTimeout {
		t.Fatalf("expected timeout classification, got reason=%v err=%v", res.Reason, err)
	}
}

// TestExecutorAdjacentStepGoal sanity-checks that the helper drives a concrete
// GoalBlock-style target center for the simplest scaffold/tunnel use case.
func TestExecutorAdjacentStepGoal(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client := &mockClient{x: 2.5, y: 64, z: 2.5}
	w := flatGroundWorld(5)
	// Step toward +z neighbor.
	stats, err := executeAdjacentStep(ctx, client, w, [3]int{2, 64, 3}, MovementOptions{})
	if err != nil {
		t.Fatalf("adjacent +z step failed: %v", err)
	}
	if !stats.Reached {
		t.Fatalf("expected reached, got %+v", stats)
	}
	_ = goal.NewGoalBlock(2, 64, 3) // documents the intended caller goal shape
}
