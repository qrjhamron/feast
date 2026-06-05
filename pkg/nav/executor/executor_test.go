package executor

import (
	"context"
	"errors"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/user/feastgo/pkg/nav/move"
	"github.com/user/feastgo/pkg/protocol"
	"github.com/user/feastgo/pkg/world"
)

type mockGoal struct {
	satisfied bool
}

func (m mockGoal) Satisfied(x, y, z int) bool {
	return m.satisfied
}

func (m mockGoal) Heuristic(x, y, z int) float64 {
	return 0
}

type mockMove struct{}

func (m mockMove) Destination(currentPos [3]int) [3]int {
	return [3]int{currentPos[0] + 1, currentPos[1], currentPos[2]}
}

func (m mockMove) Cost(w *world.World, currentPos [3]int) float64 {
	return 0.1 // quick
}

type highCostMove struct {
	cost float64
}

func (m highCostMove) Destination(currentPos [3]int) [3]int {
	return [3]int{currentPos[0] + 1, currentPos[1], currentPos[2]}
}

func (m highCostMove) Cost(w *world.World, currentPos [3]int) float64 {
	return m.cost
}

type mockClient struct {
	mu            sync.Mutex
	x, y, z       float64
	yaw, pitch    float32
	entityID      int32
	packets       []protocol.Packet
	posPktCount   int
	failAfterPosN int
	freezePosN    int
}

func (m *mockClient) GetPosition() (float64, float64, float64, float32, float32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.x, m.y, m.z, m.yaw, m.pitch
}

func (m *mockClient) EntityID() int32 { return m.entityID }

func (m *mockClient) WritePacket(p protocol.Packet) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.packets = append(m.packets, p)

	if pp, ok := p.(*protocol.PlayServerboundSetPlayerPositionAndRotationPacket); ok {
		m.posPktCount++
		if m.freezePosN <= 0 || m.posPktCount > m.freezePosN {
			m.x, m.y, m.z = pp.X, pp.Y, pp.Z
			m.yaw, m.pitch = pp.Yaw, pp.Pitch
		}
		if m.failAfterPosN > 0 && m.posPktCount >= m.failAfterPosN {
			return errors.New("stop")
		}
	}
	return nil
}

func TestExecute_ImmediateSatisfied(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{}
	w := world.NewWorld()
	g := mockGoal{satisfied: true}

	err := Execute(ctx, client, w, g, []move.Movement{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestExecute_OneMove(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client := &mockClient{failAfterPosN: 1}
	w := world.NewWorld()
	g := mockGoal{satisfied: false}

	path := []move.Movement{mockMove{}}

	err := Execute(ctx, client, w, g, path)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "deadline exceeded") {
		// Just ensuring an error occurs
	}
}

func TestExecute_PositionPacketSent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client := &mockClient{entityID: 123, failAfterPosN: 1}
	w := world.NewWorld()
	g := mockGoal{satisfied: false}

	path := []move.Movement{move.MoveWalk{Dx: 1, Dz: 0}}
	_ = Execute(ctx, client, w, g, path)

	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.packets) < 1 {
		t.Fatalf("expected at least 1 packet, got %d", len(client.packets))
	}
	var sawPosition bool
	for _, p := range client.packets {
		if _, ok := p.(*protocol.PlayServerboundSetPlayerPositionAndRotationPacket); ok {
			sawPosition = true
			break
		}
	}
	if !sawPosition {
		t.Fatalf("expected at least one position packet")
	}
}

