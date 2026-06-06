package world

import (
	"errors"
	"math"
	"sync"
	"testing"
)

func TestWorldAddGetRemoveChunk(t *testing.T) {
	w := NewWorld()
	ch := NewChunk(0, 0)
	stone := BlockState{Name: "stone", Solid: true}
	ch.SetBlock(1, 64, 1, stone)
	w.AddChunk(ch)

	got, err := w.GetBlock(1, 64, 1)
	if err != nil {
		t.Fatalf("GetBlock returned error: %v", err)
	}
	if got != stone {
		t.Fatalf("unexpected block state: got %+v want %+v", got, stone)
	}

	w.RemoveChunk(0, 0)
	_, err = w.GetBlock(1, 64, 1)
	if !errors.Is(err, ErrBlockNotFound) {
		t.Fatalf("expected ErrBlockNotFound after remove, got %v", err)
	}
}

func TestWorldGetBlockNegativeCoordinates(t *testing.T) {
	w := NewWorld()
	ch := NewChunk(-1, -1)
	dirt := BlockState{Name: "dirt", Solid: true}
	ch.SetBlock(15, 10, 15, dirt)
	w.AddChunk(ch)

	got, err := w.GetBlock(-1, 10, -1)
	if err != nil {
		t.Fatalf("GetBlock returned error: %v", err)
	}
	if got != dirt {
		t.Fatalf("unexpected block state: got %+v want %+v", got, dirt)
	}
}

func TestWorldLoadedChunkAPIs(t *testing.T) {
	w := NewWorld()
	w.AddChunk(NewChunk(1, 2))
	w.AddChunk(NewChunk(-1, -2))

	if !w.IsChunkLoaded(1, 2) {
		t.Fatal("positive chunk should be loaded")
	}
	if !w.IsBlockLoaded(BlockPos{X: 16, Y: 64, Z: 32}) {
		t.Fatal("positive block should be loaded")
	}
	if !w.IsChunkLoaded(-1, -2) {
		t.Fatal("negative chunk should be loaded")
	}
	if !w.IsBlockLoaded(BlockPos{X: -1, Y: 64, Z: -17}) {
		t.Fatal("negative block should use floor chunk math")
	}
	if w.IsBlockLoaded(BlockPos{X: 0, Y: 64, Z: 0}) {
		t.Fatal("unloaded block should report false")
	}
	if err := w.RequireBlockLoaded(BlockPos{X: 0, Y: 64, Z: 0}); !errors.Is(err, ErrChunkNotLoaded) {
		t.Fatalf("RequireBlockLoaded error=%v want ErrChunkNotLoaded", err)
	}
}

func TestWorldUnloadedBlockAccessAndUpdates(t *testing.T) {
	w := NewWorld()
	if ok := w.SetBlock(5, 64, 5, 1); ok {
		t.Fatal("SetBlock in unloaded chunk should reject clearly")
	}
	if got, err := w.GetBlock(5, 64, 5); err == nil || got.Name == "air" {
		t.Fatalf("GetBlock unloaded got=%+v err=%v; must not invent air", got, err)
	}

	ch := NewChunk(0, 0)
	ch.SetBlock(5, 64, 5, BlockState{Name: "stone", Solid: true, ID: 1})
	w.AddChunk(ch)
	w.RemoveChunk(0, 0)
	if w.IsBlockLoaded(BlockPos{X: 5, Y: 64, Z: 5}) {
		t.Fatal("block should not be loaded after chunk unload")
	}
	if got, err := w.GetBlock(5, 64, 5); err == nil || got.Name == "stone" {
		t.Fatalf("GetBlock after unload got=%+v err=%v; stale data should be inaccessible", got, err)
	}
}

