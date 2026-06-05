package move

import (
	"strings"

	"github.com/qrjhamron/feast/pkg/world"
)

// terrainMultiplier returns a multiplicative movement-cost factor based on the
// block the player is standing on (floor) and/or occupying (feet).
//
// This is a deliberately small, name-based model intended for offline-mode bots.
// If a block name is unknown, it defaults to 1.0.
func terrainMultiplier(w *world.World, feetX, feetY, feetZ int) float64 {
	floor := 1.0
	if b, err := w.GetBlock(feetX, feetY-1, feetZ); err == nil {
		if isIceName(b.Name) {
			floor *= 0.5
		}
		if isSoulSandName(b.Name) {
			floor *= 2.5
		}
	}

	feet := 1.0
	if b, err := w.GetBlock(feetX, feetY, feetZ); err == nil {
		if isWaterName(b.Name) {
			feet *= 4.0
		}
	}

	return floor * feet
}

func isWaterName(name string) bool {
	n := strings.ToLower(name)
	return n == "water" || strings.HasSuffix(n, ":water")
}

func isIceName(name string) bool {
	n := strings.ToLower(name)
	// Includes ice variants like packed_ice/blue_ice/frosted_ice.
	return n == "ice" || strings.HasSuffix(n, ":ice") || strings.HasSuffix(n, "_ice") || strings.HasSuffix(n, ":packed_ice") || strings.HasSuffix(n, ":blue_ice") || strings.HasSuffix(n, ":frosted_ice")
}

func isSoulSandName(name string) bool {
	n := strings.ToLower(name)
	return n == "soul_sand" || strings.HasSuffix(n, ":soul_sand")
}
