package hpa

import (
	"log"
	"sync"

	"github.com/qrjhamron/feast/pkg/state"
	"github.com/qrjhamron/feast/pkg/world"
)

type ClusterCoord struct {
	X, Z int
}

func blockToChunkCoord(block int) int {
	chunk := block / 16
	if block < 0 && block%16 != 0 {
		chunk--
	}
	return chunk
}

type EntranceNode struct {
	pos          [3]int
	clusterCoord ClusterCoord
	neighborPos  [3]int
	direction    int
}

type Cluster struct {
	coord     ClusterCoord
	minX      int
	minZ      int
	maxX      int
	maxZ      int
	sections  [16]bool
	entrances []EntranceNode
	dirty     bool
}

type TransitionScanner struct{}

type openSegment struct {
	start int
}

func (s *TransitionScanner) ScanBoundary(w *world.World, clusterA, clusterB *Cluster) []EntranceNode {
	var entrances []EntranceNode

	if clusterA == nil || clusterB == nil || w == nil {
		return entrances
	}

	var scanAxis int // 0 for X, 2 for Z
	var boundaryA, boundaryB int
	var minP, maxP int

	if clusterA.coord.X == clusterB.coord.X {
		scanAxis = 0 // scan along X
		minP = max(clusterA.minX, clusterB.minX)
		maxP = min(clusterA.maxX, clusterB.maxX)
		if clusterA.coord.Z < clusterB.coord.Z {
			boundaryA = clusterA.maxZ
			boundaryB = clusterB.minZ
		} else {
			boundaryA = clusterA.minZ
			boundaryB = clusterB.maxZ
		}
	} else if clusterA.coord.Z == clusterB.coord.Z {
		scanAxis = 2 // scan along Z
		minP = max(clusterA.minZ, clusterB.minZ)
		maxP = min(clusterA.maxZ, clusterB.maxZ)
		if clusterA.coord.X < clusterB.coord.X {
			boundaryA = clusterA.maxX
			boundaryB = clusterB.minX
		} else {
			boundaryA = clusterA.minX
			boundaryB = clusterB.maxX
		}
	} else {
		return entrances
	}

	active := make(map[int]openSegment)
	closeSegment := func(y, start, end int) {
		if end < start {
			return
		}
		mid := (start + end) / 2
		var posX, posZ, nX, nZ int
		if scanAxis == 0 {
			posX = mid
			posZ = boundaryA
			nX = mid
			nZ = boundaryB
		} else {
			posX = boundaryA
			posZ = mid
			nX = boundaryB
			nZ = mid
		}
		entrances = append(entrances, EntranceNode{
			pos:          [3]int{posX, y, posZ},
			clusterCoord: clusterA.coord,
			neighborPos:  [3]int{nX, y, nZ},
			direction:    0,
		})
	}

	for p := minP; p <= maxP; p++ {
		var xA, zA, xB, zB int
		if scanAxis == 0 {
			xA = p
			zA = boundaryA
			xB = p
			zB = boundaryB
		} else {
			xA = boundaryA
			zA = p
			xB = boundaryB
			zB = p
		}

		walkableAtP := make(map[int]struct{}, 2)
		for _, y := range uniqueCandidateYs(w.GetSurfaceY(xA, zA), w.GetSurfaceY(xB, zB)) {
			if y <= world.MinY || y >= world.MaxY {
				continue
			}
			if boundaryWalkableAt(w, xA, zA, xB, zB, y) {
				markBoundaryWalkable(active, walkableAtP, y, p)
			}
		}
		if len(walkableAtP) == 0 {
			for y := world.MinY + 1; y < world.MaxY; y++ {
				if boundaryWalkableAt(w, xA, zA, xB, zB, y) {
					markBoundaryWalkable(active, walkableAtP, y, p)
					break
				}
			}
		}
		for y, seg := range active {
			if _, ok := walkableAtP[y]; !ok {
				closeSegment(y, seg.start, p-1)
				delete(active, y)
			}
		}
	}
	for y, seg := range active {
		closeSegment(y, seg.start, maxP)
	}
	return entrances
}