func TestWorldGetBlockMissingChunkAndYBounds(t *testing.T) {
	w := NewWorld()
	_, err := w.GetBlock(0, MinY-1, 0)
	if !errors.Is(err, ErrBlockNotFound) {
		t.Fatalf("expected y-bounds error to wrap ErrBlockNotFound, got %v", err)
	}

	_, err = w.GetBlock(0, MaxY+1, 0)
	if !errors.Is(err, ErrBlockNotFound) {
		t.Fatalf("expected y-bounds error to wrap ErrBlockNotFound, got %v", err)
	}

	_, err = w.GetBlock(0, 10, 0)
	if !errors.Is(err, ErrBlockNotFound) {
		t.Fatalf("expected missing chunk error, got %v", err)
	}
}

func TestWorldGetBlockSupportedYRange(t *testing.T) {
	w := NewWorld()
	ch := NewChunk(0, 0)
	stone := BlockState{Name: "stone", Solid: true, ID: 1}
	for _, y := range []int{MinY, -1, 0, 255, MaxY} {
		if ok := ch.SetBlock(0, y, 0, stone); !ok {
			t.Fatalf("SetBlock failed at y=%d", y)
		}
	}
	w.AddChunk(ch)

	for _, y := range []int{MinY, -1, 0, 255, MaxY} {
		got, err := w.GetBlock(0, y, 0)
		if err != nil {
			t.Fatalf("GetBlock(%d) error: %v", y, err)
		}
		if got.ID != stone.ID {
			t.Fatalf("GetBlock(%d) id=%d want=%d", y, got.ID, stone.ID)
		}
	}
}

func TestWorldIsPassable(t *testing.T) {
	w := NewWorld()
	ch := NewChunk(0, 0)
	airID, ok := FindAnyStateIDByName("air")
	if !ok {
		t.Fatal("missing air state")
	}
	shortGrassID, ok := FindAnyStateIDByName("short_grass")
	if !ok {
		t.Fatal("missing short_grass state")
	}
	torchID, ok := FindAnyStateIDByName("torch")
	if !ok {
		t.Fatal("missing torch state")
	}
	stoneID, ok := FindAnyStateIDByName("stone")
	if !ok {
		t.Fatal("missing stone state")
	}
	dirtID, ok := FindAnyStateIDByName("dirt")
	if !ok {
		t.Fatal("missing dirt state")
	}
	woodID, ok := FindAnyStateIDByName("oak_planks")
	if !ok {
		t.Fatal("missing oak_planks state")
	}

	ch.SetBlock(0, 50, 0, blockStateFromID(airID))
	ch.SetBlock(1, 50, 0, blockStateFromID(shortGrassID))
	ch.SetBlock(2, 50, 0, blockStateFromID(torchID))
	ch.SetBlock(3, 50, 0, blockStateFromID(stoneID))
	ch.SetBlock(4, 50, 0, blockStateFromID(dirtID))
	ch.SetBlock(5, 50, 0, blockStateFromID(woodID))
	w.AddChunk(ch)

	if !w.IsPassable(0, 50, 0) {
		t.Fatal("air should be passable")
	}
	if !w.IsPassable(1, 50, 0) {
		t.Fatal("grass should be passable")
	}
	if !w.IsPassable(2, 50, 0) {
		t.Fatal("torch should be passable")
	}
	if w.IsPassable(3, 50, 0) {
		t.Fatal("stone should be solid")
	}
	if w.IsPassable(4, 50, 0) {
		t.Fatal("dirt should be solid")
	}
	if w.IsPassable(5, 50, 0) {
		t.Fatal("wood should be solid")
	}
	if w.IsPassable(ChunkWidth*10, 50, ChunkDepth*10) {
		t.Fatal("missing chunk/block should not be passable")
	}
}

