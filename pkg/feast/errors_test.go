package feast

import (
	"errors"
	"fmt"
	"testing"
)

func TestSentinelErrors(t *testing.T) {
	testCases := []struct {
		name string
		err  error
	}{
		{"ErrClientClosed", ErrClientClosed},
		{"ErrNotConnected", ErrNotConnected},
		{"ErrNotReady", ErrNotReady},
		{"ErrPositionNotSynced", ErrPositionNotSynced},
		{"ErrMovementTimeout", ErrMovementTimeout},
		{"ErrMovementStuck", ErrMovementStuck},
		{"ErrNoPath", ErrNoPath},
		{"ErrGravityTimeout", ErrGravityTimeout},
		{"ErrHeldItemUnknown", ErrHeldItemUnknown},
		{"ErrChunkNotLoaded", ErrChunkNotLoaded},
		{"ErrBlockNotReplaceable", ErrBlockNotReplaceable},
		{"ErrNoSupportBlock", ErrNoSupportBlock},
		{"ErrTargetOutOfReach", ErrTargetOutOfReach},
		{"ErrNoPlaceableBlock", ErrNoPlaceableBlock},
		{"ErrPlacementRolledBack", ErrPlacementRolledBack},
		{"ErrBlockUpdateTimeout", ErrBlockUpdateTimeout},
		{"ErrBlockAir", ErrBlockAir},
		{"ErrBlockUnbreakable", ErrBlockUnbreakable},
		{"ErrBreakTargetUnsafe", ErrBreakTargetUnsafe},
		{"ErrBreakOutOfReach", ErrBreakOutOfReach},
		{"ErrBreakRolledBack", ErrBreakRolledBack},
		{"ErrContainerNotOpen", ErrContainerNotOpen},
		{"ErrContainerSlotNotFound", ErrContainerSlotNotFound},
		{"ErrInventoryItemNotFound", ErrInventoryItemNotFound},
		{"ErrInventoryFull", ErrInventoryFull},
		{"ErrContainerFull", ErrContainerFull},
		{"ErrContainerActionTimeout", ErrContainerActionTimeout},
		{"ErrContainerRejected", ErrContainerRejected},
		{"ErrContainerOpenTimeout", ErrContainerOpenTimeout},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if !errors.Is(tc.err, tc.err) {
				t.Errorf("expected %v to match itself with errors.Is", tc.err)
			}

			wrapped := fmt.Errorf("wrapped error context: %w", tc.err)
			if !errors.Is(wrapped, tc.err) {
				t.Errorf("expected wrapped error to match %v with errors.Is", tc.err)
			}
		})
	}
}
