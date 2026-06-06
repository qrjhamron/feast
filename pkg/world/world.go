package world

import (
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/qrjhamron/feast/pkg/protocol"
)

var (
	ErrBlockNotFound  = errors.New("block not found")
	ErrChunkNotLoaded = errors.New("chunk not loaded")
)

type BlockPos = protocol.BlockPos

type chunkKey struct {
	x int
	z int
}

type World struct {
	chunks      sync.Map // map[chunkKey]*Chunk
	botMu       sync.RWMutex
	botX        float64
	botZ        float64
	botEntityID int32
	entities    *EntityStore
	entitiesMu  sync.RWMutex
}

const MaxLoadedChunks = 1024

// UnknownSurfaceY is returned when a surface query targets an unloaded chunk.
const UnknownSurfaceY = -999

func NewWorld() *World {
	return &World{}
}

func (w *World) AddChunk(chunk *Chunk) {
	if w == nil || chunk == nil {
		return
	}
	w.chunks.Store(chunkKey{x: chunk.ChunkX, z: chunk.ChunkZ}, chunk)
	w.evictIfNeeded()
}

func (w *World) RemoveChunk(chunkX, chunkZ int) {
	if w == nil {
		return
	}
	w.chunks.Delete(chunkKey{x: chunkX, z: chunkZ})
}

// Reset clears all loaded chunks.
func (w *World) Reset() {
	if w == nil {
		return
	}
	w.chunks.Range(func(key, _ any) bool {
		w.chunks.Delete(key)
		return true
	})
}

// ChunkCount returns the number of loaded chunks currently in memory.
func (w *World) ChunkCount() int {
	if w == nil {
		return 0
	}
	count := 0
	w.chunks.Range(func(_, _ any) bool {
		count++
		return true
	})
	return count
}

// HasChunk reports whether the specified chunk coordinate is currently loaded.
func (w *World) HasChunk(chunkX, chunkZ int) bool {
	if w == nil {
		return false
	}
	_, ok := w.chunks.Load(chunkKey{x: chunkX, z: chunkZ})
	return ok
}

func (w *World) IsChunkLoaded(chunkX, chunkZ int) bool {
	return w.HasChunk(chunkX, chunkZ)
}

func (w *World) IsBlockLoaded(pos BlockPos) bool {
	if w == nil || int(pos.Y) < MinY || int(pos.Y) > MaxY {
		return false
	}
	return w.IsChunkLoaded(floorDiv(int(pos.X), ChunkWidth), floorDiv(int(pos.Z), ChunkDepth))
}

func (w *World) RequireBlockLoaded(pos BlockPos) error {
	if w == nil {
		return ErrChunkNotLoaded
	}
	if int(pos.Y) < MinY || int(pos.Y) > MaxY {
		return fmt.Errorf("%w: y out of range", ErrBlockNotFound)
	}
	chunkX := floorDiv(int(pos.X), ChunkWidth)
	chunkZ := floorDiv(int(pos.Z), ChunkDepth)
	if !w.IsChunkLoaded(chunkX, chunkZ) {
		return fmt.Errorf("%w: missing chunk (%d,%d)", ErrChunkNotLoaded, chunkX, chunkZ)
	}
	return nil
}

// ChunkCoords returns a snapshot of loaded chunk coordinates.
func (w *World) ChunkCoords() [][2]int {
	if w == nil {
		return nil
	}
	out := make([][2]int, 0, 64)
	w.chunks.Range(func(key, _ any) bool {
		ck, ok := key.(chunkKey)
		if ok {
			out = append(out, [2]int{ck.x, ck.z})
		}
		return true
	})
	return out
}

// SetBotPosition sets the latest bot position used as eviction center.
func (w *World) SetBotPosition(x, z float64) {
	if w == nil {
		return
	}
	w.botMu.Lock()
	w.botX = x
	w.botZ = z
	w.botMu.Unlock()
}

