package world

import (
	"errors"
	"sync"
	"testing"

	"github.com/qrjhamron/feast/pkg/protocol"
)

func TestWorldErrorsWrapSentinel(t *testing.T) {
	w := NewWorld()

	// Unloaded chunk must satisfy BOTH sentinels (preserves prior contract).
	_, err := w.GetBlock(0, 64, 0)
	if !errors.Is(err, ErrBlockNotFound) || !errors.Is(err, ErrChunkNotLoaded) {
		t.Fatalf("unloaded GetBlock err=%v must wrap ErrBlockNotFound and ErrChunkNotLoaded", err)
	}

	// Y out of bounds wraps ErrBlockNotFound (existing contract) and the new
	// ErrBlockOutOfBounds sentinel.
	_, err = w.GetBlock(0, MaxY+1, 0)
	if !errors.Is(err, ErrBlockNotFound) || !errors.Is(err, ErrBlockOutOfBounds) {
		t.Fatalf("oob GetBlock err=%v must wrap ErrBlockNotFound and ErrBlockOutOfBounds", err)
	}

	if err := w.RequireBlockLoaded(BlockPos{X: 0, Y: 64, Z: 0}); !errors.Is(err, ErrChunkNotLoaded) {
		t.Fatalf("RequireBlockLoaded err=%v want ErrChunkNotLoaded", err)
	}
	if err := w.RequireBlockLoaded(BlockPos{X: 0, Y: int32(MaxY + 1), Z: 0}); !errors.Is(err, ErrBlockOutOfBounds) {
		t.Fatalf("RequireBlockLoaded oob err=%v want ErrBlockOutOfBounds", err)
	}
}

func TestWorldChunkUnloadRemovesBlocksAndBlockEntities(t *testing.T) {
	w := NewWorld()
	ch := NewChunk(3, -2)
	ch.SetBlock(1, 64, 1, BlockState{Name: "chest", Solid: true, ID: 1})
	ch.BlockEntities = []BlockEntity{{X: 49, Y: 64, Z: -31, TypeID: 1}}
	w.AddChunk(ch)

	if w.ChunkCount() != 1 {
		t.Fatalf("ChunkCount=%d want 1", w.ChunkCount())
	}
	if len(w.GetBlockEntities(3, -2)) != 1 {
		t.Fatal("expected block entity present before unload")
	}
	// world coords of local (1,64,1) in chunk (3,-2): x=49, z=-31.
	if _, err := w.GetBlock(49, 64, -31); err != nil {
		t.Fatalf("expected block present before unload: %v", err)
	}

	w.RemoveChunk(3, -2)

	if w.ChunkCount() != 0 {
		t.Fatalf("ChunkCount after unload=%d want 0", w.ChunkCount())
	}
	if w.GetBlockEntities(3, -2) != nil {
		t.Fatal("block entities must be gone after chunk unload")
	}
	if _, err := w.GetBlock(49, 64, -31); !errors.Is(err, ErrChunkNotLoaded) {
		t.Fatalf("block must be inaccessible after unload, err=%v", err)
	}
	if w.IsBlockLoaded(BlockPos{X: 49, Y: 64, Z: -31}) {
		t.Fatal("block must not report loaded after unload")
	}
}

func TestWorldChunkCountAccuracy(t *testing.T) {
	w := NewWorld()
	w.AddChunk(NewChunk(0, 0))
	w.AddChunk(NewChunk(1, 0))
	if w.ChunkCount() != 2 {
		t.Fatalf("ChunkCount=%d want 2", w.ChunkCount())
	}
	// Re-adding the same coordinate must not double count (servers resend chunks).
	w.AddChunk(NewChunk(0, 0))
	if w.ChunkCount() != 2 {
		t.Fatalf("ChunkCount after re-add=%d want 2", w.ChunkCount())
	}
	w.RemoveChunk(0, 0)
	if w.ChunkCount() != 1 {
		t.Fatalf("ChunkCount after remove=%d want 1", w.ChunkCount())
	}
	// Removing an absent chunk must not underflow.
	w.RemoveChunk(42, 42)
	if w.ChunkCount() != 1 {
		t.Fatalf("ChunkCount after remove-absent=%d want 1", w.ChunkCount())
	}
	w.Reset()
	if w.ChunkCount() != 0 {
		t.Fatalf("ChunkCount after reset=%d want 0", w.ChunkCount())
	}
}

func TestWorldSectionYBounds(t *testing.T) {
	w := NewWorld()
	ch := NewChunk(0, 0)
	stone := BlockState{Name: "stone", Solid: true, ID: 1}
	for _, y := range []int{MinY, MinY + 1, -1, 0, 100, MaxY - 1, MaxY} {
		if !ch.SetBlock(0, y, 0, stone) {
			t.Fatalf("SetBlock failed at in-range y=%d", y)
		}
	}
	if ch.SetBlock(0, MinY-1, 0, stone) {
		t.Fatal("SetBlock should reject y<MinY")
	}
	if ch.SetBlock(0, MaxY+1, 0, stone) {
		t.Fatal("SetBlock should reject y>MaxY")
	}
	w.AddChunk(ch)

	for _, y := range []int{MinY, MaxY} {
		if _, err := w.GetBlock(0, y, 0); err != nil {
			t.Fatalf("GetBlock(y=%d): %v", y, err)
		}
	}
	if _, err := w.GetBlock(0, MinY-1, 0); !errors.Is(err, ErrBlockOutOfBounds) {
		t.Fatalf("y<MinY err=%v want ErrBlockOutOfBounds", err)
	}
	if _, err := w.GetBlock(0, MaxY+1, 0); !errors.Is(err, ErrBlockOutOfBounds) {
		t.Fatalf("y>MaxY err=%v want ErrBlockOutOfBounds", err)
	}
}

