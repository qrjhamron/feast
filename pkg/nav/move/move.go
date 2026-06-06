package move

import "github.com/qrjhamron/feast/pkg/world"

// Movement is a single navigation step between two block positions.
//
// Node/position semantics (shared by the planner, executor and every
// primitive in this package):
//
//   - A position [3]int{x,y,z} is the player's FEET block: the block the
//     player's lower hitbox occupies. The player stands on top of it.
//   - The GROUND/support block is y-1 and must be solid (non-passable) for a
//     grounded move to be valid.
//   - The HEAD block is y+1 and must be passable (the player is ~2 blocks tall).
//   - The executor converts a feet block to a world-space target of
//     (x+0.5, y, z+0.5) — the horizontal center of the block, at floor height.
//
// Cost returns the traversal cost from from, or +Inf if the move is invalid
// (blocked, unsupported, through lava, into an unloaded chunk, etc.).
// Destination returns the resulting feet block position.
type Movement interface {
	Cost(w *world.World, from [3]int) float64
	Destination(from [3]int) [3]int
}

// RotatingMovement optionally provides a preferred yaw (in Minecraft degrees)
// to face while executing this movement step.
type RotatingMovement interface {
	Movement
	DesiredYaw(from [3]int) float32
}
