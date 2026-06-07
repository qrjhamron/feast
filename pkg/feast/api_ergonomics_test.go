package feast

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/state"
	"github.com/qrjhamron/feast/pkg/world"
)

func waitForHandlerCountAtLeast(t *testing.T, c *Client, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := c.Events().HandlerCount(); got >= want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("handler count never reached %d; got %d", want, c.Events().HandlerCount())
}

func TestStateSnapshotReportsReadyFields(t *testing.T) {
	c := NewClient(Options{Username: "FeastGoBot"})
	markCoreActionReady(t, c)
	c.stateMu.Lock()
	c.player.EntityID = 42
	c.player.Health = 17.5
	c.player.Food = 9
	c.stateMu.Unlock()
	c.trackSelectedHotbarSlot(3)
	c.trackInventorySlot(39, protocol.ItemStack{
		Present: true,
		ItemID:  1,
		Count:   4,
		NBT:     []byte{0x01, 0x02},
	})

	snap := c.State()
	if snap.Username != "FeastGoBot" {
		t.Fatalf("Username = %q, want %q", snap.Username, "FeastGoBot")
	}
	if !snap.Connected {
		t.Fatal("expected Connected=true")
	}
	if !snap.Ready {
		t.Fatal("expected Ready=true")
	}
	if !snap.PositionSynced {
		t.Fatal("expected PositionSynced=true")
	}
	if snap.CurrentState != state.StatePlay {
		t.Fatalf("CurrentState = %v, want play", snap.CurrentState)
	}
	if snap.EntityID != 42 {
		t.Fatalf("EntityID = %d, want 42", snap.EntityID)
	}
	if snap.Position != (Vec3{X: 0.5, Y: 64, Z: 0.5}) {
		t.Fatalf("Position = %+v, want 0.5/64/0.5", snap.Position)
	}
	if snap.Yaw != 0 || snap.Pitch != 0 {
		t.Fatalf("Yaw/Pitch = %v/%v, want 0/0", snap.Yaw, snap.Pitch)
	}
	if snap.Health != 17.5 {
		t.Fatalf("Health = %v, want 17.5", snap.Health)
	}
	if snap.Food != 9 {
		t.Fatalf("Food = %d, want 9", snap.Food)
	}
	if snap.SelectedHotbarSlot != 3 {
		t.Fatalf("SelectedHotbarSlot = %d, want 3", snap.SelectedHotbarSlot)
	}
	if !snap.HeldItem.Present || snap.HeldItem.Name != "stone" || snap.HeldItem.Count != 4 {
		t.Fatalf("HeldItem = %+v, want present stone x4", snap.HeldItem)
	}
}

func TestStateSnapshotConcurrentSafe(t *testing.T) {
	c := NewClient(Options{})
	markCoreActionReady(t, c)
	c.trackInventorySlot(36, protocol.ItemStack{Present: true, ItemID: 1, Count: 8})
	c.trackSelectedHotbarSlot(0)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			c.bus.Emit(state.PositionEvent{X: float64(i), Y: 64, Z: 0, Yaw: float32(i % 360), Pitch: float32(i % 90)})
			c.bus.Emit(state.HealthEvent{Health: float32(20 - (i % 4)), Food: int32(20 - (i % 7)), Saturation: 5})
			c.bus.Emit(state.HeldItemEvent{Slot: i % 9})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			_ = c.State()
		}
	}()
	wg.Wait()
}

func TestStateSnapshotDoesNotExposeMutableInventory(t *testing.T) {
	c := NewClient(Options{})
	c.trackInventorySlot(36, protocol.ItemStack{
		Present: true,
		ItemID:  1,
		Count:   1,
		NBT:     []byte{0x01, 0x02},
	})
	c.trackSelectedHotbarSlot(0)

	snap := c.State()
	snap.HeldItem.NBT[0] = 0xFF

	held, ok := c.HeldItem()
	if !ok {
		t.Fatal("expected held item")
	}
	if held.NBT[0] != 0x01 {
		t.Fatalf("HeldItem NBT mutated through snapshot: got %x", held.NBT)
	}
}

