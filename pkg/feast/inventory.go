package feast

import (
	"context"
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/registry"
	"github.com/qrjhamron/feast/pkg/state"
	"github.com/qrjhamron/feast/pkg/world"
)

var (
// Errors moved to errors.go
)

const (
	playerInventoryHotbarStart int16 = 36
	playerInventoryHotbarEnd   int16 = 44

	// CreativeSmokeMode is the placement-plan mode used by creative smoke helpers.
	CreativeSmokeMode = "creative_smoke"
	// CreativeSmokeItemName is the default block item used by creative smoke helpers.
	CreativeSmokeItemName = "stone"
	// CreativeSmokeItemID is the protocol item ID for CreativeSmokeItemName.
	CreativeSmokeItemID = int32(1)
)

// ItemStack represents an item in an inventory slot.
type ItemStack = registry.ItemStack

// InventoryState represents a thread-safe snapshot of the bot's inventory.
type InventoryState struct {
	// SelectedHotbarSlot is the currently active hotbar slot (0-8).
	SelectedHotbarSlot int
	// Slots contains all items, keyed by slot index.
	Slots map[int]ItemStack
}

// PlacementPlan describes a precomputed creative placement operation.
//
// Advanced: this is primarily for smoke tests and protocol diagnostics. Normal
// callers should use [Client.PlaceBlockCreative] or [Client.PlaceBlockSurvival].
type PlacementPlan struct {
	// Target is the block position expected to change.
	Target protocol.BlockPos
	// Support is the neighboring support block clicked by the placement packet.
	Support protocol.BlockPos
	// Face is the clicked support face.
	Face byte
	// OldState is the target state before placement.
	OldState world.BlockState
	// HeldItemName is the item name selected for placement.
	HeldItemName string
	// HeldItemID is the protocol item ID selected for placement.
	HeldItemID int32
	// SelectedSlot is the hotbar slot selected for placement.
	SelectedSlot int
	// InventorySlot is the absolute player-inventory slot used for the item.
	InventorySlot int16
	// Sequence is the block-change sequence sent to the server.
	Sequence int32
	// Mode identifies the plan source.
	Mode string
	// ExpectedPacket is the final packet sent to perform placement.
	ExpectedPacket protocol.Packet
}

var placementSequence int32

// debugPlaceSurvival prints a [place-survival] log line only when Debug is enabled.
func (c *Client) debugPlaceSurvival(format string, args ...any) {
	c.debugActionf("place-survival", format, args...)
}

// Inventory returns a pointer to a thread-safe copy of the client's inventory state.
func (c *Client) Inventory() *InventoryState {
	c.inventoryMu.RLock()
	defer c.inventoryMu.RUnlock()
	return c.getInventoryStateCopy()
}

func (c *Client) getInventoryStateCopy() *InventoryState {
	stateCopy := &InventoryState{
		SelectedHotbarSlot: c.inventory.SelectedHotbarSlot,
		Slots:              make(map[int]ItemStack),
	}
	for k, v := range c.inventory.Slots {
		stateCopy.Slots[k] = v
	}
	return stateCopy
}

// InventorySnapshot returns a thread-safe copy of the client's inventory state.
func (c *Client) InventorySnapshot() InventoryState {
	c.inventoryMu.RLock()
	defer c.inventoryMu.RUnlock()
	stateCopy := InventoryState{
		SelectedHotbarSlot: c.inventory.SelectedHotbarSlot,
		Slots:              make(map[int]ItemStack),
	}
	for k, v := range c.inventory.Slots {
		stateCopy.Slots[k] = v
	}
	return stateCopy
}

// SelectedHotbarSlot returns the currently selected hotbar slot (0-8).
func (c *Client) SelectedHotbarSlot() int {
	c.inventoryMu.RLock()
	defer c.inventoryMu.RUnlock()
	return c.inventory.SelectedHotbarSlot
}

