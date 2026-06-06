package world

import "math"

// Position semantics convention (used consistently across navigation, movement,
// and scaffolding):
//
//   - A player's position Y is the FEET Y coordinate (the Y of the block the
//     player is standing in, fractional while falling/jumping).
//   - Feet block   = floor(player X, Y, Z).
//   - Ground block = feet block with Y-1 (the supporting block below the feet).
//   - Head block   = feet block with Y+1 (the upper half of the 2-block-tall
//     player hitbox).
//   - Standing node   = the integer feet block coordinate.
//   - Standing center = (feetX+0.5, feetY, feetZ+0.5); the X/Z are centered on
//     the block, Y stays at the feet (block top) level.
//
// IMPORTANT: a player's current/start position must always be derived from the
// live position snapshot via FeetBlockFromPosition. The motion-blocking
// "surface Y" cache (GetSurfaceY) describes the top solid block of a terrain
// column and must only be used when selecting an EXTERNAL candidate target from
// terrain — never to re-derive where the bot currently is. Mixing "surface top
// block Y" with "feet Y" is what produces impossible routes when the bot is
// underground or in a cave.

// OnGroundEpsilon is the maximum distance (in blocks) the feet may sit above a
// block top while still being considered "resting" on it. Server-side movement
// is collision-snapped, so anything beyond a tiny epsilon means the player is
// mid-air (falling or jumping) and on_ground must be reported false.
const OnGroundEpsilon = 1.0e-4

// FeetBlockFromPosition returns the integer feet block for a world position.
// The player position Y is the feet Y, so this is a straight floor of each axis.
func FeetBlockFromPosition(pos Vec3) BlockPos {
	return BlockPos{
		X: int32(math.Floor(pos.X)),
		Y: int32(math.Floor(pos.Y)),
		Z: int32(math.Floor(pos.Z)),
	}
}

// GroundBlockFromFeet returns the supporting block directly below the feet.
func GroundBlockFromFeet(feet BlockPos) BlockPos {
	return BlockPos{X: feet.X, Y: feet.Y - 1, Z: feet.Z}
}

// HeadBlockFromFeet returns the head block (upper hitbox half) above the feet.
func HeadBlockFromFeet(feet BlockPos) BlockPos {
	return BlockPos{X: feet.X, Y: feet.Y + 1, Z: feet.Z}
}

// StandingCenter returns the X/Z-centered world position for a feet block, with
// Y at the feet (block-top) level. This is the canonical movement target center.
func StandingCenter(feet BlockPos) Vec3 {
	return Vec3{
		X: float64(feet.X) + 0.5,
		Y: float64(feet.Y),
		Z: float64(feet.Z) + 0.5,
	}
}

// IsOnGround reports whether a player at the given world position is genuinely
// resting on solid ground. Unlike a naive "is the block below solid" check, it
// also requires the feet to be at (within OnGroundEpsilon of) the block top, so
// a position whose Y is still descending toward the support is correctly
// reported as airborne.
//
// When the relevant chunk is not loaded the result is optimistic (true) to
// avoid spurious falling behavior over unknown terrain, matching the rest of
// the movement code.
func (w *World) IsOnGround(pos Vec3) bool {
	if w == nil {
		return true
	}
	feet := FeetBlockFromPosition(pos)
	if !w.HasChunk(floorDiv(int(feet.X), ChunkWidth), floorDiv(int(feet.Z), ChunkDepth)) {
		return true
	}
	// Feet must be resting on the block top (fractional Y ~ 0); otherwise the
	// player is mid-air (falling or rising) and not on the ground.
	if pos.Y-math.Floor(pos.Y) > OnGroundEpsilon {
		return false
	}
	ground := GroundBlockFromFeet(feet)
	return !w.IsPassable(int(ground.X), int(ground.Y), int(ground.Z))
}