func TestStateSnapshotAfterRespawnUnsynced(t *testing.T) {
	c := NewClient(Options{})
	markCoreActionReady(t, c)

	emitRespawn(c, "minecraft:the_nether")

	snap := c.State()
	if snap.PositionSynced {
		t.Fatal("expected PositionSynced=false after respawn")
	}
	if snap.Ready {
		t.Fatal("expected Ready=false after respawn")
	}
	if snap.CurrentState != state.StatePlay {
		t.Fatalf("CurrentState = %v, want play after respawn", snap.CurrentState)
	}
}

func TestTypedEventHelpersLeaveRawOnWorking(t *testing.T) {
	c := NewClient(Options{})

	respawnTyped := 0
	respawnRaw := 0
	respawnUnsub := c.OnRespawn(func(e state.RespawnEvent) {
		respawnTyped++
	})
	if _, err := c.On("respawn", func(e state.Event) {
		if _, ok := e.(state.RespawnEvent); ok {
			respawnRaw++
		}
	}); err != nil {
		t.Fatalf("raw respawn subscribe: %v", err)
	}

	posTyped := 0
	posRaw := 0
	posUnsub := c.OnPositionSync(func(e PositionEvent) {
		posTyped++
	})
	if _, err := c.On("position", func(e state.Event) {
		if _, ok := e.(state.PositionEvent); ok {
			posRaw++
		}
	}); err != nil {
		t.Fatalf("raw position subscribe: %v", err)
	}

	blockTyped := 0
	blockRaw := 0
	blockUnsub := c.OnBlockUpdate(func(e BlockUpdateEvent) {
		blockTyped++
	})
	if _, err := c.On("block_update", func(e state.Event) {
		if _, ok := e.(state.BlockUpdateEvent); ok {
			blockRaw++
		}
	}); err != nil {
		t.Fatalf("raw block_update subscribe: %v", err)
	}

	emitRespawn(c, "minecraft:the_end")
	c.bus.Emit(state.PositionEvent{X: 1, Y: 64, Z: 1})
	c.bus.Emit(state.BlockUpdateEvent{X: 1, Y: 64, Z: 1, StateID: 1})

	if respawnTyped != 1 || respawnRaw != 1 {
		t.Fatalf("respawn counts = typed:%d raw:%d, want 1/1", respawnTyped, respawnRaw)
	}
	if posTyped != 1 || posRaw != 1 {
		t.Fatalf("position counts = typed:%d raw:%d, want 1/1", posTyped, posRaw)
	}
	if blockTyped != 1 || blockRaw != 1 {
		t.Fatalf("block_update counts = typed:%d raw:%d, want 1/1", blockTyped, blockRaw)
	}

	respawnUnsub()
	respawnUnsub()
	posUnsub()
	posUnsub()
	blockUnsub()
	blockUnsub()

	emitRespawn(c, "minecraft:the_nether")
	c.bus.Emit(state.PositionEvent{X: 2, Y: 64, Z: 2})
	c.bus.Emit(state.BlockUpdateEvent{X: 2, Y: 64, Z: 2, StateID: 1})

	if respawnTyped != 1 || respawnRaw != 2 {
		t.Fatalf("respawn unsubscribe counts = typed:%d raw:%d, want 1/2", respawnTyped, respawnRaw)
	}
	if posTyped != 1 || posRaw != 2 {
		t.Fatalf("position unsubscribe counts = typed:%d raw:%d, want 1/2", posTyped, posRaw)
	}
	if blockTyped != 1 || blockRaw != 2 {
		t.Fatalf("block unsubscribe counts = typed:%d raw:%d, want 1/2", blockTyped, blockRaw)
	}
}

func TestTypedEventHandlerPanicRecovered(t *testing.T) {
	c := NewClient(Options{})
	called := 0
	c.OnRespawn(func(state.RespawnEvent) {
		panic("boom")
	})
	c.OnRespawn(func(state.RespawnEvent) {
		called++
	})

	emitRespawn(c, "minecraft:the_nether")

	if called != 1 {
		t.Fatalf("panic recovery blocked later handlers: got %d", called)
	}
}