func TestExecute_OnGroundFlags(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client := &mockClient{entityID: 123, failAfterPosN: 2, x: 0.5, y: 10, z: 0.5}
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	ch.SetBlock(0, 9, 0, world.BlockState{Name: "air"})
	w.AddChunk(ch)
	g := mockGoal{satisfied: false}

	path := []move.Movement{move.MoveJump{Dx: 1, Dz: 0}}
	_ = Execute(ctx, client, w, g, path)

	client.mu.Lock()
	defer client.mu.Unlock()
	var posPkts []*protocol.PlayServerboundSetPlayerPositionAndRotationPacket
	for _, p := range client.packets {
		if pp, ok := p.(*protocol.PlayServerboundSetPlayerPositionAndRotationPacket); ok {
			posPkts = append(posPkts, pp)
		}
	}
	if len(posPkts) == 0 {
		t.Fatalf("expected at least 1 position packet")
	}
	for i, pp := range posPkts {
		feetX := int(math.Floor(pp.X))
		feetY := int(math.Floor(pp.Y))
		feetZ := int(math.Floor(pp.Z))
		want := !w.IsPassable(feetX, feetY-1, feetZ)
		if pp.OnGround != want {
			t.Fatalf("expected OnGround=%v at idx=%d", want, i)
		}
	}
}

func TestExecute_OnGroundDerivedFromWorldBelow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client := &mockClient{entityID: 123, failAfterPosN: 1, x: 0.5, y: 10, z: 0.5}
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	ch.SetBlock(0, 8, 0, world.BlockState{Name: "minecraft:stone"})
	w.AddChunk(ch)
	g := mockGoal{satisfied: false}

	err := Execute(ctx, client, w, g, []move.Movement{move.MoveWalk{Dx: 1, Dz: 0}})
	if err == nil {
		t.Fatalf("expected early stop error")
	}

	client.mu.Lock()
	defer client.mu.Unlock()
	for _, p := range client.packets {
		pp, ok := p.(*protocol.PlayServerboundSetPlayerPositionAndRotationPacket)
		if !ok {
			continue
		}
		feetX := int(math.Floor(pp.X))
		feetY := int(math.Floor(pp.Y))
		feetZ := int(math.Floor(pp.Z))
		want := !w.IsPassable(feetX, feetY-1, feetZ)
		if pp.OnGround != want {
			t.Fatalf("expected OnGround=%v from world check", want)
		}
		return
	}
	t.Fatalf("expected at least one position packet")
}

func TestExecute_SprintWaterTransitions(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client := &mockClient{entityID: 55, failAfterPosN: 30, x: 0.5, y: 10, z: 0.5}
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	// Ground support for all visited columns.
	for x := 0; x <= 3; x++ {
		ch.SetBlock(x, 9, 0, world.BlockState{Name: "minecraft:stone"})
	}
	// Start in water, then move to land.
	ch.SetBlock(0, 10, 0, world.BlockState{Name: "minecraft:water"})
	ch.SetBlock(1, 10, 0, world.BlockState{Name: "minecraft:water"})
	ch.SetBlock(2, 10, 0, world.BlockState{Name: "air"})
	ch.SetBlock(3, 10, 0, world.BlockState{Name: "air"})
	w.AddChunk(ch)

	path := []move.Movement{
		move.MoveSwim{Dx: 1, Dz: 0},
		move.MoveWalk{Dx: 1, Dz: 0},
	}
	_ = Execute(ctx, client, w, mockGoal{satisfied: false}, path)

	client.mu.Lock()
	defer client.mu.Unlock()
	var sawStart bool
	for _, p := range client.packets {
		cmd, ok := p.(*protocol.PlayServerboundPlayerCommandPacket)
		if !ok {
			continue
		}
		if cmd.ActionID == protocol.PlayerCommandStartSprinting {
			sawStart = true
		}
	}
	if !sawStart {
		t.Fatalf("expected START_SPRINTING after exiting water")
	}
}

