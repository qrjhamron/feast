package move

import (
	"math"

	"github.com/user/feastgo/pkg/world"
)

// MoveWalkDiagonal moves one block diagonally on the XZ plane (no Y change).
//
// Corner cutting is forbidden: both adjacent cardinal cells must be passable
// (including headroom) to prevent clipping through corners.
type MoveWalkDiagonal struct {
	Dx, Dz int // must be in {-1, +1}
}

func (m MoveWalkDiagonal) Destination(from [3]int) [3]int {
	return [3]int{from[0] + m.Dx, from[1], from[2] + m.Dz}
}

func (m MoveWalkDiagonal) DesiredYaw(from [3]int) float32 {
	return yawForDelta(m.Dx, m.Dz)
}

func (m MoveWalkDiagonal) Cost(w *world.World, from [3]int) float64 {
	if m.Dx == 0 || m.Dz == 0 {
		return math.Inf(1)
	}

	dest := m.Destination(from)
	if inWater(w, from) || inWater(w, dest) {
		return math.Inf(1)
	}

	// Destination checks match MoveWalk.
	if !w.IsPassable(dest[0], dest[1], dest[2]) {
		return math.Inf(1)
	}
	if !w.IsPassable(dest[0], dest[1]+1, dest[2]) {
		return math.Inf(1)
	}
	if w.IsPassable(dest[0], dest[1]-1, dest[2]) {
		return math.Inf(1)
	}
	if !w.IsPassable(from[0], from[1]+1, from[2]) {
		return math.Inf(1)
	}

	// Prevent corner cutting: both adjacent cardinal squares must be traversable.
	adj1 := [3]int{from[0] + m.Dx, from[1], from[2]}
	adj2 := [3]int{from[0], from[1], from[2] + m.Dz}
	if !w.IsPassable(adj1[0], adj1[1], adj1[2]) || !w.IsPassable(adj1[0], adj1[1]+1, adj1[2]) {
		return math.Inf(1)
	}
	if !w.IsPassable(adj2[0], adj2[1], adj2[2]) || !w.IsPassable(adj2[0], adj2[1]+1, adj2[2]) {
		return math.Inf(1)
	}
	// Ensure the diagonally-adjacent floors exist too (standing height).
	if w.IsPassable(adj1[0], adj1[1]-1, adj1[2]) || w.IsPassable(adj2[0], adj2[1]-1, adj2[2]) {
		return math.Inf(1)
	}

	return math.Sqrt2 * terrainMultiplier(w, dest[0], dest[1], dest[2])
}