func TestWaitForPositionSyncAlreadySynced(t *testing.T) {
	c := NewClient(Options{})
	c.stateMu.Lock()
	c.positionSynced = true
	c.stateMu.Unlock()

	base := c.Events().HandlerCount()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.WaitForPositionSync(ctx); err != nil {
		t.Fatalf("WaitForPositionSync: %v", err)
	}
	if got := c.Events().HandlerCount(); got != base {
		t.Fatalf("handler leak: base=%d got=%d", base, got)
	}
}

func TestWaitForPositionSyncWakesOnEvent(t *testing.T) {
	c := NewClient(Options{})
	base := c.Events().HandlerCount()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- c.WaitForPositionSync(ctx) }()
	waitForHandlerCountAtLeast(t, c, base+1)

	c.bus.Emit(state.PositionEvent{X: 1, Y: 64, Z: 1})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WaitForPositionSync returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForPositionSync did not wake on position sync")
	}
	if got := c.Events().HandlerCount(); got != base {
		t.Fatalf("handler leak after wake: base=%d got=%d", base, got)
	}
}

func TestWaitForPositionSyncContextCancel(t *testing.T) {
	c := NewClient(Options{})
	base := c.Events().HandlerCount()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() { done <- c.WaitForPositionSync(ctx) }()
	waitForHandlerCountAtLeast(t, c, base+1)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("WaitForPositionSync cancel error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForPositionSync did not return after cancel")
	}
	if got := c.Events().HandlerCount(); got != base {
		t.Fatalf("handler leak after cancel: base=%d got=%d", base, got)
	}
}

func TestWaitForBlockUpdateWakesOnSingleBlockUpdate(t *testing.T) {
	c := NewClient(Options{})
	addFlatActionChunk(c)
	base := c.Events().HandlerCount()
	target := protocol.BlockPos{X: 2, Y: 64, Z: 2}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct {
		block world.BlockState
		err   error
	}, 1)
	go func() {
		block, err := c.WaitForBlockUpdate(ctx, int(target.X), int(target.Y), int(target.Z))
		done <- struct {
			block world.BlockState
			err   error
		}{block: block, err: err}
	}()
	waitForHandlerCountAtLeast(t, c, base+2)

	c.bus.Emit(state.BlockUpdateEvent{X: target.X, Y: target.Y, Z: target.Z, StateID: 1})

	select {
	case out := <-done:
		if out.err != nil {
			t.Fatalf("WaitForBlockUpdate returned error: %v", out.err)
		}
		if out.block.Name != "stone" {
			t.Fatalf("WaitForBlockUpdate block = %+v, want stone", out.block)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForBlockUpdate did not wake on block update")
	}
	if got := c.Events().HandlerCount(); got != base {
		t.Fatalf("handler leak after block update: base=%d got=%d", base, got)
	}
}

func TestWaitForBlockUpdateWakesOnSectionBlockUpdate(t *testing.T) {
	c := NewClient(Options{})
	addFlatActionChunk(c)
	base := c.Events().HandlerCount()
	target := protocol.BlockPos{X: 2, Y: 64, Z: 2}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := c.WaitForBlockUpdate(ctx, int(target.X), int(target.Y), int(target.Z))
		done <- err
	}()
	waitForHandlerCountAtLeast(t, c, base+2)

	c.bus.Emit(state.SectionBlocksUpdateEvent{Updates: []state.SectionBlockUpdate{
		{X: target.X, Y: target.Y, Z: target.Z, StateID: 1},
	}})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WaitForBlockUpdate returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForBlockUpdate did not wake on section block update")
	}
	if got := c.Events().HandlerCount(); got != base {
		t.Fatalf("handler leak after section update: base=%d got=%d", base, got)
	}
}

