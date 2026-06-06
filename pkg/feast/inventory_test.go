package feast

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	feastconn "github.com/qrjhamron/feast/pkg/conn"
	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/state"
	"github.com/qrjhamron/feast/pkg/world"
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
		markCoreActionReady(t, c)
		ch := world.NewChunk(0, 0)
		c.world.AddChunk(ch)

		// Target (10, 64, 10) must start as air so PlaceBlockSurvival's
		// pre-placement air-check passes; the goroutine below simulates the
		// server confirming placement by updating the block afterwards.
		c.world.SetBlock(10, 63, 10, 1) // support block: solid stone
		c.world.SetBlock(10, 64, 10, 0) // target block: air (placement target)

		// Mock player coordinates so it doesn't overlap target (10, 64, 10)
		c.stateMu.Lock()
		c.player.X = 8.5
		c.player.Y = 64.0
		c.player.Z = 8.5
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
			// Delay ensures PlaceBlockSurvival's upfront air-check completes
			// before the goroutine sets the block to stone.  Without this
			// delay the goroutine races with the check and can cause it to
			// see stone instead of air, failing the test non-deterministically.
			time.Sleep(100 * time.Millisecond)
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

func TestBreakBlockAutoTool(t *testing.T) {
	t.Run("auto_select_tools", func(t *testing.T) {
		c := NewClient(Options{})
		markCoreActionReady(t, c)
		ch := world.NewChunk(0, 0)
		c.world.AddChunk(ch)

		// Set bot position close to target
		c.stateMu.Lock()
		c.player.X = 0.0
		c.player.Y = 64.0
		c.player.Z = 0.0
		c.stateMu.Unlock()

		// Add tools to hotbar
		// 831 is iron_pickaxe, 830 is iron_shovel, 832 is iron_axe
		c.trackInventorySlot(36, protocol.ItemStack{Present: true, ItemID: 831, Count: 1}) // Slot 0
		c.trackInventorySlot(37, protocol.ItemStack{Present: true, ItemID: 830, Count: 1}) // Slot 1
		c.trackInventorySlot(38, protocol.ItemStack{Present: true, ItemID: 832, Count: 1}) // Slot 2

		// Mock connection
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

		// Test Case 1: Stone block -> Pickaxe (Slot 0)
		c.world.SetBlock(0, 64, 2, 1) // Stone
		// Simulate server sending block update to air when dug
		go func() {
			time.Sleep(50 * time.Millisecond)
			c.world.SetBlock(0, 64, 2, 0)
			c.bus.Emit(state.BlockUpdateEvent{X: 0, Y: 64, Z: 2, StateID: 0})
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		err := c.BreakBlock(ctx, protocol.BlockPos{X: 0, Y: 64, Z: 2}, BreakOptions{AutoTool: true})
		cancel()
		if err != nil {
			t.Fatalf("expected stone break to succeed, got: %v", err)
		}
		c.inventoryMu.RLock()
		if c.inventory.SelectedHotbarSlot != 0 {
			t.Errorf("expected hotbar slot 0 for pickaxe, got %d", c.inventory.SelectedHotbarSlot)
		}
		c.inventoryMu.RUnlock()

		// Test Case 2: Dirt block -> Shovel (Slot 1)
		c.world.SetBlock(0, 64, 2, 10) // Dirt (state ID 10)
		go func() {
			time.Sleep(50 * time.Millisecond)
			c.world.SetBlock(0, 64, 2, 0)
			c.bus.Emit(state.BlockUpdateEvent{X: 0, Y: 64, Z: 2, StateID: 0})
		}()
		ctx, cancel = context.WithTimeout(context.Background(), 1*time.Second)
		err = c.BreakBlock(ctx, protocol.BlockPos{X: 0, Y: 64, Z: 2}, BreakOptions{AutoTool: true})
		cancel()
		if err != nil {
			t.Fatalf("expected dirt break to succeed, got: %v", err)
		}
		c.inventoryMu.RLock()
		if c.inventory.SelectedHotbarSlot != 1 {
			t.Errorf("expected hotbar slot 1 for shovel, got %d", c.inventory.SelectedHotbarSlot)
		}
		c.inventoryMu.RUnlock()

		// Test Case 3: Oak Log -> Axe (Slot 2)
		c.world.SetBlock(0, 64, 2, 131) // Oak Log (state ID 131)
		go func() {
			time.Sleep(50 * time.Millisecond)
			c.world.SetBlock(0, 64, 2, 0)
			c.bus.Emit(state.BlockUpdateEvent{X: 0, Y: 64, Z: 2, StateID: 0})
		}()
		ctx, cancel = context.WithTimeout(context.Background(), 1*time.Second)
		err = c.BreakBlock(ctx, protocol.BlockPos{X: 0, Y: 64, Z: 2}, BreakOptions{AutoTool: true})
		cancel()
		if err != nil {
			t.Fatalf("expected log break to succeed, got: %v", err)
		}
		c.inventoryMu.RLock()
		if c.inventory.SelectedHotbarSlot != 2 {
			t.Errorf("expected hotbar slot 2 for axe, got %d", c.inventory.SelectedHotbarSlot)
		}
		c.inventoryMu.RUnlock()
	})

	t.Run("fallback_and_errors", func(t *testing.T) {
		c := NewClient(Options{})
		markCoreActionReady(t, c)
		ch := world.NewChunk(0, 0)
		c.world.AddChunk(ch)

		c.stateMu.Lock()
		c.player.X = 0.0
		c.player.Y = 64.0
		c.player.Z = 0.0
		c.stateMu.Unlock()

		// Mock connection
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

		// Target is out of reach
		err := c.BreakBlock(context.Background(), protocol.BlockPos{X: 10, Y: 64, Z: 10}, BreakOptions{AutoTool: true})
		if err == nil {
			t.Errorf("expected error for out of reach target")
		}

		// Target is under feet
		err = c.BreakBlock(context.Background(), protocol.BlockPos{X: 0, Y: 63, Z: 0}, BreakOptions{AutoTool: true})
		if err == nil {
			t.Errorf("expected error for under feet target")
		}

		// Fallback cleanly when no tool is found
		c.world.SetBlock(0, 64, 2, 1) // Stone
		go func() {
			time.Sleep(50 * time.Millisecond)
			c.world.SetBlock(0, 64, 2, 0)
			c.bus.Emit(state.BlockUpdateEvent{X: 0, Y: 64, Z: 2, StateID: 0})
		}()
		err = c.BreakBlock(context.Background(), protocol.BlockPos{X: 0, Y: 64, Z: 2}, BreakOptions{AutoTool: true})
		if err != nil {
			t.Errorf("expected fallback to succeed, got: %v", err)
		}
	})
}

func TestContainerChestSupport(t *testing.T) {
	t.Run("open_container_and_updates", func(t *testing.T) {
		c := NewClient(Options{})

		// Mock connection
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

		// Simulate server sending open screen event
		go func() {
			time.Sleep(100 * time.Millisecond)
			c.bus.Emit(state.OpenScreenEvent{
				WindowID:   5,
				WindowType: 2, // generic_9x3 chest
				Title:      "My Chest Title",
			})
			// Simulate server sending container content event
			c.bus.Emit(state.ContainerContentEvent{
				WindowID: 5,
				Slots: []protocol.ItemStack{
					{Present: true, ItemID: 1, Count: 64}, // Slot 0: stone
					{Present: false},                      // Slot 1: empty
				},
			})
		}()

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		container, err := c.OpenChest(ctx, BlockPos{X: 1, Y: 2, Z: 3})
		if err != nil {
			t.Fatalf("expected OpenChest to succeed, got: %v", err)
		}
		if container.ID != 5 || container.Type != "chest" || container.Title != "My Chest Title" {
			t.Fatalf("unexpected container values: %+v", container)
		}

		// Verify ActiveContainer
		active := c.ActiveContainer()
		if active == nil || active.ID != 5 {
			t.Fatalf("expected active container to be set")
		}

		// Verify slots
		items := c.ContainerItems(5)
		if len(items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(items))
		}
		if !items[0].Present || items[0].ItemID != 1 {
			t.Fatalf("expected slot 0 to have stone, got %+v", items[0])
		}
		if items[1].Present {
			t.Fatalf("expected slot 1 to be empty, got %+v", items[1])
		}

		// Test slot update
		c.bus.Emit(state.InventorySlotEvent{
			WindowID: 5,
			Slot:     1,
			Item:     protocol.ItemStack{Present: true, ItemID: 28, Count: 32}, // dirt
		})

		items = c.ContainerItems(5)
		if !items[1].Present || items[1].ItemID != 28 {
			t.Fatalf("expected slot 1 to have dirt now, got %+v", items[1])
		}

		// Close Container
		err = c.CloseContainer(ctx, 5)
		if err != nil {
			t.Fatalf("expected CloseContainer to succeed, got %v", err)
		}

		// Verify container cleared
		if c.ActiveContainer() != nil {
			t.Fatalf("expected active container to be cleared")
		}
		if c.ContainerItems(5) != nil {
			t.Fatalf("expected container slots to be cleared")
		}
	})

	t.Run("unknown_container_fallback", func(t *testing.T) {
		c := NewClient(Options{})

		// Simulate server opening unknown container type
		c.bus.Emit(state.OpenScreenEvent{
			WindowID:   6,
			WindowType: 99, // unknown type
			Title:      "Strange Box",
		})

		active := c.ActiveContainer()
		if active == nil || active.Type != "window_type_99" {
			t.Fatalf("expected unknown container type fallback to window_type_99, got %+v", active)
		}
	})
}