// SetBlock updates the cached world block state at absolute world coordinates.
// It returns false if the chunk is missing or coordinates are out of range.
func (w *World) SetBlock(x, y, z int, stateID uint16) bool {
	if w == nil {
		return false
	}
	if y < MinY || y > MaxY {
		return false
	}

	chunkX := floorDiv(x, ChunkWidth)
	chunkZ := floorDiv(z, ChunkDepth)
	localX := mod(x, ChunkWidth)
	localZ := mod(z, ChunkDepth)

	var chunk *Chunk
	if v, ok := w.chunks.Load(chunkKey{x: chunkX, z: chunkZ}); ok {
		chunk, _ = v.(*Chunk)
	}
	if chunk == nil {
		return false
	}

	// Preserve name/solid information when known.
	bs := blockStateFromID(int32(stateID))
	if bs.Name == "" {
		bs.ID = int32(stateID)
	}
	return chunk.SetBlock(localX, y, localZ, bs)
}

func (w *World) GetBlock(x, y, z int) (BlockState, error) {
	if w == nil {
		return BlockState{}, ErrBlockNotFound
	}
	if y < MinY || y > MaxY {
		return BlockState{}, fmt.Errorf("%w: y out of range", ErrBlockNotFound)
	}

	chunkX := floorDiv(x, ChunkWidth)
	chunkZ := floorDiv(z, ChunkDepth)
	localX := mod(x, ChunkWidth)
	localZ := mod(z, ChunkDepth)

	var chunk *Chunk
	if v, ok := w.chunks.Load(chunkKey{x: chunkX, z: chunkZ}); ok {
		chunk, _ = v.(*Chunk)
	}
	if chunk == nil {
		return BlockState{}, fmt.Errorf("%w: %w (%d,%d)", ErrBlockNotFound, ErrChunkNotLoaded, chunkX, chunkZ)
	}

	block, ok := chunk.BlockAt(localX, y, localZ)
	if !ok {
		return BlockState{}, fmt.Errorf("%w: invalid local coords (%d,%d,%d)", ErrBlockNotFound, localX, y, localZ)
	}
	return block, nil
}

// GetBlockEntities returns a copy of typed block entities in a loaded chunk.
func (w *World) GetBlockEntities(chunkX, chunkZ int) []BlockEntity {
	if w == nil {
		return nil
	}
	v, ok := w.chunks.Load(chunkKey{x: chunkX, z: chunkZ})
	if !ok {
		return nil
	}
	chunk, _ := v.(*Chunk)
	if chunk == nil {
		return nil
	}
	chunk.mu.RLock()
	defer chunk.mu.RUnlock()
	if len(chunk.BlockEntities) == 0 {
		return nil
	}
	out := make([]BlockEntity, len(chunk.BlockEntities))
	copy(out, chunk.BlockEntities)
	return out
}

// GetSurfaceY returns the cached motion-blocking surface Y for world X,Z.
// If the chunk is not loaded, it returns UnknownSurfaceY.
func (w *World) GetSurfaceY(x, z int) int {
	if w == nil {
		return UnknownSurfaceY
	}
	chunkX := floorDiv(x, ChunkWidth)
	chunkZ := floorDiv(z, ChunkDepth)
	localX := mod(x, ChunkWidth)
	localZ := mod(z, ChunkDepth)
	v, ok := w.chunks.Load(chunkKey{x: chunkX, z: chunkZ})
	if !ok {
		return UnknownSurfaceY
	}
	chunk, _ := v.(*Chunk)
	if chunk == nil {
		return UnknownSurfaceY
	}
	index := localZ*ChunkWidth + localX
	chunk.mu.RLock()
	y := int(chunk.SurfaceY[index])
	chunk.mu.RUnlock()
	return y
}

func (w *World) IsPassable(coord any, rest ...int) bool {
	x, y, z, ok := parseBlockArgs(coord, rest...)
	if !ok {
		return false
	}
	block, err := w.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	return isPassableSafetyName(block.Name)
}

func (w *World) IsReplaceable(pos BlockPos) bool {
	block, err := w.GetBlock(int(pos.X), int(pos.Y), int(pos.Z))
	if err != nil {
		return false
	}
	return isReplaceableSafetyName(block.Name)
}

func (w *World) IsSolid(pos BlockPos) bool {
	block, err := w.GetBlock(int(pos.X), int(pos.Y), int(pos.Z))
	if err != nil {
		return false
	}
	return isSolidSafetyBlock(block)
}

