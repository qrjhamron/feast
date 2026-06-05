package move

import (
	"math"

	"github.com/qrjhamron/feast/pkg/world"
)

// MoveSwim moves one block horizontally through water.
type MoveSwim struct {
	Dx, Dz int
}

func (m MoveSwim) Destination(from [3]int) [3]int {
	return [3]int{from[0] + m.Dx, from[1], from[2] + m.Dz}
}

func (m MoveSwim) DesiredYaw(from [3]int) float32 {
	return yawForDelta(m.Dx, m.Dz)
}

func (m MoveSwim) Cost(w *world.World, from [3]int) float64 {
	dest := m.Destination(from)

	if !inWater(w, from) || !inWater(w, dest) {
		return math.Inf(1)
	}
	if !isPassableOrWater(w, dest[0], dest[1], dest[2]) {
		return math.Inf(1)
	}
	if !isPassableOrWater(w, dest[0], dest[1]+1, dest[2]) {
		return math.Inf(1)
	}
	return 4.0
}

// SprintEnabled indicates whether this movement can be sprinted safely.
func (m MoveSwim) SprintEnabled() bool { return false }

// MoveSwimUp exits water onto a solid bank while stepping up one block.
type MoveSwimUp struct {
	Dx, Dz int
}

func (m MoveSwimUp) Destination(from [3]int) [3]int {
	return [3]int{from[0] + m.Dx, from[1] + 1, from[2] + m.Dz}
}

func (m MoveSwimUp) DesiredYaw(from [3]int) float32 {
	return yawForDelta(m.Dx, m.Dz)
}

func (m MoveSwimUp) Cost(w *world.World, from [3]int) float64 {
	dest := m.Destination(from)

	if !inWater(w, from) {
		return math.Inf(1)
	}
	// Require a solid bank block directly under destination feet.
	if w.IsPassable(dest[0], dest[1]-1, dest[2]) {
		return math.Inf(1)
	}
	if !w.IsPassable(dest[0], dest[1], dest[2]) || !w.IsPassable(dest[0], dest[1]+1, dest[2]) {
		return math.Inf(1)
	}
	return 3.0
}

// SprintEnabled indicates whether this movement can be sprinted safely.
func (m MoveSwimUp) SprintEnabled() bool { return false }

func inWater(w *world.World, pos [3]int) bool {
	if w == nil {
		return false
	}
	if feet, err := w.GetBlock(pos[0], pos[1], pos[2]); err == nil && isWaterName(feet.Name) {
		return true
	}
	if head, err := w.GetBlock(pos[0], pos[1]+1, pos[2]); err == nil && isWaterName(head.Name) {
		return true
	}
	return false
}

func isPassableOrWater(w *world.World, x, y, z int) bool {
	if w == nil {
		return false
	}
	if w.IsPassable(x, y, z) {
		return true
	}
	block, err := w.GetBlock(x, y, z)
	return err == nil && isWaterName(block.Name)
}
