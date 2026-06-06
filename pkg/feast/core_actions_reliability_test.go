package feast

import (
	"bytes"
	"context"
	"errors"
	"math"
	"net"
	"testing"
	"time"

	feastconn "github.com/qrjhamron/feast/pkg/conn"
	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/state"
	"github.com/qrjhamron/feast/pkg/world"
)

func readyCoreActionClient(t *testing.T) (*Client, net.Conn) {
	t.Helper()
	c := NewClient(Options{})
	markCoreActionReady(t, c)

	a, b := net.Pipe()
	c.conn = feastconn.New(a)
	t.Cleanup(func() {
		_ = a.Close()
		_ = b.Close()
	})
	go func() {
		buf := make([]byte, 4096)
		for {
			if _, err := b.Read(buf); err != nil {
				return
			}
		}
	}()
	return c, b
}

func markCoreActionReady(t *testing.T, c *Client) {
	t.Helper()
	if err := c.fsm.Transition(state.StateLogin); err != nil {
		t.Fatal(err)
	}
	if err := c.fsm.Transition(state.StateConfiguration); err != nil {
		t.Fatal(err)
	}
	if err := c.fsm.Transition(state.StatePlay); err != nil {
		t.Fatal(err)
	}
	c.stateMu.Lock()
	c.positionSynced = true
	c.player.X = 0.5
	c.player.Y = 64
	c.player.Z = 0.5
	c.stateMu.Unlock()
}

func addFlatActionChunk(c *Client) {
	ch := world.NewChunk(0, 0)
	for x := 0; x < world.ChunkWidth; x++ {
		for z := 0; z < world.ChunkDepth; z++ {
			ch.SetBlock(x, 63, z, world.BlockState{Name: "stone", Solid: true, ID: 1})
			ch.SetBlock(x, 64, z, world.BlockState{Name: "air", ID: 0})
			ch.SetBlock(x, 65, z, world.BlockState{Name: "air", ID: 0})
		}
	}
	c.world.AddChunk(ch)
}