func TestWorldBlockSafetyHelpers(t *testing.T) {
	w := NewWorld()
	ch := NewChunk(0, 0)
	ch.SetBlock(0, 64, 0, BlockState{Name: "air", ID: 0})
	ch.SetBlock(1, 64, 0, BlockState{Name: "short_grass", ID: 42})
	ch.SetBlock(2, 64, 0, BlockState{Name: "water", ID: 100})
	ch.SetBlock(3, 64, 0, BlockState{Name: "lava", ID: 101})
	ch.SetBlock(4, 64, 0, BlockState{Name: "stone", Solid: true, ID: 1})
	ch.SetBlock(5, 64, 0, BlockState{Name: "dirt", Solid: true, ID: 10})
	ch.SetBlock(6, 64, 0, BlockState{Name: "grass_block", Solid: true, ID: 9})
	ch.SetBlock(7, 64, 0, BlockState{Name: "chest", Solid: true, ID: 200})
	ch.SetBlock(8, 63, 0, BlockState{Name: "stone", Solid: true, ID: 1})
	ch.SetBlock(8, 64, 0, BlockState{Name: "air", ID: 0})
	w.AddChunk(ch)

	for _, pos := range []BlockPos{{X: 0, Y: 64, Z: 0}, {X: 1, Y: 64, Z: 0}} {
		if !w.IsReplaceable(pos) || !w.IsPassable(pos) || w.IsSolid(pos) {
			t.Fatalf("replaceable/passable mismatch at %+v", pos)
		}
	}
	if !w.IsPassable(BlockPos{X: 2, Y: 64, Z: 0}) {
		t.Fatal("water should be passable for local movement; callers must still treat it as non-ground")
	}
	if w.IsPassable(BlockPos{X: 3, Y: 64, Z: 0}) || w.IsReplaceable(BlockPos{X: 3, Y: 64, Z: 0}) {
		t.Fatal("lava should not be passable or replaceable")
	}
	for _, pos := range []BlockPos{{X: 4, Y: 64, Z: 0}, {X: 5, Y: 64, Z: 0}, {X: 6, Y: 64, Z: 0}, {X: 7, Y: 64, Z: 0}} {
		if !w.IsSolid(pos) || w.IsPassable(pos) || w.IsReplaceable(pos) {
			t.Fatalf("solid mismatch at %+v", pos)
		}
	}
	if !w.HasSolidGround(BlockPos{X: 8, Y: 64, Z: 0}) {
		t.Fatal("air block above stone should have solid ground")
	}
	if w.HasSolidGround(BlockPos{X: 0, Y: 64, Z: 0}) {
		t.Fatal("air without loaded solid block below should not have solid ground")
	}
}

func TestWorldConcurrentAddAndRead(t *testing.T) {
	w := NewWorld()
	wg := sync.WaitGroup{}

	for i := 0; i < 20; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch := NewChunk(i, 0)
			ch.SetBlock(0, 64, 0, BlockState{Name: "stone", Solid: true})
			w.AddChunk(ch)
		}()
	}

	for i := 0; i < 20; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = w.GetBlock(i*ChunkWidth, 64, 0)
		}()
	}

	wg.Wait()
}

func TestWorldResetClearsChunks(t *testing.T) {
	w := NewWorld()
	ch := NewChunk(0, 0)
	ch.SetBlock(0, 64, 0, BlockState{ID: 1, Name: "stone", Solid: true})
	w.AddChunk(ch)

	w.Reset()
	if _, err := w.GetBlock(0, 64, 0); !errors.Is(err, ErrBlockNotFound) {
		t.Fatalf("expected missing block after reset, got %v", err)
	}
}

func TestWorldChunkCacheEvictsWhenOverLimit(t *testing.T) {
	w := NewWorld()
	w.SetBotPosition(0, 0)

	for i := 0; i < MaxLoadedChunks+50; i++ {
		ch := NewChunk(i, 0)
		ch.SetBlock(0, 64, 0, BlockState{ID: 1, Name: "stone", Solid: true})
		w.AddChunk(ch)
	}

	count := 0
	w.chunks.Range(func(_, _ any) bool {
		count++
		return true
	})
	if count > MaxLoadedChunks {
		t.Fatalf("chunk cache exceeded max: got %d want <= %d", count, MaxLoadedChunks)
	}
}