func boundaryWalkableAt(w *world.World, xA, zA, xB, zB, y int) bool {
	return w.IsPassable(xA, y, zA) && w.IsPassable(xA, y+1, zA) && !w.IsPassable(xA, y-1, zA) &&
		w.IsPassable(xB, y, zB) && w.IsPassable(xB, y+1, zB) && !w.IsPassable(xB, y-1, zB)
}

func markBoundaryWalkable(active map[int]openSegment, walkableAtP map[int]struct{}, y, p int) {
	walkableAtP[y] = struct{}{}
	if _, ok := active[y]; !ok {
		active[y] = openSegment{start: p}
	}
}

func uniqueCandidateYs(surfaceA, surfaceB int) []int {
	out := make([]int, 0, 2)
	if surfaceA != world.UnknownSurfaceY {
		out = append(out, surfaceA+1)
	}
	if surfaceB != world.UnknownSurfaceY && surfaceB != surfaceA {
		out = append(out, surfaceB+1)
	}
	return out
}

type ClusterManager struct {
	clusters sync.Map
	bus      *state.EventBus
}

func NewClusterManager(bus *state.EventBus) *ClusterManager {
	m := &ClusterManager{bus: bus}
	if bus != nil {
		bus.On("block_update", func(e state.Event) {
			// handled generically if needed.
		})
	}
	return m
}

// Count returns the total number of tracked clusters.
func (m *ClusterManager) Count() int {
	count := 0
	m.clusters.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// BuiltCount returns the number of clusters with non-dirty state.
func (m *ClusterManager) BuiltCount() int {
	count := 0
	m.clusters.Range(func(_, value any) bool {
		c := value.(*Cluster)
		if !c.dirty {
			count++
		}
		return true
	})
	return count
}

// EntranceCount returns the total number of entrance nodes across clusters.
func (m *ClusterManager) EntranceCount() int {
	count := 0
	m.clusters.Range(func(_, value any) bool {
		c := value.(*Cluster)
		count += len(c.entrances)
		return true
	})
	return count
}

// ForEach iterates all clusters.
func (m *ClusterManager) ForEach(fn func(*Cluster)) {
	m.clusters.Range(func(_, value any) bool {
		fn(value.(*Cluster))
		return true
	})
}

func (m *ClusterManager) GetOrCreate(chunkX, chunkZ int) *Cluster {
	coord := ClusterCoord{X: chunkX, Z: chunkZ}
	if val, ok := m.clusters.Load(coord); ok {
		return val.(*Cluster)
	}
	c := &Cluster{
		coord: coord,
		minX:  chunkX * 16,
		maxX:  chunkX*16 + 15,
		minZ:  chunkZ * 16,
		maxZ:  chunkZ*16 + 15,
		dirty: true,
	}
	m.clusters.Store(coord, c)
	return c
}

// MarkDirty marks the target cluster as dirty, creating it if needed.
func (m *ClusterManager) MarkDirty(chunkX, chunkZ int) *Cluster {
	c := m.GetOrCreate(chunkX, chunkZ)
	c.dirty = true
	return c
}

func (m *ClusterManager) Invalidate(blockX, blockZ int) {
	chunkX := blockToChunkCoord(blockX)
	chunkZ := blockToChunkCoord(blockZ)

	c := m.GetOrCreate(chunkX, chunkZ)
	c.dirty = true

	localX := blockX - (chunkX * 16)
	localZ := blockZ - (chunkZ * 16)
	if localX == 0 {
		m.GetOrCreate(chunkX-1, chunkZ).dirty = true
	} else if localX == 15 {
		m.GetOrCreate(chunkX+1, chunkZ).dirty = true
	}
	if localZ == 0 {
		m.GetOrCreate(chunkX, chunkZ-1).dirty = true
	} else if localZ == 15 {
		m.GetOrCreate(chunkX, chunkZ+1).dirty = true
	}
	log.Printf("[hpa] cluster invalidate block=(%d,%d) clusters=%d built=%d", blockX, blockZ, m.Count(), m.BuiltCount())
}