func TestFindNearestBlockRadiusBoundary(t *testing.T) {
	w := NewWorld()
	ch := NewChunk(0, 0)
	ch.SetBlock(5, 64, 0, BlockState{Name: "diamond_ore", Solid: true, ID: 1})
	w.AddChunk(ch)
	origin := Vec3{X: 0, Y: 64, Z: 0}

	if _, ok := w.FindNearestBlock(origin, "diamond_ore", 5); !ok {
		t.Fatal("block at distance 5 must be included at radius 5")
	}
	if _, ok := w.FindNearestBlock(origin, "diamond_ore", 4); ok {
		t.Fatal("block at distance 5 must be excluded at radius 4")
	}
}

func TestFindNearestBlockOrdering(t *testing.T) {
	build := func() BlockHit {
		w := NewWorld()
		ch := NewChunk(0, 0)
		ore := BlockState{Name: "diamond_ore", Solid: true, ID: 1}
		ch.SetBlock(2, 64, 0, ore) // distance 2
		ch.SetBlock(0, 64, 2, ore) // distance 2 (tie)
		w.AddChunk(ch)
		hit, ok := w.FindNearestBlock(Vec3{X: 0, Y: 64, Z: 0}, "diamond_ore", 8)
		if !ok {
			t.Fatal("expected a hit")
		}
		return hit
	}
	first := build()
	for i := 0; i < 25; i++ {
		got := build()
		if got.X != first.X || got.Y != first.Y || got.Z != first.Z {
			t.Fatalf("tie-break is non-deterministic: %+v vs %+v", got, first)
		}
	}
}

func TestFindBlocksLimit(t *testing.T) {
	w := NewWorld()
	ch := NewChunk(0, 0)
	ore := BlockState{Name: "diamond_ore", Solid: true, ID: 1}
	ch.SetBlock(1, 64, 0, ore)
	ch.SetBlock(2, 64, 0, ore)
	ch.SetBlock(3, 64, 0, ore)
	w.AddChunk(ch)

	hits := w.FindBlocks(Vec3{X: 0, Y: 64, Z: 0}, "diamond_ore", 2, 16)
	if len(hits) != 2 {
		t.Fatalf("want 2 hits (capped to count), got %d", len(hits))
	}
	if hits[0].Distance > hits[1].Distance {
		t.Fatalf("hits must be sorted nearest-first: %v", hits)
	}
	if hits[0].X != 1 {
		t.Fatalf("nearest hit should be x=1, got x=%d", hits[0].X)
	}
}

func TestEntityCollisionExcludesBot(t *testing.T) {
	w := NewWorld()
	store := NewEntityStore()
	w.SetEntityStore(store)
	store.Upsert(&Entity{ID: 7, Type: "zombie", X: 5.0, Y: 64.0, Z: 5.0})

	box := AABB{MinX: 4.9, MinY: 64, MinZ: 4.9, MaxX: 5.1, MaxY: 65, MaxZ: 5.1}
	if !w.IsEntityBlocking(box) {
		t.Fatal("entity should block before being marked as the bot")
	}
	w.SetBotEntityID(7)
	if w.IsEntityBlocking(box) {
		t.Fatal("the bot's own entity must be excluded from collision checks")
	}
}

func TestUnknownEntitySafeDefaultHitbox(t *testing.T) {
	name := EntityTypeFromID(999999)
	if name == "" {
		t.Fatal("unknown entity id should map to a non-empty placeholder type")
	}
	box := HitboxForEntityType(name, protocol.PoseStanding)
	if box.MaxY != 1.8 || box.MaxX != 0.3 || box.MinX != -0.3 {
		t.Fatalf("unknown entity should get a safe default hitbox, got %+v", box)
	}
}

func TestFindAnyStateIDByNameDeterministic(t *testing.T) {
	id1, ok := FindAnyStateIDByName("stone")
	if !ok {
		t.Fatal("stone must resolve to a state id")
	}
	for i := 0; i < 10; i++ {
		id2, ok := FindAnyStateIDByName("stone")
		if !ok || id2 != id1 {
			t.Fatalf("FindAnyStateIDByName must be deterministic: %d vs %d", id2, id1)
		}
	}
	if ResolveBlockName(id1) != "stone" {
		t.Fatalf("state id %d does not resolve back to stone", id1)
	}
	if _, ok := FindAnyStateIDByName("definitely_not_a_block"); ok {
		t.Fatal("unknown block name must return ok=false")
	}
}

func TestWorldConcurrentMixedOps(t *testing.T) {
	t.Parallel()
	w := NewWorld()
	store := NewEntityStore()
	w.SetEntityStore(store)

	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		g := g
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 400; i++ {
				ch := NewChunk(g, i%8)
				ch.SetBlock(0, 64, 0, BlockState{Name: "stone", Solid: true, ID: 1})
				w.AddChunk(ch)
				w.RemoveChunk(g, i%8)
			}
		}()
	}
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 400; i++ {
				_, _ = w.GetBlock(0, 64, 0)
				_ = w.IsPassable(0, 64, 0)
				_, _ = w.FindNearestBlock(Vec3{X: 0, Y: 64, Z: 0}, "stone", 8)
				store.Upsert(&Entity{ID: int32(i % 16), Type: "pig", X: 0.5, Y: 64, Z: 0.5})
				_ = w.IsEntityBlocking(AABB{MinX: 0, MinY: 64, MinZ: 0, MaxX: 1, MaxY: 65, MaxZ: 1})
				_ = w.ChunkCount()
			}
		}()
	}
	wg.Wait()
}