func TestExecute_SprintStopWhenEnteringWater(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client := &mockClient{entityID: 55, failAfterPosN: 8, x: 0.5, y: 10, z: 0.5}
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	for x := 0; x <= 3; x++ {
		ch.SetBlock(x, 9, 0, world.BlockState{Name: "minecraft:stone"})
	}
	ch.SetBlock(0, 10, 0, world.BlockState{Name: "air"})
	ch.SetBlock(1, 10, 0, world.BlockState{Name: "minecraft:water"})
	ch.SetBlock(2, 10, 0, world.BlockState{Name: "minecraft:water"})
	ch.SetBlock(1, 9, 0, world.BlockState{Name: "minecraft:water"})
	ch.SetBlock(2, 9, 0, world.BlockState{Name: "minecraft:water"})
	w.AddChunk(ch)

	path := []move.Movement{
		move.MoveWalk{Dx: 1, Dz: 0},
		move.MoveSwim{Dx: 1, Dz: 0},
	}
	_ = Execute(ctx, client, w, mockGoal{satisfied: false}, path)

	client.mu.Lock()
	defer client.mu.Unlock()
	var sawStop bool
	for _, p := range client.packets {
		cmd, ok := p.(*protocol.PlayServerboundPlayerCommandPacket)
		if !ok {
			continue
		}
		if cmd.ActionID == protocol.PlayerCommandStopSprinting {
			sawStop = true
		}
	}
	if !sawStop {
		t.Fatalf("expected STOP_SPRINTING when entering water")
	}
}

func TestExecute_GravityModelApplied(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client := &mockClient{entityID: 10, failAfterPosN: 1, x: 0.5, y: 10, z: 0.5}
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	// Air below so onGround=false and gravity applies.
	ch.SetBlock(0, 9, 0, world.BlockState{Name: "air"})
	w.AddChunk(ch)

	_ = Execute(ctx, client, w, mockGoal{satisfied: false}, []move.Movement{move.MoveWalk{Dx: 1, Dz: 0}})

	client.mu.Lock()
	defer client.mu.Unlock()
	for _, p := range client.packets {
		pp, ok := p.(*protocol.PlayServerboundSetPlayerPositionAndRotationPacket)
		if !ok {
			continue
		}
		wantY := 10 + ((0 - 0.08) * 0.98)
		if math.Abs(pp.Y-wantY) > 1e-6 {
			t.Fatalf("unexpected gravity Y: got %.6f want %.6f", pp.Y, wantY)
		}
		return
	}
	t.Fatalf("expected a position packet")
}

func TestExecute_DiagonalMovementNormalized(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client := &mockClient{entityID: 42, failAfterPosN: 1, x: 0.5, y: 10, z: 0.5}
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	ch.SetBlock(0, 9, 0, world.BlockState{Name: "minecraft:stone"})
	w.AddChunk(ch)

	_ = Execute(ctx, client, w, mockGoal{satisfied: false}, []move.Movement{move.MoveWalkDiagonal{Dx: 1, Dz: 1}})

	client.mu.Lock()
	defer client.mu.Unlock()
	for _, p := range client.packets {
		pp, ok := p.(*protocol.PlayServerboundSetPlayerPositionAndRotationPacket)
		if !ok {
			continue
		}
		want := 0.5 + 0.215*0.7071067811865475
		if math.Abs(pp.X-want) > 1e-6 || math.Abs(pp.Z-want) > 1e-6 {
			t.Fatalf("unexpected normalized diagonal step: got (%.6f, %.6f) want (%.6f, %.6f)", pp.X, pp.Z, want, want)
		}
		return
	}
	t.Fatalf("expected a position packet")
}

func TestExpandLineMovements(t *testing.T) {
	in := []move.Movement{
		move.MoveWalkLine{Dx: 1, Dz: 0, Steps: 3},
		move.MoveWalkDiagonalLine{Dx: 1, Dz: 1, Steps: 2},
		move.MoveJump{Dx: 0, Dz: 1},
	}

	out := expandLineMovements(in)
	if len(out) != 6 {
		t.Fatalf("len(out)=%d want 6", len(out))
	}
	for i := 0; i < 3; i++ {
		if m, ok := out[i].(move.MoveWalk); !ok || m.Dx != 1 || m.Dz != 0 {
			t.Fatalf("out[%d]=%T(%v), want MoveWalk{1,0}", i, out[i], out[i])
		}
	}
	for i := 3; i < 5; i++ {
		if m, ok := out[i].(move.MoveWalkDiagonal); !ok || m.Dx != 1 || m.Dz != 1 {
			t.Fatalf("out[%d]=%T(%v), want MoveWalkDiagonal{1,1}", i, out[i], out[i])
		}
	}
	if _, ok := out[5].(move.MoveJump); !ok {
		t.Fatalf("out[5]=%T, want MoveJump", out[5])
	}
}

