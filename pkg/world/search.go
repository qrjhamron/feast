package world

import (
	"math"
	"sort"
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
//
// Only chunk columns overlapping the search radius are visited (looked up
// directly by coordinate), so cost scales with the radius rather than with the
// total number of loaded chunks. Chunks are visited in a deterministic
// coordinate order, so ties at equal distance resolve deterministically.
func (w *World) FindNearestBlock(origin Vec3, name string, radius int) (BlockHit, bool) {
	if w == nil || radius < 0 {
		return BlockHit{}, false
	}
	target := normalizeBlockName(name)
	if target == "" {
		return BlockHit{}, false
	}

	originX := int(math.Floor(origin.X))
	originZ := int(math.Floor(origin.Z))
	originBlockY := int(math.Floor(origin.Y))
	minY := maxInt(MinY, originBlockY-radius)
	maxY := minInt(MaxY, originBlockY+radius)
	radiusSq := float64(radius * radius)

	minCX := floorDiv(originX-radius, ChunkWidth)
	maxCX := floorDiv(originX+radius, ChunkWidth)
	minCZ := floorDiv(originZ-radius, ChunkDepth)
	maxCZ := floorDiv(originZ+radius, ChunkDepth)

	var best BlockHit
	bestDistSq := math.Inf(1)
	found := false

	for cx := minCX; cx <= maxCX; cx++ {
		for cz := minCZ; cz <= maxCZ; cz++ {
			v, ok := w.chunks.Load(chunkKey{x: cx, z: cz})
			if !ok {
				continue
			}
			chunk, _ := v.(*Chunk)
			if chunk == nil {
				continue
			}
			baseX := cx * ChunkWidth
			baseZ := cz * ChunkDepth
			for localZ := 0; localZ < ChunkDepth; localZ++ {
				worldZ := baseZ + localZ
				dz := float64(worldZ) - origin.Z
				dz2 := dz * dz
				if dz2 > radiusSq {
					continue
				}
				for localX := 0; localX < ChunkWidth; localX++ {
					worldX := baseX + localX
					dx := float64(worldX) - origin.X
					dxz2 := dx*dx + dz2
					if dxz2 > radiusSq {
						continue
					}
					for y := minY; y <= maxY; y++ {
						dy := float64(y) - origin.Y
						distSq := dxz2 + dy*dy
						if distSq > radiusSq || distSq >= bestDistSq {
							continue
						}
						name, nameOK := chunk.blockNameAt(localX, y, localZ)
						// Registry-derived block names are already normalized, so a
						// direct comparison is correct and avoids per-block string work.
						if !nameOK || name != target {
							continue
						}
						// Only materialize the full block state on a match.
						block, _ := chunk.BlockAt(localX, y, localZ)
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
		}
	}

	return best, found
}

// FindBlocks searches loaded chunks for up to count nearest blocks matching name
// within radius. The returned slice is sorted by distance (closest first).
func (w *World) FindBlocks(origin Vec3, name string, count, radius int) []BlockHit {
	if w == nil || radius < 0 || count <= 0 {
		return nil
	}
	target := normalizeBlockName(name)
	if target == "" {
		return nil
	}

	originX := int(math.Floor(origin.X))
	originZ := int(math.Floor(origin.Z))
	originBlockY := int(math.Floor(origin.Y))
	minY := maxInt(MinY, originBlockY-radius)
	maxY := minInt(MaxY, originBlockY+radius)
	radiusSq := float64(radius * radius)

	minCX := floorDiv(originX-radius, ChunkWidth)
	maxCX := floorDiv(originX+radius, ChunkWidth)
	minCZ := floorDiv(originZ-radius, ChunkDepth)
	maxCZ := floorDiv(originZ+radius, ChunkDepth)

	var hits []BlockHit

	for cx := minCX; cx <= maxCX; cx++ {
		for cz := minCZ; cz <= maxCZ; cz++ {
			v, ok := w.chunks.Load(chunkKey{x: cx, z: cz})
			if !ok {
				continue
			}
			chunk, _ := v.(*Chunk)
			if chunk == nil {
				continue
			}
			baseX := cx * ChunkWidth
			baseZ := cz * ChunkDepth
			for localZ := 0; localZ < ChunkDepth; localZ++ {
				worldZ := baseZ + localZ
				dz := float64(worldZ) - origin.Z
				dz2 := dz * dz
				if dz2 > radiusSq {
					continue
				}
				for localX := 0; localX < ChunkWidth; localX++ {
					worldX := baseX + localX
					dx := float64(worldX) - origin.X
					dxz2 := dx*dx + dz2
					if dxz2 > radiusSq {
						continue
					}
					for y := minY; y <= maxY; y++ {
						dy := float64(y) - origin.Y
						distSq := dxz2 + dy*dy
						if distSq > radiusSq {
							continue
						}
						name, nameOK := chunk.blockNameAt(localX, y, localZ)
						if !nameOK || name != target {
							continue
						}
						block, _ := chunk.BlockAt(localX, y, localZ)
						hits = append(hits, BlockHit{
							X:        worldX,
							Y:        y,
							Z:        worldZ,
							Block:    block,
							Distance: math.Sqrt(distSq),
						})
					}
				}
			}
		}
	}

	sort.Slice(hits, func(i, j int) bool {
		return hits[i].Distance < hits[j].Distance
	})
	if len(hits) > count {
		hits = hits[:count]
	}
	return hits
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
