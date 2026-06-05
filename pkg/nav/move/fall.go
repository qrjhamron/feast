package move

import (
	"math"

	"github.com/qrjhamron/feast/pkg/world"
)

// MoveFall moves horizontally by (Dx,Dz) while dropping Dy blocks (negative).
type MoveFall struct {
	Dx, Dy, Dz int
}

func (m MoveFall) Destination(from [3]int) [3]int {
	return [3]int{from[0] + m.Dx, from[1] + m.Dy, from[2] + m.Dz}
}

func (m MoveFall) DesiredYaw(from [3]int) float32 {
	return yawForDelta(m.Dx, m.Dz)
}

func (m MoveFall) Cost(w *world.World, from [3]int) float64 {
	// Drop down Dy blocks (where Dy is negative)
	// Allow longer falls; caller can cap the set of candidate falls.
	if m.Dy >= 0 || m.Dy < -16 {
		return math.Inf(1)
	}

	dest := m.Destination(from)

	// space above from (y+1) must be passable
	if !w.IsPassable(from[0], from[1]+1, from[2]) {
		return math.Inf(1)
	}

	// The drop path must be passable (from y down to destination y)
	// We also ensure player height (y+1) is clear for the whole drop,
	// which means checking up to from[1]+1 for the adjacent block.
	for y := dest[1]; y <= from[1]+1; y++ {
		if !w.IsPassable(dest[0], y, dest[2]) {
			return math.Inf(1)
		}
	}

	// destination floor must be solid
	if w.IsPassable(dest[0], dest[1]-1, dest[2]) {
		return math.Inf(1)
	}

	drop := math.Abs(float64(m.Dy))
	cost := drop * 0.5
	if drop > 3 {
		// Fall damage kicks in after 3 blocks. This is a very rough penalty to
		// bias the search away from painful drops.
		cost += (drop - 3) * 1.0
	}
	return cost * terrainMultiplier(w, dest[0], dest[1], dest[2])
}
