// Package smoke contains shared helpers for FeastGo smoke/integration tests.
// It provides common patterns (waiting for chunks, block search, etc.) used
// across cmd/smoke test modes.
package smoke

import (
	"bytes"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/qrjhamron/feast/pkg/feast"
	"github.com/qrjhamron/feast/pkg/world"
)

// WaitForChunks blocks until the client's world has at least minimum chunks
// loaded or timeout elapses.
func WaitForChunks(client *feast.Client, minimum int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if client.World().ChunkCount() >= minimum {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// FindBreakTarget returns a nearby grass_block, dirt, or stone hit.
func FindBreakTarget(client *feast.Client) (world.BlockHit, bool) {
	pos := client.Position()
	origin := world.Vec3{X: pos.X, Y: pos.Y, Z: pos.Z}
	for _, name := range []string{"grass_block", "dirt", "stone"} {
		if hit, ok := client.World().FindNearestBlock(origin, name, 6); ok {
			return hit, true
		}
	}
	return world.BlockHit{}, false
}

// FindPlaceTarget returns an air block above solid ground near the bot.
func FindPlaceTarget(client *feast.Client) (world.BlockHit, bool) {
	pos := client.Position()
	originX := int(math.Floor(pos.X))
	originZ := int(math.Floor(pos.Z))
	for radius := 1; radius <= 8; radius++ {
		for dx := -radius; dx <= radius; dx++ {
			for dz := -radius; dz <= radius; dz++ {
				if absInt(dx) != radius && absInt(dz) != radius {
					continue
				}
				wx := originX + dx
				wz := originZ + dz
				cx := float64(wx) + 0.5
				cz := float64(wz) + 0.5
				if math.Hypot(cx-pos.X, cz-pos.Z) < 2.0 {
					continue
				}
				surfaceY := client.World().GetSurfaceY(wx, wz)
				if surfaceY == world.UnknownSurfaceY {
					continue
				}
				targetY := surfaceY + 1
				target, err := client.World().GetBlock(wx, targetY, wz)
				if err != nil || target.Name != "air" {
					continue
				}
				support, err := client.World().GetBlock(wx, targetY-1, wz)
				if err != nil || !support.Solid {
					continue
				}
				return world.BlockHit{X: wx, Y: targetY, Z: wz, Block: target}, true
			}
		}
	}
	return world.BlockHit{}, false
}

// BreakDelay returns the approximate mining time for a block name.
func BreakDelay(name string) time.Duration {
	switch strings.TrimPrefix(strings.ToLower(name), "minecraft:") {
	case "grass_block", "dirt":
		return 1500 * time.Millisecond
	case "stone":
		return 8 * time.Second
	default:
		return 2 * time.Second
	}
}

// FaceName returns a human-readable face label for a byte face constant.
func FaceName(face byte) string {
	switch face {
	case 0:
		return "bottom"
	case 1:
		return "top"
	case 2:
		return "north"
	case 3:
		return "south"
	case 4:
		return "west"
	case 5:
		return "east"
	default:
		return fmt.Sprintf("unknown_%d", face)
	}
}

// RawFromPacket encodes a protocol packet into a RawPacket for synthetic dispatch.
func RawFromBytes(id int32, body *bytes.Buffer) []byte {
	return body.Bytes()
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