func TestWaitForBlockUpdateIgnoresOtherBlocks(t *testing.T) {
	c := NewClient(Options{})
	addFlatActionChunk(c)
	target := protocol.BlockPos{X: 2, Y: 64, Z: 2}
	base := c.Events().HandlerCount()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		_, err := c.WaitForBlockUpdate(ctx, int(target.X), int(target.Y), int(target.Z))
		done <- err
	}()
	waitForHandlerCountAtLeast(t, c, base+2)

	c.bus.Emit(state.BlockUpdateEvent{X: 8, Y: 64, Z: 8, StateID: 1})
	select {
	case err := <-done:
		t.Fatalf("WaitForBlockUpdate woke on other block: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	c.bus.Emit(state.BlockUpdateEvent{X: target.X, Y: target.Y, Z: target.Z, StateID: 1})
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WaitForBlockUpdate returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForBlockUpdate did not wake on target block")
	}
}

func TestWaitForBlockUpdateContextCancelUnsubscribes(t *testing.T) {
	c := NewClient(Options{})
	addFlatActionChunk(c)
	base := c.Events().HandlerCount()
	target := protocol.BlockPos{X: 2, Y: 64, Z: 2}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := c.WaitForBlockUpdate(ctx, int(target.X), int(target.Y), int(target.Z))
		done <- err
	}()
	waitForHandlerCountAtLeast(t, c, base+2)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("WaitForBlockUpdate cancel error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForBlockUpdate did not return after cancel")
	}
	if got := c.Events().HandlerCount(); got != base {
		t.Fatalf("handler leak after cancel: base=%d got=%d", base, got)
	}
}

func TestWaitForBlockStateAlreadyMatches(t *testing.T) {
	c := NewClient(Options{})
	addFlatActionChunk(c)
	base := c.Events().HandlerCount()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.WaitForBlockState(ctx, 2, 64, 2, "air"); err != nil {
		t.Fatalf("WaitForBlockState already-match: %v", err)
	}
	if got := c.Events().HandlerCount(); got != base {
		t.Fatalf("handler leak on already-match path: base=%d got=%d", base, got)
	}
}

func TestWaitForBlockStateWaitsUntilMatch(t *testing.T) {
	c := NewClient(Options{})
	addFlatActionChunk(c)
	base := c.Events().HandlerCount()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- c.WaitForBlockState(ctx, 2, 64, 2, "stone") }()
	waitForHandlerCountAtLeast(t, c, base+2)

	c.bus.Emit(state.BlockUpdateEvent{X: 2, Y: 64, Z: 2, StateID: 1})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WaitForBlockState returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitForBlockState did not wake on matching update")
	}
}

func TestWaitForBlockStateUnloadedDoesNotMatch(t *testing.T) {
	c := NewClient(Options{})
	base := c.Events().HandlerCount()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := c.WaitForBlockState(ctx, 9999, 64, 9999, "stone")
	if err == nil {
		t.Fatal("expected WaitForBlockState to time out for unloaded block")
	}
	if got := c.Events().HandlerCount(); got != base {
		t.Fatalf("handler leak on unloaded path: base=%d got=%d", base, got)
	}
}

