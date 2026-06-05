package move

import (
	"math"

	"github.com/user/feastgo/pkg/world"
)

// MoveParkour jumps a 1-block gap by moving forward two blocks on the same Y.
//
// This is a simplified "gap jump" primitive: it does not model sprinting or
// precise physics, but it enforces basic clearance and landing conditions.
type MoveParkour struct {
	Dx, Dz int // cardinal direction only: one of (±1,0) or (0,±1)
}

func (m MoveParkour) Destination(from [3]int) [3]int {
	return [3]int{from[0] + 2*m.Dx, from[1], from[2] + 2*m.Dz}
}

func (m MoveParkour) DesiredYaw(from [3]int) float32 {
	return yawForDelta(m.Dx, m.Dz)
}

func (m MoveParkour) Cost(w *world.World, from [3]int) float64 {
	if (m.Dx == 0) == (m.Dz == 0) { // both zero or both non-zero
		return math.Inf(1)
	}

	dest := m.Destination(from)
	mid := [3]int{from[0] + m.Dx, from[1], from[2] + m.Dz}

	// Headroom at start.
	if !w.IsPassable(from[0], from[1]+1, from[2]) {
		return math.Inf(1)
	}

	// Mid-air clearance over the gap.
	if !w.IsPassable(mid[0], mid[1], mid[2]) || !w.IsPassable(mid[0], mid[1]+1, mid[2]) {
		return math.Inf(1)
	}

	// Landing position + headroom.
	if !w.IsPassable(dest[0], dest[1], dest[2]) || !w.IsPassable(dest[0], dest[1]+1, dest[2]) {
		return math.Inf(1)
	}
	if w.IsPassable(dest[0], dest[1]-1, dest[2]) {
		return math.Inf(1)
	}

	return 4.0 * terrainMultiplier(w, dest[0], dest[1], dest[2])
}
