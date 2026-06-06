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

// MovementOptions configures navigation settings like the movement profile to use.
type MovementOptions = executor.MovementOptions

// MovementStats contains statistics from a completed navigation.
type MovementStats = executor.MovementStats

func (c *Client) SetMovementProfile(profile MovementProfile) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.movementProfile = profile
}

func (c *Client) MovementProfile() MovementProfile {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	if c.movementProfile == "" {
		return MovementBotLike
	}
	return c.movementProfile
}

func (c *Client) LastMovementStats() MovementStats {
	c.statsTrackMu.RLock()
	defer c.statsTrackMu.RUnlock()
	return c.lastMovementStats
}

// Advanced: PositionSyncSeq returns the sequence ID of the last processed position packet.
func (c *Client) PositionSyncSeq() uint64 {
	return atomic.LoadUint64(&c.positionSyncSeq)
}

func (c *Client) ActiveOptions() MovementOptions {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.activeOptions
}

func (c *Client) NavigateToWithOptions(ctx context.Context, g goal.Goal, opts MovementOptions) error {
	return c.NavigateTo(ctx, g, opts)
}