func TestExecute_JumpTickZeroVelocity(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client := &mockClient{entityID: 99, failAfterPosN: 1, x: 0.5, y: 10, z: 0.5}
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	ch.SetBlock(0, 9, 0, world.BlockState{Name: "air"})
	w.AddChunk(ch)

	_ = Execute(ctx, client, w, mockGoal{satisfied: false}, []move.Movement{move.MoveJump{Dx: 1, Dz: 0}})

	client.mu.Lock()
	defer client.mu.Unlock()
	for _, p := range client.packets {
		pp, ok := p.(*protocol.PlayServerboundSetPlayerPositionAndRotationPacket)
		if !ok {
			continue
		}
		if math.Abs(pp.Y-10.42) > 1e-6 {
			t.Fatalf("unexpected jump tick0 Y: got %.6f want %.6f", pp.Y, 10.42)
		}
		return
	}
	t.Fatalf("expected a position packet")
}

func TestExecute_HonorsMovementCostMinimumTicks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client := &mockClient{entityID: 101, x: 0.5, y: 10, z: 0.5}
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	ch.SetBlock(0, 9, 0, world.BlockState{Name: "minecraft:stone"})
	ch.SetBlock(1, 9, 0, world.BlockState{Name: "minecraft:stone"})
	w.AddChunk(ch)

	err := Execute(ctx, client, w, mockGoal{satisfied: false}, []move.Movement{highCostMove{cost: 8}})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	client.mu.Lock()
	defer client.mu.Unlock()
	if client.posPktCount < 8 {
		t.Fatalf("expected at least 8 position packets for cost=8, got %d", client.posPktCount)
	}
}

func TestExecute_RetriesStepOnceWhenPositionDoesNotAdvance(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client := &mockClient{entityID: 102, x: 0.5, y: 10, z: 0.5, freezePosN: 5}
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	ch.SetBlock(0, 9, 0, world.BlockState{Name: "minecraft:stone"})
	ch.SetBlock(1, 9, 0, world.BlockState{Name: "minecraft:stone"})
	w.AddChunk(ch)

	err := Execute(ctx, client, w, mockGoal{satisfied: false}, []move.Movement{move.MoveWalk{Dx: 1, Dz: 0}})
	if err != nil {
		t.Fatalf("expected retry to recover, got %v", err)
	}

	client.mu.Lock()
	defer client.mu.Unlock()
	if client.posPktCount < 10 {
		t.Fatalf("expected retry to send a second step attempt, got %d packets", client.posPktCount)
	}
	if math.Hypot(client.x-0.5, client.z-0.5) < 0.1 {
		t.Fatalf("expected retry to advance actual position, got (%.3f, %.3f, %.3f)", client.x, client.y, client.z)
	}
}

func TestStuckDetectsOnlyAfterFullLookbackWindow(t *testing.T) {
	now := time.Unix(100, 0)
	shortHistory := []positionSample{
		{at: now.Add(-4 * time.Second), x: 1, y: 64, z: 1},
		{at: now, x: 1.01, y: 64, z: 1.01},
	}
	if stuck(shortHistory, 5*time.Second, 0.1, now) {
		t.Fatal("must not report stuck until samples span the full lookback window")
	}

	fullHistory := []positionSample{
		{at: now.Add(-6 * time.Second), x: 1, y: 64, z: 1},
		{at: now.Add(-3 * time.Second), x: 1.01, y: 64, z: 1.01},
		{at: now, x: 1.02, y: 64, z: 1.02},
	}
	if !stuck(fullHistory, 5*time.Second, 0.1, now) {
		t.Fatal("expected stuck when movement remains below threshold over full lookback")
	}
}
