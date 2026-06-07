package feast

import (
	"context"
	"sync/atomic"

	"github.com/qrjhamron/feast/pkg/nav/executor"
	"github.com/qrjhamron/feast/pkg/nav/goal"
)

// MovementProfile configures the pathfinder's movement heuristics.
type MovementProfile = executor.MovementProfile

const (
	// MovementBotLike creates a profile optimized for fastest direct paths (teleport/skip bounds).
	MovementBotLike MovementProfile = executor.MovementBotLike
	// MovementHumanLike creates a profile simulating walking (stepping, avoiding direct diagonal clips).
	MovementHumanLike MovementProfile = executor.MovementHumanLike
)

// MovementOptions configures one navigation attempt.
//
// The zero value uses the client's active movement profile.
type MovementOptions = executor.MovementOptions

// MovementStats contains statistics from a completed navigation.
type MovementStats = executor.MovementStats

// SetMovementProfile sets the default movement profile used by future
// navigation calls that do not provide [MovementOptions].
func (c *Client) SetMovementProfile(profile MovementProfile) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.movementProfile = profile
}

// MovementProfile returns the client's default movement profile.
func (c *Client) MovementProfile() MovementProfile {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if c.movementProfile == "" {
		return MovementBotLike
	}
	return c.movementProfile
}

// LastMovementStats returns the most recent movement statistics snapshot.
func (c *Client) LastMovementStats() MovementStats {
	c.statsTrackMu.RLock()
	defer c.statsTrackMu.RUnlock()
	return c.lastMovementStats
}

// Advanced: PositionSyncSeq returns the sequence ID of the last processed position packet.
func (c *Client) PositionSyncSeq() uint64 {
	return atomic.LoadUint64(&c.positionSyncSeq)
}

// ActiveOptions returns the movement options used by the current or most recent
// navigation attempt.
//
// Advanced: this is primarily useful for diagnostics and test harnesses.
func (c *Client) ActiveOptions() MovementOptions {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.activeOptions
}

// NavigateToWithOptions is a compatibility wrapper for [Client.NavigateTo].
//
// Prefer passing options directly to NavigateTo or [Client.NavigateWithResult].
func (c *Client) NavigateToWithOptions(ctx context.Context, g goal.Goal, opts MovementOptions) error {
	return c.NavigateTo(ctx, g, opts)
}
