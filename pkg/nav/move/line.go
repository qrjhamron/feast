package move

import (
	"math"

	"github.com/qrjhamron/feast/pkg/world"
)

// MoveWalkLine moves multiple blocks in a straight cardinal line on the XZ
// plane (no Y change). It is used for path smoothing.
type MoveWalkLine struct {
	Dx, Dz int // cardinal direction: one of (±1,0) or (0,±1)
	Steps  int // number of blocks to move
}

func (m MoveWalkLine) Destination(from [3]int) [3]int {
	return [3]int{from[0] + m.Dx*m.Steps, from[1], from[2] + m.Dz*m.Steps}
}

func (m MoveWalkLine) DesiredYaw(from [3]int) float32 {
	return yawForDelta(m.Dx, m.Dz)
}

func (m MoveWalkLine) Cost(w *world.World, from [3]int) float64 {
	if m.Steps <= 0 {
		return math.Inf(1)
	}
	if (m.Dx == 0) == (m.Dz == 0) {
		return math.Inf(1)
	}

	// Validate each intermediate step is walkable and accumulate cost.
	pos := from
	total := 0.0
	for i := 0; i < m.Steps; i++ {
		step := MoveWalk{Dx: m.Dx, Dz: m.Dz}
		c := step.Cost(w, pos)
		if math.IsInf(c, 1) {
			return math.Inf(1)
		}
		total += c
		pos = step.Destination(pos)
	}
	return total
}

// MoveWalkDiagonalLine moves multiple blocks in a straight diagonal line on the
// XZ plane (no Y change) with corner-cutting checks per step.
type MoveWalkDiagonalLine struct {
	Dx, Dz int // diagonal direction: one of (±1,±1)
	Steps  int
}

func (m MoveWalkDiagonalLine) Destination(from [3]int) [3]int {
	return [3]int{from[0] + m.Dx*m.Steps, from[1], from[2] + m.Dz*m.Steps}
}

func (m MoveWalkDiagonalLine) DesiredYaw(from [3]int) float32 {
	return yawForDelta(m.Dx, m.Dz)
}

func (m MoveWalkDiagonalLine) Cost(w *world.World, from [3]int) float64 {
	if m.Steps <= 0 {
		return math.Inf(1)
	}
	if m.Dx == 0 || m.Dz == 0 {
		return math.Inf(1)
	}

	pos := from
	total := 0.0
	for i := 0; i < m.Steps; i++ {
		step := MoveWalkDiagonal{Dx: m.Dx, Dz: m.Dz}
		c := step.Cost(w, pos)
		if math.IsInf(c, 1) {
			return math.Inf(1)
		}
		total += c
		pos = step.Destination(pos)
	}
	return total
}