func TestPlaceBlockWithResultSuccess(t *testing.T) {
	c, _ := readyCoreActionClient(t)
	addFlatActionChunk(c)
	c.stateMu.Lock()
	c.player.X = 5.5
	c.player.Z = 5.5
	c.stateMu.Unlock()
	c.trackSelectedHotbarSlot(0)
	c.trackInventorySlot(36, protocol.ItemStack{Present: true, ItemID: 1, Count: 4})

	go func() {
		time.Sleep(25 * time.Millisecond)
		c.world.SetBlock(2, 64, 2, 1)
		c.bus.Emit(state.BlockUpdateEvent{X: 2, Y: 64, Z: 2, StateID: 1})
		c.bus.Emit(state.InventorySlotEvent{WindowID: 0, Slot: 36, Item: protocol.ItemStack{Present: true, ItemID: 1, Count: 3}})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	res, err := c.PlaceBlockWithResult(ctx, protocol.BlockPos{X: 2, Y: 64, Z: 2}, protocol.DirectionUp)
	if err != nil {
		t.Fatalf("PlaceBlockWithResult success: %v", err)
	}
	if !res.Success {
		t.Fatal("expected Success=true")
	}
	if res.Target != (protocol.BlockPos{X: 2, Y: 64, Z: 2}) {
		t.Fatalf("Target = %+v, want target", res.Target)
	}
	if res.Support != (protocol.BlockPos{X: 2, Y: 63, Z: 2}) {
		t.Fatalf("Support = %+v, want support", res.Support)
	}
	if res.Face != protocol.DirectionUp {
		t.Fatalf("Face = %v, want up", res.Face)
	}
	if res.OldState.Name != "air" || res.NewState.Name != "stone" {
		t.Fatalf("states = old:%+v new:%+v, want air/stone", res.OldState, res.NewState)
	}
	if !res.HeldItem.Present || res.HeldItem.Name != "stone" {
		t.Fatalf("HeldItem = %+v, want stone", res.HeldItem)
	}
	if res.InventoryCountBefore != 4 || res.InventoryCountAfter != 3 {
		t.Fatalf("inventory counts = %d/%d, want 4/3", res.InventoryCountBefore, res.InventoryCountAfter)
	}
	if res.RollbackDetected {
		t.Fatal("expected RollbackDetected=false")
	}
	if res.Duration <= 0 {
		t.Fatalf("Duration = %s, want >0", res.Duration)
	}
}

func TestPlaceBlockWithResultPreservesRawError(t *testing.T) {
	c, _ := readyCoreActionClient(t)
	addFlatActionChunk(c)
	c.stateMu.Lock()
	c.player.X = 5.5
	c.player.Z = 5.5
	c.stateMu.Unlock()
	c.trackSelectedHotbarSlot(0)
	c.trackInventorySlot(36, protocol.ItemStack{Present: true, ItemID: 1, Count: 4})
	c.world.SetBlock(2, 64, 2, 1)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	res, err := c.PlaceBlockWithResult(ctx, protocol.BlockPos{X: 2, Y: 64, Z: 2}, protocol.DirectionUp)
	if !errors.Is(err, ErrBlockNotReplaceable) {
		t.Fatalf("error = %v, want ErrBlockNotReplaceable", err)
	}
	if res.Success {
		t.Fatal("expected Success=false")
	}
	if res.Target != (protocol.BlockPos{X: 2, Y: 64, Z: 2}) {
		t.Fatalf("Target = %+v, want target", res.Target)
	}
	if res.OldState.Name != "stone" {
		t.Fatalf("OldState = %+v, want stone", res.OldState)
	}
}

func TestPlaceBlockWithResultNoUpdateDoesNotMeanSuccess(t *testing.T) {
	c, _ := readyCoreActionClient(t)
	addFlatActionChunk(c)
	c.stateMu.Lock()
	c.player.X = 5.5
	c.player.Z = 5.5
	c.stateMu.Unlock()
	c.trackSelectedHotbarSlot(0)
	c.trackInventorySlot(36, protocol.ItemStack{Present: true, ItemID: 1, Count: 4})

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	res, err := c.PlaceBlockWithResult(ctx, protocol.BlockPos{X: 2, Y: 64, Z: 2}, protocol.DirectionUp)
	if !errors.Is(err, ErrBlockUpdateTimeout) && !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want ErrBlockUpdateTimeout or context deadline", err)
	}
	if res.Success {
		t.Fatal("expected Success=false when no block update arrives")
	}
}