func TestPlaceBlockSurvivalReliabilityErrors(t *testing.T) {
	t.Run("short_grass_replaceable_succeeds", func(t *testing.T) {
		c, _ := readyCoreActionClient(t)
		addFlatActionChunk(c)
		c.stateMu.Lock()
		c.player.X = 5.5
		c.player.Z = 5.5
		c.stateMu.Unlock()
		grassID, ok := world.FindAnyStateIDByName("short_grass")
		if !ok {
			t.Fatal("missing short_grass state")
		}
		c.world.SetBlock(2, 64, 2, uint16(grassID))
		c.trackInventorySlot(36, protocol.ItemStack{Present: true, ItemID: 1, Count: 4})
		c.trackSelectedHotbarSlot(0)
		go func() {
			time.Sleep(25 * time.Millisecond)
			c.world.SetBlock(2, 64, 2, 1)
			c.bus.Emit(state.BlockUpdateEvent{X: 2, Y: 64, Z: 2, StateID: 1})
			c.bus.Emit(state.InventorySlotEvent{WindowID: 0, Slot: 36, Item: protocol.ItemStack{Present: true, ItemID: 1, Count: 3}})
		}()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := c.PlaceBlockSurvival(ctx, protocol.BlockPos{X: 2, Y: 64, Z: 2}, protocol.DirectionUp); err != nil {
			t.Fatalf("PlaceBlockSurvival short_grass: %v", err)
		}
	})

	t.Run("stone_fails_not_replaceable", func(t *testing.T) {
		c, _ := readyCoreActionClient(t)
		addFlatActionChunk(c)
		c.world.SetBlock(2, 64, 2, 1)
		c.trackInventorySlot(36, protocol.ItemStack{Present: true, ItemID: 1, Count: 4})
		err := c.PlaceBlockSurvival(context.Background(), protocol.BlockPos{X: 2, Y: 64, Z: 2}, protocol.DirectionUp)
		if !errors.Is(err, ErrBlockNotReplaceable) {
			t.Fatalf("error=%v want ErrBlockNotReplaceable", err)
		}
	})

	t.Run("unloaded_target_fails", func(t *testing.T) {
		c, _ := readyCoreActionClient(t)
		c.trackInventorySlot(36, protocol.ItemStack{Present: true, ItemID: 1, Count: 4})
		err := c.PlaceBlockSurvival(context.Background(), protocol.BlockPos{X: -1, Y: 64, Z: -1}, protocol.DirectionUp)
		if !errors.Is(err, ErrChunkNotLoaded) {
			t.Fatalf("error=%v want ErrChunkNotLoaded", err)
		}
	})

	t.Run("no_support_fails", func(t *testing.T) {
		c, _ := readyCoreActionClient(t)
		addFlatActionChunk(c)
		c.world.SetBlock(2, 63, 2, 0)
		c.trackInventorySlot(36, protocol.ItemStack{Present: true, ItemID: 1, Count: 4})
		err := c.PlaceBlockSurvival(context.Background(), protocol.BlockPos{X: 2, Y: 64, Z: 2}, protocol.DirectionUp)
		if !errors.Is(err, ErrNoSupportBlock) {
			t.Fatalf("error=%v want ErrNoSupportBlock", err)
		}
	})

	t.Run("out_of_reach_fails", func(t *testing.T) {
		c, _ := readyCoreActionClient(t)
		addFlatActionChunk(c)
		c.trackInventorySlot(36, protocol.ItemStack{Present: true, ItemID: 1, Count: 4})
		err := c.PlaceBlockSurvival(context.Background(), protocol.BlockPos{X: 10, Y: 64, Z: 10}, protocol.DirectionUp)
		if !errors.Is(err, ErrTargetOutOfReach) {
			t.Fatalf("error=%v want ErrTargetOutOfReach", err)
		}
	})

	t.Run("overlap_bot_fails", func(t *testing.T) {
		c, _ := readyCoreActionClient(t)
		addFlatActionChunk(c)
		c.trackInventorySlot(36, protocol.ItemStack{Present: true, ItemID: 1, Count: 4})
		err := c.PlaceBlockSurvival(context.Background(), protocol.BlockPos{X: 0, Y: 64, Z: 0}, protocol.DirectionUp)
		if !errors.Is(err, ErrBlockNotReplaceable) && err == nil {
			t.Fatalf("expected placement preflight error, got %v", err)
		}
	})

	t.Run("rollback_detected", func(t *testing.T) {
		c, _ := readyCoreActionClient(t)
		addFlatActionChunk(c)
		c.stateMu.Lock()
		c.player.X = 5.5
		c.player.Z = 5.5
		c.stateMu.Unlock()
		c.trackInventorySlot(36, protocol.ItemStack{Present: true, ItemID: 1, Count: 4})
		go func() {
			time.Sleep(25 * time.Millisecond)
			c.bus.Emit(state.BlockUpdateEvent{X: 2, Y: 64, Z: 2, StateID: 0})
		}()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		err := c.PlaceBlockSurvival(ctx, protocol.BlockPos{X: 2, Y: 64, Z: 2}, protocol.DirectionUp)
		if !errors.Is(err, ErrPlacementRolledBack) {
			t.Fatalf("error=%v want ErrPlacementRolledBack", err)
		}
	})
}

