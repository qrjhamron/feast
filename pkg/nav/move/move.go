package move

import "github.com/qrjhamron/feast/pkg/world"

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