// HeldItem returns the held item from the selected slot and true if present.
func (c *Client) HeldItem() (ItemStack, bool) {
	c.inventoryMu.RLock()
	defer c.inventoryMu.RUnlock()
	slot := 36 + c.inventory.SelectedHotbarSlot
	stack, ok := c.inventory.Slots[slot]
	if !ok || !stack.Present {
		return ItemStack{Present: false}, false
	}
	return stack, true
}

// SelectHotbarSlot changes the active hotbar slot.
func (c *Client) SelectHotbarSlot(ctx context.Context, slot int) error {
	if slot < 0 || slot > 8 {
		return fmt.Errorf("invalid hotbar slot %d (must be 0-8)", slot)
	}
	err := c.writePacket(&protocol.PlayServerboundSetHeldItemPacket{
		Slot: int16(slot),
	})
	if err != nil {
		return fmt.Errorf("failed to send set held item packet: %w", err)
	}

	c.inventoryMu.Lock()
	c.inventory.SelectedHotbarSlot = slot
	c.inventoryMu.Unlock()
	return nil
}

// FindHotbarItem searches the hotbar (slots 36-44) for an item with the given name.
func (c *Client) FindHotbarItem(name string) (slot int, stack ItemStack, ok bool) {
	c.inventoryMu.RLock()
	defer c.inventoryMu.RUnlock()
	for i := 0; i < 9; i++ {
		invSlot := 36 + i
		st, present := c.inventory.Slots[invSlot]
		if present && st.Present && st.Name == name {
			return i, st, true
		}
	}
	return -1, ItemStack{}, false
}

func (c *Client) trackSelectedHotbarSlot(slot int) {
	if slot < 0 || slot > 8 {
		return
	}
	c.inventoryMu.Lock()
	c.inventory.SelectedHotbarSlot = slot
	c.inventoryMu.Unlock()
}

func (c *Client) trackInventorySlot(slot int16, item protocol.ItemStack) {
	c.trackInventorySlotWithSource(slot, item, "server")
}

func (c *Client) trackInventorySlotWithSource(slot int16, item protocol.ItemStack, source string) {
	c.inventoryMu.Lock()
	defer c.inventoryMu.Unlock()
	if c.inventory.Slots == nil {
		c.inventory.Slots = make(map[int]ItemStack)
	}
	if !item.Present {
		c.inventory.Slots[int(slot)] = ItemStack{Present: false, Source: source}
	} else {
		name, _ := ItemNameFromID(item.ItemID)
		c.inventory.Slots[int(slot)] = ItemStack{
			Present: true,
			ItemID:  item.ItemID,
			Name:    name,
			Count:   int(item.Count),
			NBT:     item.NBT,
			Source:  source,
		}
	}
}

func (c *Client) trackContainerContent(windowID int32, slots []protocol.ItemStack) {
	if windowID != 0 {
		return
	}
	c.inventoryMu.Lock()
	defer c.inventoryMu.Unlock()
	if c.inventory.Slots == nil {
		c.inventory.Slots = make(map[int]ItemStack)
	}
	for slot, item := range slots {
		if !item.Present {
			c.inventory.Slots[slot] = ItemStack{Present: false, Source: "server"}
		} else {
			name, _ := ItemNameFromID(item.ItemID)
			c.inventory.Slots[slot] = ItemStack{
				Present: true,
				ItemID:  item.ItemID,
				Name:    name,
				Count:   int(item.Count),
				NBT:     item.NBT,
				Source:  "server",
			}
		}
	}
}

// OverlapsPlayer reports whether target would intersect the bot's current
// player hitbox.
//
// Advanced: normal placement calls run this safety check automatically.
func (c *Client) OverlapsPlayer(target protocol.BlockPos) bool {
	c.stateMu.RLock()
	px, py, pz := c.player.X, c.player.Y, c.player.Z
	c.stateMu.RUnlock()

	// Player bounding box: width=0.6 (X/Z +/- 0.3), height=1.8 (Y to Y+1.8)
	// Target block bounding box: [X, X+1], [Y, Y+1], [Z, Z+1]
	overlapX := float64(target.X) < px+0.3 && float64(target.X+1) > px-0.3
	overlapY := float64(target.Y) < py+1.8 && float64(target.Y+1) > py
	overlapZ := float64(target.Z) < pz+0.3 && float64(target.Z+1) > pz-0.3

	return overlapX && overlapY && overlapZ
}