func TestBreakBlockReliabilityErrors(t *testing.T) {
	t.Run("break_dirt_succeeds_without_tool", func(t *testing.T) {
		c, _ := readyCoreActionClient(t)
		addFlatActionChunk(c)
		c.world.SetBlock(2, 64, 2, 10)
		go func() {
			time.Sleep(25 * time.Millisecond)
			c.world.SetBlock(2, 64, 2, 0)
			c.bus.Emit(state.BlockUpdateEvent{X: 2, Y: 64, Z: 2, StateID: 0})
		}()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := c.BreakBlock(ctx, protocol.BlockPos{X: 2, Y: 64, Z: 2}); err != nil {
			t.Fatalf("BreakBlock dirt: %v", err)
		}
	})

	t.Run("air_fails", func(t *testing.T) {
		c, _ := readyCoreActionClient(t)
		addFlatActionChunk(c)
		err := c.BreakBlock(context.Background(), protocol.BlockPos{X: 2, Y: 64, Z: 2})
		if !errors.Is(err, ErrBlockAir) {
			t.Fatalf("error=%v want ErrBlockAir", err)
		}
	})

	t.Run("unloaded_fails", func(t *testing.T) {
		c, _ := readyCoreActionClient(t)
		err := c.BreakBlock(context.Background(), protocol.BlockPos{X: -1, Y: 64, Z: -1})
		if !errors.Is(err, ErrChunkNotLoaded) {
			t.Fatalf("error=%v want ErrChunkNotLoaded", err)
		}
	})

	t.Run("under_feet_fails", func(t *testing.T) {
		c, _ := readyCoreActionClient(t)
		addFlatActionChunk(c)
		err := c.BreakBlock(context.Background(), protocol.BlockPos{X: 0, Y: 63, Z: 0})
		if !errors.Is(err, ErrBreakTargetUnsafe) {
			t.Fatalf("error=%v want ErrBreakTargetUnsafe", err)
		}
	})

	t.Run("out_of_reach_fails", func(t *testing.T) {
		c, _ := readyCoreActionClient(t)
		addFlatActionChunk(c)
		c.world.SetBlock(10, 64, 10, 10)
		err := c.BreakBlock(context.Background(), protocol.BlockPos{X: 10, Y: 64, Z: 10})
		if !errors.Is(err, ErrBreakOutOfReach) {
			t.Fatalf("error=%v want ErrBreakOutOfReach", err)
		}
	})

	t.Run("rollback_detected", func(t *testing.T) {
		c, _ := readyCoreActionClient(t)
		addFlatActionChunk(c)
		c.world.SetBlock(2, 64, 2, 10)
		go func() {
			time.Sleep(25 * time.Millisecond)
			c.bus.Emit(state.BlockUpdateEvent{X: 2, Y: 64, Z: 2, StateID: 10})
		}()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		err := c.BreakBlock(ctx, protocol.BlockPos{X: 2, Y: 64, Z: 2})
		if !errors.Is(err, ErrBreakRolledBack) {
			t.Fatalf("error=%v want ErrBreakRolledBack", err)
		}
	})
}

