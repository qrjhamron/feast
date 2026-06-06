package world

import (
	"math"
	"strings"
)

// Vec3 is a floating-point world-space coordinate.
type Vec3 struct {
	X float64
	Y float64
	Z float64
}

// BlockHit is the result of a loaded-world block search.
type BlockHit struct {
	X        int
	Y        int
	Z        int
	Block    BlockState
	Distance float64
}

// FindNearestBlock searches loaded chunks for the nearest block with the given
// registry name within radius blocks of origin.
func (w *World) FindNearestBlock(origin Vec3, name string, radius int) (BlockHit, bool) {
	if w == nil || radius < 0 {
		return BlockHit{}, false
	}
	target := normalizeBlockName(name)
	if target == "" {
		return BlockHit{}, false
	}

	originBlockY := int(math.Floor(origin.Y))
	minY := maxInt(MinY, originBlockY-radius)
	maxY := minInt(MaxY, originBlockY+radius)
	radiusSq := float64(radius * radius)

	var best BlockHit
	bestDistSq := math.Inf(1)
	found := false

	w.chunks.Range(func(_, value any) bool {
		chunk, ok := value.(*Chunk)
		if !ok || chunk == nil {
			return true
		}
		baseX := chunk.ChunkX * ChunkWidth
		baseZ := chunk.ChunkZ * ChunkDepth
		for localZ := 0; localZ < ChunkDepth; localZ++ {
			worldZ := baseZ + localZ
			dz := float64(worldZ) - origin.Z
			if dz*dz > radiusSq {
				continue
			}
			for localX := 0; localX < ChunkWidth; localX++ {
				worldX := baseX + localX
				dx := float64(worldX) - origin.X
				if dx*dx+dz*dz > radiusSq {
					continue
				}
				for y := minY; y <= maxY; y++ {
					dy := float64(y) - origin.Y
					distSq := dx*dx + dy*dy + dz*dz
					if distSq > radiusSq || distSq >= bestDistSq {
						continue
					}
					block, ok := chunk.BlockAt(localX, y, localZ)
					if !ok || normalizeBlockName(block.Name) != target {
						continue
					}
					found = true
					bestDistSq = distSq
					best = BlockHit{
						X:        worldX,
						Y:        y,
						Z:        worldZ,
						Block:    block,
						Distance: math.Sqrt(distSq),
					}
				}
			}
		}
		return true
	})

	return best, found
}

func normalizeBlockName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.TrimPrefix(name, "minecraft:")
	return name
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
