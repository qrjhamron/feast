package feast

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	feastconn "github.com/user/feastgo/pkg/conn"
	"github.com/user/feastgo/pkg/protocol"
	"github.com/user/feastgo/pkg/state"
	"github.com/user/feastgo/pkg/world"
)

func TestDefaultEmptyInventory(t *testing.T) {
	c := NewClient(Options{})
	snap := c.InventorySnapshot()
	if snap.SelectedHotbarSlot != 0 {
		t.Errorf("expected SelectedHotbarSlot=0, got %d", snap.SelectedHotbarSlot)
	}
	for i := 36; i <= 44; i++ {
		slot, exists := snap.Slots[i]
		if exists && slot.Present {
			t.Errorf("expected hotbar slot %d to be empty, got %+v", i, slot)
		}
	}
}

func TestSelectHotbarSlot(t *testing.T) {
	c := NewClient(Options{})
	// Mock connection for packet sending
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	c.conn = feastconn.New(a)

	go func() {
		buf := make([]byte, 1024)
		for {
			_, err := b.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	err := c.SelectHotbarSlot(context.Background(), 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	snap := c.InventorySnapshot()
	if snap.SelectedHotbarSlot != 5 {
		t.Fatalf("selected slot=%d want 5", snap.SelectedHotbarSlot)
	}
}

func TestSetSlotUpdate(t *testing.T) {
	c := NewClient(Options{})
	c.bus.Emit(state.HeldItemEvent{Slot: 2})
	c.bus.Emit(state.InventorySlotEvent{
		WindowID: 0,
		Slot:     38, // 36 + 2
		Item:     protocol.ItemStack{Present: true, ItemID: 1, Count: 64, NBT: []byte{0x00}},
	})

	snap := c.InventorySnapshot()
	heldSlot := 38
	heldItem := snap.Slots[heldSlot]
	if !heldItem.Present || heldItem.ItemID != 1 || heldItem.Name != "stone" {
		t.Fatalf("unexpected snapshot slot 38: %+v", heldItem)
	}
	if heldItem.Source != "server" {
		t.Fatalf("expected source to be server, got %q", heldItem.Source)
	}
	if !IsPlaceableBlockItem(heldItem) {
		t.Fatalf("expected stone to be placeable block")
	}
}

func TestCreativeSlotInjection(t *testing.T) {
	c := NewClient(Options{})
	target := protocol.BlockPos{X: 10, Y: 64, Z: 20}

	// Preload chunk and support block so PrepareCreativeSmokePlacement succeeds
	ch := world.NewChunk(0, 1)
	c.world.AddChunk(ch)

	// Get support and target
	// target chunk coordinate: x=10 -> chunkX=0, z=20 -> chunkZ=1
	// set support block (y=63) to solid (stone ID=1)
	c.world.SetBlock(10, 63, 20, 1)
	c.world.SetBlock(10, 64, 20, 0) // target is air

	plan, err := c.PrepareCreativeSmokePlacement(target)
	if err != nil {
		t.Fatalf("PrepareCreativeSmokePlacement failed: %v", err)
	}

	// Mock connection
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	c.conn = feastconn.New(a)

	// Read side in background to prevent block on write
	go func() {
		buf := make([]byte, 1024)
		for {
			_, err := b.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	// Call Execute
	err = c.ExecuteCreativeSmokePlacement(plan)
	if err != nil && !errors.Is(err, ErrClientClosed) {
		t.Fatalf("expected success or ErrClientClosed, got: %v", err)
	}

	snap := c.InventorySnapshot()
	// Slot 36 corresponds to hotbar slot 0 -> slot 36
	hotbarSlot := snap.Slots[36]
	if !hotbarSlot.Present || hotbarSlot.ItemID != 1 || hotbarSlot.Source != "creative_smoke" {
		t.Fatalf("expected creative smoke injection in slot 36, got: %+v", hotbarSlot)
	}
}

func TestUnknownItemID(t *testing.T) {
	c := NewClient(Options{})
	c.trackInventorySlot(36, protocol.ItemStack{Present: true, ItemID: 9999, Count: 1, NBT: []byte{0x00}})

	snap := c.InventorySnapshot()
	hotbarSlot := snap.Slots[36]
	if !hotbarSlot.Present || hotbarSlot.Name != "item_9999" {
		t.Fatalf("expected name to be item_9999, got: %+v", hotbarSlot)
	}
	if IsPlaceableBlockItem(hotbarSlot) {
		t.Fatalf("expected unknown item to not be placeable")
	}
}

func TestEmptySlotPlacementReturnsCleanError(t *testing.T) {
	c := NewClient(Options{})
	err := c.PlaceBlock(protocol.BlockPos{X: 1, Y: 65, Z: 1}, protocol.BlockFaceTop)
	if !errors.Is(err, ErrHeldItemUnknown) {
		t.Fatalf("PlaceBlock error=%v want %v", err, ErrHeldItemUnknown)
	}
}

func TestUnloadedChunkTargetReturnsCleanError(t *testing.T) {
	c := NewClient(Options{})
	c.trackInventorySlot(36, protocol.ItemStack{Present: true, ItemID: 1, Count: 1, NBT: []byte{0x00}})
	c.trackSelectedHotbarSlot(0)

	// target is at x=0, z=0, Y=64, no chunks loaded
	err := c.PlaceBlock(protocol.BlockPos{X: 0, Y: 64, Z: 0}, protocol.BlockFaceTop)
	if err == nil || !errors.Is(err, world.ErrBlockNotFound) && err.Error() == "" {
		t.Fatalf("expected error from unloaded chunk, got %v", err)
	}
}

func TestNegativeCoordinateTarget(t *testing.T) {
	c := NewClient(Options{})
	c.trackInventorySlot(36, protocol.ItemStack{Present: true, ItemID: 1, Count: 1, NBT: []byte{0x00}})
	c.trackSelectedHotbarSlot(0)

	// target is at negative coordinates
	target := protocol.BlockPos{X: -10, Y: 64, Z: -20}
	err := c.PlaceBlock(target, protocol.BlockFaceTop)
	if err == nil {
		t.Fatalf("expected error from negative coordinate unloaded chunk, got nil")
	}
}

func TestItemAndBlockMapping(t *testing.T) {
	// Tests Phase 3 requirements
	// 1. Unknown items do not panic, return ok=false
	name, ok := ItemNameFromID(12345)
	if ok || name != "item_12345" {
		t.Errorf("expected ok=false for unknown ID, got ok=%v name=%s", ok, name)
	}

	// 2. Known items
	name, ok = ItemNameFromID(1)
	if !ok || name != "stone" {
		t.Errorf("expected stone for ID 1, got ok=%v name=%s", ok, name)
	}

	// 3. Placeable check
	stoneItem := ItemStack{Present: true, ItemID: 1, Name: "stone", Count: 1}
	if !IsPlaceableBlockItem(stoneItem) {
		t.Errorf("expected stone to be placeable block item")
	}

	unknownItem := ItemStack{Present: true, ItemID: 9999, Name: "item_9999", Count: 1}
	if IsPlaceableBlockItem(unknownItem) {
		t.Errorf("expected unknown item to not be placeable")
	}

	emptyItem := ItemStack{Present: false}
	if IsPlaceableBlockItem(emptyItem) {
		t.Errorf("expected empty item to not be placeable")
	}
}

func TestPlaceBlockSurvivalSuccessAndFailure(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		c := NewClient(Options{})
		ch := world.NewChunk(0, 0)
		c.world.AddChunk(ch)

		// Set target (10, 64, 10) to air, and support (10, 63, 10) to solid stone
		c.world.SetBlock(10, 63, 10, 1) // stone
		c.world.SetBlock(10, 64, 10, 0) // air

		// Mock player coordinates so it doesn't overlap target (10, 64, 10)
		c.stateMu.Lock()
		c.player.X = 5.0
		c.player.Y = 64.0
		c.player.Z = 5.0
		c.stateMu.Unlock()

		// Track stone in hotbar slot 36
		c.trackInventorySlot(36, protocol.ItemStack{Present: true, ItemID: 1, Count: 16})
		c.trackSelectedHotbarSlot(0)

		// Mock connection
		a, b := net.Pipe()
		defer a.Close()
		defer b.Close()
		c.conn = feastconn.New(a)

		// Background reader and block update emitter
		go func() {
			buf := make([]byte, 1024)
			for {
				_, err := b.Read(buf)
				if err != nil {
					return
				}
			}
		}()

		go func() {
			// Wait a bit, then set target block in world and emit event
			// (Simulating the server responding with a block update)
			c.world.SetBlock(10, 64, 10, 1)
			c.bus.Emit(state.BlockUpdateEvent{X: 10, Y: 64, Z: 10, StateID: 1})
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		err := c.PlaceBlockSurvival(ctx, protocol.BlockPos{X: 10, Y: 64, Z: 10}, protocol.DirectionUp)
		if err != nil {
			t.Fatalf("expected PlaceBlockSurvival to succeed, got: %v", err)
		}
	})

	t.Run("no_placeable_block", func(t *testing.T) {
		c := NewClient(Options{})
		ch := world.NewChunk(0, 0)
		c.world.AddChunk(ch)
		c.world.SetBlock(10, 63, 10, 1)
		c.world.SetBlock(10, 64, 10, 0)

		c.stateMu.Lock()
		c.player.X = 5.0
		c.player.Y = 64.0
		c.player.Z = 5.0
		c.stateMu.Unlock()

		// Empty slot 36
		c.trackInventorySlot(36, protocol.ItemStack{Present: false})

		err := c.PlaceBlockSurvival(context.Background(), protocol.BlockPos{X: 10, Y: 64, Z: 10}, protocol.DirectionUp)
		if err == nil {
			t.Fatalf("expected error due to no placeable block, got nil")
		}
	})
}
