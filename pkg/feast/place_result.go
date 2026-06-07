package feast

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/qrjhamron/feast/pkg/world"
)

// PlaceResult contains details about a block placement attempt.
//
// Success is true only when the server actually confirms the new block state.
// On failure, PlaceBlockWithResult returns the best-effort snapshot collected
// before the error alongside the original error.
type PlaceResult struct {
	Success bool

	Target  BlockPos
	Support BlockPos
	Face    Direction

	OldState world.BlockState
	NewState world.BlockState

	SelectedSlot         int
	HeldItem             ItemStack
	InventoryCountBefore int
	InventoryCountAfter  int

	RollbackDetected bool
	Duration         time.Duration
}

// PlaceBlockWithResult places a block in survival mode and returns a result
// snapshot describing the attempt.
//
// The action reuses the existing survival placement path. Success is reported
// only after the server sends an actual block update for the target position;
// a packet write or acknowledge alone is not enough.
func (c *Client) PlaceBlockWithResult(ctx context.Context, target BlockPos, face Direction) (PlaceResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	start := time.Now()
	res := PlaceResult{Target: target, Face: face}
	if c == nil {
		return res, ErrNotConnected
	}

	preflight, err := c.preparePlaceResult(target, face)
	if err != nil {
		preflight.Duration = time.Since(start)
		return preflight, err
	}
	res = preflight

	err = c.PlaceBlockSurvivalInternal(ctx, target, face)
	if c.world != nil {
		if block, blockErr := c.world.GetBlock(int(target.X), int(target.Y), int(target.Z)); blockErr == nil {
			res.NewState = block
		}
	}
	res.SelectedSlot = c.SelectedHotbarSlot()
	if held, ok := c.HeldItem(); ok {
		res.HeldItem = cloneItemStack(held)
		res.InventoryCountAfter = held.Count
	}
	if err != nil {
		res.Duration = time.Since(start)
		res.RollbackDetected = errors.Is(err, ErrPlacementRolledBack)
		return res, err
	}
	res.Success = true
	res.Duration = time.Since(start)
	return res, nil
}

func (c *Client) preparePlaceResult(target BlockPos, face Direction) (PlaceResult, error) {
	res := PlaceResult{Target: target, Face: face}
	if err := c.requireCoreActionReady(); err != nil {
		return res, err
	}

	dx, dy, dz := directionOffset(face)
	support := BlockPos{X: target.X + dx, Y: target.Y + dy, Z: target.Z + dz}
	res.Support = support

	if err := c.checkPlacementValidity(target); err != nil {
		return res, err
	}
	if c.blockCenterDistance(target) > 6.0 {
		return res, ErrTargetOutOfReach
	}

	oldState, err := c.world.GetBlock(int(target.X), int(target.Y), int(target.Z))
	if err != nil {
		return res, err
	}
	res.OldState = oldState
	if !c.world.IsReplaceable(world.BlockPos(target)) {
		return res, ErrBlockNotReplaceable
	}

	if err := c.world.RequireBlockLoaded(world.BlockPos(support)); err != nil {
		return res, err
	}
	supportState, err := c.world.GetBlock(int(support.X), int(support.Y), int(support.Z))
	if err != nil {
		return res, err
	}
	if !c.world.IsSolid(world.BlockPos(support)) {
		return res, fmt.Errorf("%w: %s", ErrNoSupportBlock, supportState.Name)
	}
	if c.OverlapsPlayer(target) {
		return res, fmt.Errorf("placement target overlaps player hitbox")
	}
	if c.world.IsEntityBlocking(blockAABB(target)) {
		return res, fmt.Errorf("placement target overlaps blocking entity hitbox")
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
			return res, ErrNoPlaceableBlock
		}
		selectedSlot = foundSlot
	}
	res.SelectedSlot = selectedSlot
	res.HeldItem = cloneItemStack(heldItem)
	res.InventoryCountBefore = heldItem.Count
	return res, nil
}
