package move

import (
	"math"
	"strings"

	"github.com/qrjhamron/feast/pkg/world"
)

// MoveClimb moves one block upward while staying in the same XZ column.
type MoveClimb struct{}

func (m MoveClimb) Destination(from [3]int) [3]int {
	return [3]int{from[0], from[1] + 1, from[2]}
}

func (m MoveClimb) DesiredYaw(from [3]int) float32 {
	return 0
}

func (m MoveClimb) Cost(w *world.World, from [3]int) float64 {
	feet, err := w.GetBlock(from[0], from[1], from[2])
	if err != nil || !isClimbableBlock(feet) {
		return math.Inf(1)
	}

	dest := m.Destination(from)
	if !w.IsPassable(dest[0], dest[1], dest[2]) {
		return math.Inf(1)
	}
	if !w.IsPassable(dest[0], dest[1]+1, dest[2]) {
		return math.Inf(1)
	}
	return 1.0 / 0.1176
}

func isClimbableBlock(b world.BlockState) bool {
	name := strings.ToLower(b.Name)
	if name == "ladder" || strings.HasSuffix(name, ":ladder") || name == "vine" || strings.HasSuffix(name, ":vine") {
		return true
	}
	// 1.20.4 ladder/vine states are in this neighborhood in blocks.json.
	return b.ID >= 0x41B && b.ID <= 0x460
}
