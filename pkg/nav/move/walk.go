package move

import (
	"math"

	"github.com/qrjhamron/feast/pkg/world"
)

// MoveWalk moves one block horizontally (cardinal directions) without changing Y.
type MoveWalk struct {
	Dx, Dz int
}

func (m MoveWalk) Destination(from [3]int) [3]int {
	return [3]int{from[0] + m.Dx, from[1], from[2] + m.Dz}
}

func (m MoveWalk) DesiredYaw(from [3]int) float32 {
	return yawForDelta(m.Dx, m.Dz)
}

func (m MoveWalk) Cost(w *world.World, from [3]int) float64 {
	dest := m.Destination(from)

	// Order matters for the planner hot path: evaluate the cheapest checks that
	// reject the most candidate neighbors first, so walls and gaps are rejected
	// after a single world lookup instead of paying for the water probe (which
	// costs up to four GetBlock calls). Behavior is identical to checking water
	// first — water cells are still rejected, just last.

	// Destination feet block must be clear (rejects walls/solid blocks/lava).
	if !w.IsPassable(dest[0], dest[1], dest[2]) {
		return math.Inf(1)
	}
	// Destination floor (y-1) must be solid (rejects ledges/gaps).
	if w.IsPassable(dest[0], dest[1]-1, dest[2]) {
		return math.Inf(1)
	}
	// Headroom above destination (y+1) — the player is ~2 blocks tall.
	if !w.IsPassable(dest[0], dest[1]+1, dest[2]) {
		return math.Inf(1)
	}
	// Headroom above the origin so we can step out.
	if !w.IsPassable(from[0], from[1]+1, from[2]) {
		return math.Inf(1)
	}
	// Walking is a dry-land move; swimming handles water columns.
	if inWater(w, from) || inWater(w, dest) {
		return math.Inf(1)
	}

	return 1.0 * terrainMultiplier(w, dest[0], dest[1], dest[2])
}