func (c *Client) checkPlacementValidity(target protocol.BlockPos) error {
	if c.world == nil {
		return world.ErrBlockNotFound
	}
	return c.world.RequireBlockLoaded(world.BlockPos(target))
}

// PlaceBlock sends a low-level UseItemOn packet for the currently held item.
//
// Advanced: this method does not wait for server confirmation. Prefer
// [Client.PlaceBlockSurvival] or [Client.PlaceBlockCreative] for public bot
// actions.
func (c *Client) PlaceBlock(target protocol.BlockPos, face byte) error {
	c.inventoryMu.RLock()
	slot := 36 + c.inventory.SelectedHotbarSlot
	stack, ok := c.inventory.Slots[slot]
	c.inventoryMu.RUnlock()

	if !ok || !stack.Present {
		return ErrHeldItemUnknown
	}
	if c.OverlapsPlayer(target) {
		return fmt.Errorf("placement target overlaps player hitbox")
	}
	if err := c.checkPlacementValidity(target); err != nil {
		return err
	}
	return c.writePacket(&protocol.PlayServerboundUseItemOnPacket{
		Hand:        protocol.MainHand,
		Position:    target,
		Face:        face,
		CursorX:     0.5,
		CursorY:     0.5,
		CursorZ:     0.5,
		InsideBlock: false,
		Sequence:    int32(atomic.AddInt32(&placementSequence, 1)),
	})
}

func (c *Client) requireCoreActionReady() error {
	if c.conn == nil {
		return ErrNotConnected
	}
	if c.CurrentState() != state.StatePlay {
		return fmt.Errorf("%w: state=%v", ErrNotReady, c.CurrentState())
	}
	if !c.PositionSynced() {
		return ErrPositionNotSynced
	}
	return nil
}

