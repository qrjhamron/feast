package move

import (
	"math"

	"github.com/user/feastgo/pkg/world"
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
	if inWater(w, from) || inWater(w, dest) {
		return math.Inf(1)
	}

	// destination is passable
	if !w.IsPassable(dest[0], dest[1], dest[2]) {
		return math.Inf(1)
	}
	// space above destination (y+1) must be passable (need 2 blocks height for player)
	if !w.IsPassable(dest[0], dest[1]+1, dest[2]) {
		return math.Inf(1)
	}
	// destination's floor (y-1) is NOT passable (solid)
	if w.IsPassable(dest[0], dest[1]-1, dest[2]) {
		return math.Inf(1)
	}
	// space above from (y+1) should also be passable
	if !w.IsPassable(from[0], from[1]+1, from[2]) {
		return math.Inf(1)
	}

	return 1.0 * terrainMultiplier(w, dest[0], dest[1], dest[2])
}