func TestWorldGetBlockEntities(t *testing.T) {
	w := NewWorld()
	ch := NewChunk(2, -3)
	ch.BlockEntities = []BlockEntity{
		{X: 33, Y: 64, Z: -47, TypeID: 1},
	}
	w.AddChunk(ch)

	out := w.GetBlockEntities(2, -3)
	if len(out) != 1 {
		t.Fatalf("entities len=%d want 1", len(out))
	}
	if out[0].X != 33 || out[0].Y != 64 || out[0].Z != -47 || out[0].TypeID != 1 {
		t.Fatalf("entity mismatch: %+v", out[0])
	}

	// Ensure callers get a copy, not internal storage.
	out[0].Y = -999
	out2 := w.GetBlockEntities(2, -3)
	if out2[0].Y != 64 {
		t.Fatalf("entity storage was mutated: %+v", out2[0])
	}
}

func TestWorldGetSurfaceY(t *testing.T) {
	w := NewWorld()
	ch := NewChunk(0, 0)
	ch.SurfaceY[0] = 70 // local x=0,z=0
	ch.SurfaceY[15*ChunkWidth+15] = 99
	w.AddChunk(ch)

	if got := w.GetSurfaceY(0, 0); got != 70 {
		t.Fatalf("GetSurfaceY(0,0)=%d want 70", got)
	}
	if got := w.GetSurfaceY(15, 15); got != 99 {
		t.Fatalf("GetSurfaceY(15,15)=%d want 99", got)
	}
	if got := w.GetSurfaceY(16, 0); got != -999 {
		t.Fatalf("GetSurfaceY missing chunk=%d want -999", got)
	}
}

func TestWorldFindNearestBlockSearchesOnlyLoadedChunks(t *testing.T) {
	w := NewWorld()
	origin := Vec3{X: 0, Y: 64, Z: 0}

	ch0 := NewChunk(0, 0)
	ch0.SetBlock(4, 64, 0, BlockState{ID: 9, Name: "grass_block", Solid: true})
	w.AddChunk(ch0)

	chNeg := NewChunk(-1, -1)
	chNeg.SetBlock(15, 64, 15, BlockState{ID: 10, Name: "dirt", Solid: true})
	w.AddChunk(chNeg)

	hit, ok := w.FindNearestBlock(origin, "dirt", 8)
	if !ok {
		t.Fatal("expected dirt in loaded negative chunk")
	}
	if hit.X != -1 || hit.Y != 64 || hit.Z != -1 {
		t.Fatalf("unexpected dirt hit: %+v", hit)
	}
	if math.Abs(hit.Distance-math.Sqrt2) > 1e-9 {
		t.Fatalf("unexpected distance: got %.6f want %.6f", hit.Distance, math.Sqrt2)
	}

	hit, ok = w.FindNearestBlock(origin, "grass_block", 8)
	if !ok {
		t.Fatal("expected grass_block in loaded chunk")
	}
	if hit.X != 4 || hit.Y != 64 || hit.Z != 0 {
		t.Fatalf("unexpected grass hit: %+v", hit)
	}

	if _, ok := w.FindNearestBlock(origin, "stone", 64); ok {
		t.Fatal("unloaded chunks must not be searched or treated as matching")
	}
}

func TestWorldFindNearestBlockHandlesUnknownNameAndRadius(t *testing.T) {
	w := NewWorld()
	ch := NewChunk(0, 0)
	ch.SetBlock(1, 64, 1, BlockState{ID: 9, Name: "grass_block", Solid: true})
	w.AddChunk(ch)

	if _, ok := w.FindNearestBlock(Vec3{X: 0, Y: 64, Z: 0}, "definitely_not_a_block", 16); ok {
		t.Fatal("unknown block name must not match")
	}
	if _, ok := w.FindNearestBlock(Vec3{X: 0, Y: 64, Z: 0}, "grass_block", 1); ok {
		t.Fatal("block outside radius must not match")
	}
}