func (w *World) HasSolidGround(pos BlockPos) bool {
	below := BlockPos{X: pos.X, Y: pos.Y - 1, Z: pos.Z}
	return w.IsBlockLoaded(below) && w.IsSolid(below)
}

func parseBlockArgs(coord any, rest ...int) (int, int, int, bool) {
	switch v := coord.(type) {
	case BlockPos:
		if len(rest) != 0 {
			return 0, 0, 0, false
		}
		return int(v.X), int(v.Y), int(v.Z), true
	case int:
		if len(rest) != 2 {
			return 0, 0, 0, false
		}
		return v, rest[0], rest[1], true
	default:
		return 0, 0, 0, false
	}
}

func isPassableSafetyName(name string) bool {
	name = normalizeBlockName(name)
	// Water is traversable by swim movement but never solid ground; lava is unsafe
	// for local survival movement and is treated as blocking.
	if name == "lava" {
		return false
	}
	if name == "water" {
		return true
	}
	return isPassableBlockName(name)
}

func isReplaceableSafetyName(name string) bool {
	switch normalizeBlockName(name) {
	case "air", "cave_air", "void_air", "short_grass", "tall_grass", "fern", "large_fern", "snow":
		return true
	default:
		return false
	}
}

func isSolidSafetyBlock(block BlockState) bool {
	name := normalizeBlockName(block.Name)
	switch name {
	case "", "air", "cave_air", "void_air", "short_grass", "tall_grass", "water", "lava":
		return false
	case "stone", "dirt", "grass_block", "chest":
		return true
	default:
		if block.Solid {
			return true
		}
		return !isPassableSafetyName(name)
	}
}

func floorDiv(a, b int) int {
	q := a / b
	r := a % b
	if r != 0 && ((r < 0) != (b < 0)) {
		q--
	}
	return q
}

func mod(a, b int) int {
	m := a % b
	if m < 0 {
		m += b
	}
	return m
}

func (w *World) evictIfNeeded() {
	if w == nil {
		return
	}
	count := 0
	w.chunks.Range(func(_, _ any) bool {
		count++
		return true
	})
	if count <= MaxLoadedChunks {
		return
	}

	w.botMu.RLock()
	bx := int(math.Floor(w.botX))
	bz := int(math.Floor(w.botZ))
	w.botMu.RUnlock()
	bcx := floorDiv(bx, ChunkWidth)
	bcz := floorDiv(bz, ChunkDepth)

	for count > MaxLoadedChunks {
		var farKey chunkKey
		found := false
		maxDist := -1
		w.chunks.Range(func(key, _ any) bool {
			ck, ok := key.(chunkKey)
			if !ok {
				return true
			}
			dx := ck.x - bcx
			dz := ck.z - bcz
			d2 := dx*dx + dz*dz
			if !found || d2 > maxDist {
				found = true
				maxDist = d2
				farKey = ck
			}
			return true
		})
		if !found {
			return
		}
		w.chunks.Delete(farKey)
		count--
	}
}

func (w *World) SetEntityStore(store *EntityStore) {
	w.entitiesMu.Lock()
	defer w.entitiesMu.Unlock()
	w.entities = store
}

func (w *World) SetBotEntityID(id int32) {
	w.botMu.Lock()
	w.botEntityID = id
	w.botMu.Unlock()
}

func (w *World) EntityCollisions(box AABB) []Entity {
	w.entitiesMu.RLock()
	store := w.entities
	w.entitiesMu.RUnlock()

	if store == nil {
		return nil
	}

	w.botMu.RLock()
	botID := w.botEntityID
	w.botMu.RUnlock()

	var collided []Entity
	store.mu.RLock()
	for _, e := range store.m {
		if e.ID == botID {
			continue
		}
		if e.Hitbox().Intersects(box) {
			collided = append(collided, *e)
		}
	}
	store.mu.RUnlock()
	return collided
}

func (w *World) IsEntityBlocking(box AABB) bool {
	collisions := w.EntityCollisions(box)
	return len(collisions) > 0
}