func (c *Client) blockCenterDistance(pos protocol.BlockPos) float64 {
	c.stateMu.RLock()
	px, py, pz := c.player.X, c.player.Y, c.player.Z
	c.stateMu.RUnlock()
	dx := float64(pos.X) + 0.5 - px
	dy := float64(pos.Y) + 0.5 - py
	dz := float64(pos.Z) + 0.5 - pz
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

func blockAABB(pos protocol.BlockPos) world.AABB {
	return world.AABB{
		MinX: float64(pos.X),
		MinY: float64(pos.Y),
		MinZ: float64(pos.Z),
		MaxX: float64(pos.X) + 1,
		MaxY: float64(pos.Y) + 1,
		MaxZ: float64(pos.Z) + 1,
	}
}

func directionOffset(dir protocol.Direction) (dx, dy, dz int32) {
	switch dir {
	case protocol.DirectionDown:
		return 0, 1, 0
	case protocol.DirectionUp:
		return 0, -1, 0
	case protocol.DirectionNorth:
		return 0, 0, 1
	case protocol.DirectionSouth:
		return 0, 0, -1
	case protocol.DirectionWest:
		return 1, 0, 0
	case protocol.DirectionEast:
		return -1, 0, 0
	default:
		return 0, 0, 0
	}
}

// Advanced: PlaceBlockSurvivalInternal performs a block placement and waits for confirmation.
func (c *Client) PlaceBlockSurvivalInternal(ctx context.Context, target protocol.BlockPos, face protocol.Direction) error {
	if err := c.requireCoreActionReady(); err != nil {
		return err
	}
	c.inventoryMu.RLock()
	selectedSlot := c.inventory.SelectedHotbarSlot
	heldSlot := 36 + selectedSlot
	heldItem, ok := c.inventory.Slots[heldSlot]
	c.inventoryMu.RUnlock()

	if !ok || !heldItem.Present || !IsPlaceableBlockItem(heldItem) {
		foundSlot := -1
		c.inventoryMu.RLock()
		for i := 0; i < 9; i++ {
			st, present := c.inventory.Slots[36+i]
			if present && st.Present && IsPlaceableBlockItem(st) {
				foundSlot = i
				heldItem = st
				break
			}
		}
		c.inventoryMu.RUnlock()

		if foundSlot == -1 {
			c.debugPlaceSurvival("result=FAIL reason=no_placeable_block_in_hotbar")
			return ErrNoPlaceableBlock
		}

		if err := c.SelectHotbarSlot(ctx, foundSlot); err != nil {
			c.debugPlaceSurvival("result=FAIL reason=failed_to_select_hotbar_slot")
			return err
		}
		selectedSlot = foundSlot
		heldSlot = 36 + selectedSlot
	}

	blockName, _ := BlockNameFromItem(heldItem)
	countBefore := heldItem.Count

	c.debugPlaceSurvival("mode=full_inventory")
	c.debugPlaceSurvival("selected_slot=%d", selectedSlot)
	c.debugPlaceSurvival("held_item=%s", blockName)
	c.debugPlaceSurvival("target=%d,%d,%d", target.X, target.Y, target.Z)

	dx, dy, dz := directionOffset(face)
	support := protocol.BlockPos{X: target.X + dx, Y: target.Y + dy, Z: target.Z + dz}
	c.debugPlaceSurvival("support=%d,%d,%d", support.X, support.Y, support.Z)

	faceStr := "top"
	switch face {
	case protocol.DirectionDown:
		faceStr = "bottom"
	case protocol.DirectionUp:
		faceStr = "top"
	case protocol.DirectionNorth:
		faceStr = "north"
	case protocol.DirectionSouth:
		faceStr = "south"
	case protocol.DirectionWest:
		faceStr = "west"
	case protocol.DirectionEast:
		faceStr = "east"
	}
	c.debugPlaceSurvival("face=%s", faceStr)

	if err := c.checkPlacementValidity(target); err != nil {
		c.debugPlaceSurvival("result=FAIL reason=chunk_not_loaded_or_invalid")
		return err
	}
	if c.blockCenterDistance(target) > 6.0 {
		c.debugPlaceSurvival("result=FAIL reason=target_out_of_reach")
		return ErrTargetOutOfReach
	}

	oldState, err := c.world.GetBlock(int(target.X), int(target.Y), int(target.Z))
	if err != nil {
		c.debugPlaceSurvival("result=FAIL reason=world_get_block_failed")
		return err
	}
	c.debugPlaceSurvival("old_state=%s", oldState.Name)
	if !c.world.IsReplaceable(world.BlockPos(target)) {
		c.debugPlaceSurvival("result=FAIL reason=target_not_replaceable")
		return ErrBlockNotReplaceable
	}

	if err := c.world.RequireBlockLoaded(world.BlockPos(support)); err != nil {
		c.debugPlaceSurvival("result=FAIL reason=support_chunk_not_loaded")
		return err
	}
	supportState, err := c.world.GetBlock(int(support.X), int(support.Y), int(support.Z))
	if err != nil {
		c.debugPlaceSurvival("result=FAIL reason=support_block_not_found")
		return err
	}
	if !c.world.IsSolid(world.BlockPos(support)) {
		c.debugPlaceSurvival("result=FAIL reason=support_block_not_solid")
		return fmt.Errorf("%w: %s", ErrNoSupportBlock, supportState.Name)
	}

	if c.OverlapsPlayer(target) {
		c.debugPlaceSurvival("result=FAIL reason=overlaps_player_hitbox")
		return fmt.Errorf("placement target overlaps player hitbox")
	}
	if c.world.IsEntityBlocking(blockAABB(target)) {
		c.debugPlaceSurvival("result=FAIL reason=overlaps_entity_hitbox")
		return fmt.Errorf("placement target overlaps blocking entity hitbox")
	}

	updateCh := make(chan state.BlockUpdateEvent, 16)
	slotCh := make(chan state.InventorySlotEvent, 4)
	blockHandlerID, _ := c.On("block_update", func(e state.Event) {
		if ev, ok := e.(state.BlockUpdateEvent); ok {
			select {
			case updateCh <- ev:
			default:
			}
		}
	})
	sectionHandlerID, _ := c.On("section_blocks_update", func(e state.Event) {
		ev, ok := e.(state.SectionBlocksUpdateEvent)
		if !ok {
			return
		}
		for _, u := range ev.Updates {
			select {
			case updateCh <- state.BlockUpdateEvent{X: u.X, Y: u.Y, Z: u.Z, StateID: u.StateID}:
			default:
			}
		}
	})
	slotHandlerID, _ := c.On("inventory_slot", func(e state.Event) {
		if ev, ok := e.(state.InventorySlotEvent); ok && ev.WindowID == 0 && int(ev.Slot) == heldSlot {
			select {
			case slotCh <- ev:
			default:
			}
		}
	})
	defer c.Events().Off(blockHandlerID)
	defer c.Events().Off(sectionHandlerID)
	defer c.Events().Off(slotHandlerID)

	seq := int32(atomic.AddInt32(&placementSequence, 1))
	err = c.writePacket(&protocol.PlayServerboundUseItemOnPacket{
		Hand:        protocol.MainHand,
		Position:    support,
		Face:        byte(face),
		CursorX:     0.5,
		CursorY:     0.5,
		CursorZ:     0.5,
		InsideBlock: false,
		Sequence:    seq,
	})
	c.debugPlaceSurvival("packet_sent=%v", err == nil)
	if err != nil {
		c.debugPlaceSurvival("result=FAIL reason=packet_write_failed")
		return err
	}

	updateReceived := false
	slotUpdateReceived := false
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	for !updateReceived {
		select {
		case <-ctx.Done():
			c.debugPlaceSurvival("result=FAIL reason=context_cancelled")
			return ctx.Err()
		case <-deadline.C:
			c.debugPlaceSurvival("result=FAIL reason=block_update_timeout")
			return ErrBlockUpdateTimeout
		case ev := <-slotCh:
			slotUpdateReceived = true
			heldItem.Count = int(ev.Item.Count)
		case ev := <-updateCh:
			if ev.X == target.X && ev.Y == target.Y && ev.Z == target.Z {
				updateReceived = true
			}
		}
	}
	drain := true
	for drain {
		select {
		case ev := <-slotCh:
			slotUpdateReceived = true
			heldItem.Count = int(ev.Item.Count)
		default:
			drain = false
		}
	}

	c.debugPlaceSurvival("block_update_received=%v", updateReceived)

	newState, err := c.world.GetBlock(int(target.X), int(target.Y), int(target.Z))
	if err != nil {
		c.debugPlaceSurvival("result=FAIL reason=world_get_block_failed")
		return err
	}
	c.debugPlaceSurvival("new_state=%s", newState.Name)

	_, err = c.world.GetBlock(int(target.X), int(target.Y), int(target.Z))
	rollbackDetected := err == nil && c.world.IsReplaceable(world.BlockPos(target))
	c.debugPlaceSurvival("rollback_detected=%v", rollbackDetected)

	c.inventoryMu.RLock()
	countAfter := c.inventory.Slots[heldSlot].Count
	c.inventoryMu.RUnlock()
	c.debugPlaceSurvival("inventory_count_before=%d", countBefore)
	c.debugPlaceSurvival("inventory_count_after=%d", countAfter)
	if slotUpdateReceived && countAfter >= countBefore {
		c.debugPlaceSurvival("result=FAIL reason=inventory_count_not_decremented")
		return ErrPlacementRolledBack
	}

	if c.world.IsReplaceable(world.BlockPos(target)) || rollbackDetected {
		c.debugPlaceSurvival("result=FAIL reason=rollback_or_no_update")
		return ErrPlacementRolledBack
	}

	c.debugPlaceSurvival("result=PASS")
	return nil
}

// Advanced: PrepareCreativeSmokePlacement generates a plan for creative block placement.
func (c *Client) PrepareCreativeSmokePlacement(target protocol.BlockPos) (PlacementPlan, error) {
	if c.world == nil {
		return PlacementPlan{}, world.ErrBlockNotFound
	}
	if c.OverlapsPlayer(target) {
		return PlacementPlan{}, fmt.Errorf("placement target overlaps player hitbox")
	}
	if err := c.checkPlacementValidity(target); err != nil {
		return PlacementPlan{}, err
	}
	oldState, err := c.world.GetBlock(int(target.X), int(target.Y), int(target.Z))
	if err != nil {
		return PlacementPlan{}, err
	}
	if oldState.Name != "air" {
		return PlacementPlan{}, fmt.Errorf("target is not air: %s", oldState.Name)
	}
	support := protocol.BlockPos{X: target.X, Y: target.Y - 1, Z: target.Z}
	supportState, err := c.world.GetBlock(int(support.X), int(support.Y), int(support.Z))
	if err != nil {
		return PlacementPlan{}, err
	}
	if !supportState.Solid {
		return PlacementPlan{}, fmt.Errorf("support is not solid: %s", supportState.Name)
	}
	seq := int32(atomic.AddInt32(&placementSequence, 1))
	return PlacementPlan{
		Target:        target,
		Support:       support,
		Face:          protocol.BlockFaceTop,
		OldState:      oldState,
		HeldItemName:  CreativeSmokeItemName,
		HeldItemID:    CreativeSmokeItemID,
		SelectedSlot:  0,
		InventorySlot: playerInventoryHotbarStart,
		Sequence:      seq,
		Mode:          CreativeSmokeMode,
		ExpectedPacket: &protocol.PlayServerboundUseItemOnPacket{
			Hand:        protocol.MainHand,
			Position:    support,
			Face:        protocol.BlockFaceTop,
			CursorX:     0.5,
			CursorY:     1.0,
			CursorZ:     0.5,
			InsideBlock: false,
			Sequence:    seq,
		},
	}, nil
}

// Advanced: ExecuteCreativeSmokePlacement executes a pre-planned creative placement.
func (c *Client) ExecuteCreativeSmokePlacement(plan PlacementPlan) error {
	if err := c.writePacket(&protocol.PlayServerboundSetCreativeModeSlotPacket{
		Slot: plan.InventorySlot,
		Item: protocol.ItemStack{Present: true, ItemID: plan.HeldItemID, Count: 64, NBT: []byte{0x00}},
	}); err != nil {
		return err
	}
	c.trackSelectedHotbarSlot(plan.SelectedSlot)
	c.trackInventorySlotWithSource(plan.InventorySlot, protocol.ItemStack{Present: true, ItemID: plan.HeldItemID, Count: 64, NBT: []byte{0x00}}, "creative_smoke")
	if err := c.writePacket(&protocol.PlayServerboundSetHeldItemPacket{Slot: int16(plan.SelectedSlot)}); err != nil {
		return err
	}
	return c.writePacket(plan.ExpectedPacket)
}

// ItemNameFromID returns the registry item name for a Protocol 765 item ID.
func ItemNameFromID(id int32) (string, bool) {
	return registry.ItemNameFromID(id)
}

// BlockNameFromItem returns the block name placed by item, if item represents a
// placeable block.
func BlockNameFromItem(item ItemStack) (string, bool) {
	return registry.BlockNameFromItem(item)
}

// IsPlaceableBlockItem reports whether item represents a block that can be
// placed into the world.
func IsPlaceableBlockItem(item ItemStack) bool {
	return registry.IsPlaceableBlockItem(item)
}
