package feast

import (
	"errors"

	"github.com/qrjhamron/feast/pkg/world"
)

// ─── Sentinel errors ──────────────────────────────────────────────────────────
//
// All sentinel errors are defined here so callers can check them with
// [errors.Is].  Wrapped variants (e.g. fmt.Errorf("...: %w", ErrNotConnected))
// still match.

var (
	// Client lifecycle errors.
	ErrClientClosed = errors.New("feast: client closed")
	ErrNotConnected = errors.New("feast: not connected")
	ErrNotReady     = errors.New("feast: not in play state")

	// Position / world readiness.
	ErrPositionNotSynced = errors.New("feast: position not synced")
	ErrChunkNotLoaded    = world.ErrChunkNotLoaded

	// Inventory and block-placement errors.
	ErrHeldItemUnknown     = errors.New("held item unknown")
	ErrBlockNotReplaceable = errors.New("block not replaceable")
	ErrNoSupportBlock      = errors.New("no support block")
	ErrTargetOutOfReach    = errors.New("target out of reach")
	ErrNoPlaceableBlock    = errors.New("no placeable block")
	ErrPlacementRolledBack = errors.New("placement rolled back")
	ErrBlockUpdateTimeout  = errors.New("block update timeout")
	ErrBlockAir            = errors.New("block is air")
	ErrBlockUnbreakable    = errors.New("block unbreakable")
	ErrBreakTargetUnsafe   = errors.New("break target unsafe")
	ErrBreakOutOfReach     = errors.New("break target out of reach")
	ErrBreakRolledBack     = errors.New("break rolled back")

	// Navigation errors.
	ErrMovementTimeout = errors.New("feast: movement timeout")
	ErrMovementStuck   = errors.New("feast: movement stuck")
	ErrNoPath          = errors.New("feast: no path found")
	ErrGravityTimeout  = errors.New("feast: gravity fall timeout")

	// Container errors.
	ErrContainerNotOpen       = errors.New("container not open")
	ErrContainerSlotNotFound  = errors.New("container slot not found")
	ErrInventoryItemNotFound  = errors.New("inventory item not found")
	ErrInventoryFull          = errors.New("inventory full")
	ErrContainerFull          = errors.New("container full")
	ErrContainerActionTimeout = errors.New("container action timeout")
	ErrContainerRejected      = errors.New("container rejected action")
	ErrContainerOpenTimeout   = errors.New("feast: container open timeout")
)
