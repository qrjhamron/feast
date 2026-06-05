package move

import "math"

func yawForDelta(dx, dz int) float32 {
	if dx == 0 && dz == 0 {
		return 0
	}
	return float32(-math.Atan2(float64(dx), float64(dz)) * 180 / math.Pi)
}
