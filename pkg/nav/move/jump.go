package move

import (
	"math"

	"github.com/user/feastgo/pkg/world"
)

// MoveJump moves one block horizontally (cardinal directions) while increasing Y by 1.
type MoveJump struct {
	Dx, Dz int
}

func (m MoveJump) Destination(from [3]int) [3]int {
	return [3]int{from[0] + m.Dx, from[1] + 1, from[2] + m.Dz}
}

func (m MoveJump) DesiredYaw(from [3]int) float32 {
	return yawForDelta(m.Dx, m.Dz)
}

func (m MoveJump) Cost(w *world.World, from [3]int) float64 {
	dest := m.Destination(from)

	// block above from (y+2) is passable (need headroom to jump)
	if !w.IsPassable(from[0], from[1]+2, from[2]) {
		return math.Inf(1)
	}
	// block above destination (y+2) is passable. Since dest[1] is from[1]+1, dest[1]+1 is from[1]+2
	if !w.IsPassable(dest[0], dest[1]+1, dest[2]) {
		return math.Inf(1)
	}
	// destination (y+1) is passable. Wait, prompt says destination (y+1). But dest is already y+1 relative to from.
	// So destination (which is dest[1]) is passable. Let's assume prompt meant "destination (y+1 of original) is passable".
	if !w.IsPassable(dest[0], dest[1], dest[2]) {
		return math.Inf(1)
	}
	// destination floor (y of original) is solid. Since dest[1] is from[1]+1, dest[1]-1 is from[1].
	if w.IsPassable(dest[0], dest[1]-1, dest[2]) {
		return math.Inf(1)
	}

	return 2.0 * terrainMultiplier(w, dest[0], dest[1], dest[2])
}