func TestPlaceBlockWithResultRollbackDetected(t *testing.T) {
	c, _ := readyCoreActionClient(t)
	addFlatActionChunk(c)
	c.stateMu.Lock()
	c.player.X = 5.5
	c.player.Z = 5.5
	c.stateMu.Unlock()
	c.trackSelectedHotbarSlot(0)
	c.trackInventorySlot(36, protocol.ItemStack{Present: true, ItemID: 1, Count: 4})

	go func() {
		time.Sleep(25 * time.Millisecond)
		c.bus.Emit(state.BlockUpdateEvent{X: 2, Y: 64, Z: 2, StateID: 0})
		c.bus.Emit(state.InventorySlotEvent{WindowID: 0, Slot: 36, Item: protocol.ItemStack{Present: true, ItemID: 1, Count: 4}})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	res, err := c.PlaceBlockWithResult(ctx, protocol.BlockPos{X: 2, Y: 64, Z: 2}, protocol.DirectionUp)
	if !errors.Is(err, ErrPlacementRolledBack) {
		t.Fatalf("error = %v, want ErrPlacementRolledBack", err)
	}
	if !res.RollbackDetected {
		t.Fatal("expected RollbackDetected=true")
	}
	if res.Success {
		t.Fatal("expected Success=false on rollback")
	}
}

func TestPlaceBlockWithResultContextCancelUnsubscribes(t *testing.T) {
	c, _ := readyCoreActionClient(t)
	addFlatActionChunk(c)
	c.stateMu.Lock()
	c.player.X = 5.5
	c.player.Z = 5.5
	c.stateMu.Unlock()
	c.trackSelectedHotbarSlot(0)
	c.trackInventorySlot(36, protocol.ItemStack{Present: true, ItemID: 1, Count: 4})
	base := c.Events().HandlerCount()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := c.PlaceBlockWithResult(ctx, protocol.BlockPos{X: 2, Y: 64, Z: 2}, protocol.DirectionUp)
		done <- err
	}()
	waitForHandlerCountAtLeast(t, c, base+3)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("PlaceBlockWithResult did not return after cancel")
	}
	if got := c.Events().HandlerCount(); got != base {
		t.Fatalf("handler leak after cancel: base=%d got=%d", base, got)
	}
}

func TestPlaceBlockWithResultMatchesPlaceBlockSurvivalBehavior(t *testing.T) {
	c, _ := readyCoreActionClient(t)
	addFlatActionChunk(c)
	c.stateMu.Lock()
	c.player.X = 5.5
	c.player.Z = 5.5
	c.stateMu.Unlock()
	c.trackSelectedHotbarSlot(0)
	c.trackInventorySlot(36, protocol.ItemStack{Present: true, ItemID: 1, Count: 4})

	go func() {
		time.Sleep(25 * time.Millisecond)
		c.world.SetBlock(2, 64, 2, 1)
		c.bus.Emit(state.BlockUpdateEvent{X: 2, Y: 64, Z: 2, StateID: 1})
		c.bus.Emit(state.InventorySlotEvent{WindowID: 0, Slot: 36, Item: protocol.ItemStack{Present: true, ItemID: 1, Count: 3}})
	}()

	ctx1, cancel1 := context.WithTimeout(context.Background(), time.Second)
	defer cancel1()
	if err := c.PlaceBlockSurvival(ctx1, protocol.BlockPos{X: 2, Y: 64, Z: 2}, protocol.DirectionUp); err != nil {
		t.Fatalf("PlaceBlockSurvival: %v", err)
	}

	c.world.SetBlock(2, 64, 2, 0)
	go func() {
		time.Sleep(25 * time.Millisecond)
		c.world.SetBlock(2, 64, 2, 1)
		c.bus.Emit(state.BlockUpdateEvent{X: 2, Y: 64, Z: 2, StateID: 1})
		c.bus.Emit(state.InventorySlotEvent{WindowID: 0, Slot: 36, Item: protocol.ItemStack{Present: true, ItemID: 1, Count: 2}})
	}()

	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	res, err := c.PlaceBlockWithResult(ctx2, protocol.BlockPos{X: 2, Y: 64, Z: 2}, protocol.DirectionUp)
	if err != nil {
		t.Fatalf("PlaceBlockWithResult: %v", err)
	}
	if !res.Success {
		t.Fatal("expected Success=true")
	}
}