func TestContainerDepositWithdrawState(t *testing.T) {
	c, _ := readyCoreActionClient(t)
	c.activeContainer = &Container{ID: 2, Type: "chest", Title: "Chest"}
	c.containerSlots[2] = map[int]ItemStack{
		0: {Present: false},
		1: {Present: true, ItemID: 10, Name: "dirt", Count: 16},
	}
	c.trackInventorySlot(9, protocol.ItemStack{Present: true, ItemID: 1, Count: 32})
	c.trackInventorySlot(10, protocol.ItemStack{Present: false})

	go func() {
		time.Sleep(25 * time.Millisecond)
		c.bus.Emit(state.InventorySlotEvent{WindowID: 2, Slot: 0, Item: protocol.ItemStack{Present: true, ItemID: 1, Count: 16}})
		c.bus.Emit(state.InventorySlotEvent{WindowID: 0, Slot: 9, Item: protocol.ItemStack{Present: true, ItemID: 1, Count: 16}})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.DepositToContainer(ctx, 2, "stone", 16); err != nil {
		t.Fatalf("DepositToContainer: %v", err)
	}

	go func() {
		time.Sleep(25 * time.Millisecond)
		c.bus.Emit(state.InventorySlotEvent{WindowID: 2, Slot: 1, Item: protocol.ItemStack{Present: true, ItemID: 10, Count: 8}})
		c.bus.Emit(state.InventorySlotEvent{WindowID: 0, Slot: 10, Item: protocol.ItemStack{Present: true, ItemID: 10, Count: 8}})
	}()
	if err := c.WithdrawFromContainer(ctx, 2, "dirt", 8); err != nil {
		t.Fatalf("WithdrawFromContainer: %v", err)
	}
}

func TestContainerFailuresAndClose(t *testing.T) {
	c, _ := readyCoreActionClient(t)
	if err := c.DepositToContainer(context.Background(), 99, "stone", 1); !errors.Is(err, ErrContainerNotOpen) {
		t.Fatalf("stale container error=%v want ErrContainerNotOpen", err)
	}

	c.activeContainer = &Container{ID: 2, Type: "chest"}
	c.containerSlots[2] = map[int]ItemStack{0: {Present: true, ItemID: 10, Name: "dirt", Count: 64}}
	if err := c.DepositToContainer(context.Background(), 2, "stone", 1); !errors.Is(err, ErrInventoryItemNotFound) {
		t.Fatalf("missing deposit item error=%v want ErrInventoryItemNotFound", err)
	}
	if err := c.WithdrawFromContainer(context.Background(), 2, "stone", 1); !errors.Is(err, ErrContainerSlotNotFound) {
		t.Fatalf("missing withdraw item error=%v want ErrContainerSlotNotFound", err)
	}

	for i := 9; i <= 44; i++ {
		c.trackInventorySlot(int16(i), protocol.ItemStack{Present: true, ItemID: 1, Count: 64})
	}
	if err := c.WithdrawFromContainer(context.Background(), 2, "dirt", 1); !errors.Is(err, ErrInventoryFull) {
		t.Fatalf("full inventory error=%v want ErrInventoryFull", err)
	}

	if err := c.CloseContainer(context.Background(), 2); err != nil {
		t.Fatalf("CloseContainer: %v", err)
	}
	if c.ActiveContainer() != nil {
		t.Fatal("close should clear active container")
	}
}

func TestLookRotationMath(t *testing.T) {
	// North
	y, p := CalculateLookRotation(0, 1.62, 0, 0, 1.62, -2.0)
	if math.Abs(float64(y)-180) > 1e-4 && math.Abs(float64(y)+180) > 1e-4 {
		t.Errorf("North yaw got %f want 180 or -180", y)
	}
	if math.Abs(float64(p)) > 1e-4 {
		t.Errorf("North pitch got %f want 0", p)
	}

	// South
	y, p = CalculateLookRotation(0, 1.62, 0, 0, 1.62, 2.0)
	if math.Abs(float64(y)) > 1e-4 {
		t.Errorf("South yaw got %f want 0", y)
	}
	if math.Abs(float64(p)) > 1e-4 {
		t.Errorf("South pitch got %f want 0", p)
	}

	// East
	y, p = CalculateLookRotation(0, 1.62, 0, 2.0, 1.62, 0)
	if math.Abs(float64(y)-(-90)) > 1e-4 {
		t.Errorf("East yaw got %f want -90", y)
	}
	if math.Abs(float64(p)) > 1e-4 {
		t.Errorf("East pitch got %f want 0", p)
	}

	// West
	y, p = CalculateLookRotation(0, 1.62, 0, -2.0, 1.62, 0)
	if math.Abs(float64(y)-90) > 1e-4 {
		t.Errorf("West yaw got %f want 90", y)
	}
	if math.Abs(float64(p)) > 1e-4 {
		t.Errorf("West pitch got %f want 0", p)
	}

	// Above
	_, p = CalculateLookRotation(0, 1.62, 0, 0, 3.62, 0)
	if math.Abs(float64(p)-(-90)) > 1e-4 {
		t.Errorf("Above pitch got %f want -90", p)
	}

	// Below
	_, p = CalculateLookRotation(0, 1.62, 0, 0, -0.38, 0)
	if math.Abs(float64(p)-90) > 1e-4 {
		t.Errorf("Below pitch got %f want 90", p)
	}
}

func TestBreakBlockLookSequence(t *testing.T) {
	c := NewClient(Options{})
	markCoreActionReady(t, c)
	addFlatActionChunk(c)

	// Set player position and check target center
	c.stateMu.Lock()
	c.player.X = 0.5
	c.player.Y = 64
	c.player.Z = 0.5
	c.player.OnGround = true
	c.stateMu.Unlock()

	// Make sure the target block is stone so it is breakable (not air/bedrock)
	c.world.SetBlock(2, 64, 2, 1)

	a, b := net.Pipe()
	c.conn = feastconn.New(a)
	defer a.Close()
	defer b.Close()

	go func() {
		// BreakBlock will block until it gets block update or times out.
		// Since we only care about the sent packet sequence, we can run BreakBlock in a goroutine with a timeout.
		ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
		defer cancel()
		_ = c.BreakBlock(ctx, protocol.BlockPos{X: 2, Y: 64, Z: 2})
	}()

	s := feastconn.New(b)

	// First packet should be PlayServerboundSetPlayerPositionAndRotationPacket
	rawPkt1, err := s.ReadPacket()
	if err != nil {
		t.Fatalf("failed to read packet 1: %v", err)
	}

	var posRot protocol.PlayServerboundSetPlayerPositionAndRotationPacket
	if rawPkt1.ID != posRot.PacketID() {
		t.Fatalf("expected SetPlayerPositionAndRotation packet, got ID %d", rawPkt1.ID)
	}

	// Unmarshal and verify yaw/pitch, and preserved position/onGround
	var decodedPosRot protocol.PlayServerboundSetPlayerPositionAndRotationPacket
	r1 := protocol.NewReader(bytes.NewReader(rawPkt1.Data))
	if err := decodedPosRot.Unmarshal(r1); err != nil {
		t.Fatalf("failed to unmarshal rotation packet: %v", err)
	}

	if decodedPosRot.X != 0.5 || decodedPosRot.Y != 64 || decodedPosRot.Z != 0.5 || !decodedPosRot.OnGround {
		t.Errorf("expected position (0.5, 64, 0.5) and onGround true, got (%f, %f, %f) onGround=%v",
			decodedPosRot.X, decodedPosRot.Y, decodedPosRot.Z, decodedPosRot.OnGround)
	}

	// Calculate expected yaw/pitch from (0.5, 65.62, 0.5) to (2.5, 64.5, 2.5)
	expectedYaw, expectedPitch := CalculateLookRotation(0.5, 65.62, 0.5, 2.5, 64.5, 2.5)
	if math.Abs(float64(decodedPosRot.Yaw-expectedYaw)) > 1e-4 {
		t.Errorf("expected yaw %f, got %f", expectedYaw, decodedPosRot.Yaw)
	}
	if math.Abs(float64(decodedPosRot.Pitch-expectedPitch)) > 1e-4 {
		t.Errorf("expected pitch %f, got %f", expectedPitch, decodedPosRot.Pitch)
	}

	// Second packet should be PlayServerboundPlayerActionPacket (StartDigging)
	rawPkt2, err := s.ReadPacket()
	if err != nil {
		t.Fatalf("failed to read packet 2: %v", err)
	}

	var actionPkt protocol.PlayServerboundPlayerActionPacket
	if rawPkt2.ID != actionPkt.PacketID() {
		t.Fatalf("expected PlayerAction packet, got ID %d", rawPkt2.ID)
	}

	var decodedAction protocol.PlayServerboundPlayerActionPacket
	r2 := protocol.NewReader(bytes.NewReader(rawPkt2.Data))
	if err := decodedAction.Unmarshal(r2); err != nil {
		t.Fatalf("failed to unmarshal player action: %v", err)
	}

	if decodedAction.Status != protocol.PlayerActionStartDigging {
		t.Errorf("expected StartDigging status %d, got %d", protocol.PlayerActionStartDigging, decodedAction.Status)
	}
}
