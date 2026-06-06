package world

import (
	"testing"
)

// benchWorld builds a deterministic world of size*size chunks centered on the
// origin, each filled with a stone floor at y=63 and a sparse set of "diamond_ore"
// markers, so block lookups and searches exercise realistic data.
func benchWorld(b testing.TB, size int) *World {
	b.Helper()
	w := NewWorld()
	stone := blockStateFromID(mustStateID(b, "stone"))
	air := blockStateFromID(mustStateID(b, "air"))
	ore := blockStateFromID(mustStateID(b, "diamond_ore"))
	half := size / 2
	for cx := -half; cx < size-half; cx++ {
		for cz := -half; cz < size-half; cz++ {
			ch := NewChunk(cx, cz)
			for lx := 0; lx < ChunkWidth; lx++ {
				for lz := 0; lz < ChunkDepth; lz++ {
					ch.SetBlock(lx, 63, lz, stone)
					ch.SetBlock(lx, 64, lz, air)
				}
			}
			// One ore marker per chunk near the floor.
			ch.SetBlock(7, 12, 7, ore)
			w.AddChunk(ch)
		}
	}
	return w
}

func mustStateID(b testing.TB, name string) int32 {
	b.Helper()
	id, ok := FindAnyStateIDByName(name)
	if !ok {
		b.Fatalf("missing state id for %q", name)
	}
	return id
}

func BenchmarkWorldGetBlockLoaded(b *testing.B) {
	w := benchWorld(b, 8)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = w.GetBlock(5, 63, 5)
	}
}

func BenchmarkWorldGetBlockUnloaded(b *testing.B) {
	w := benchWorld(b, 8)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Far outside any loaded chunk.
		_, _ = w.GetBlock(100000, 63, 100000)
	}
}

func BenchmarkWorldIsPassable(b *testing.B) {
	w := benchWorld(b, 8)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Alternate loaded-air / loaded-solid / unloaded to exercise all paths.
		_ = w.IsPassable(5, 64, 5)
		_ = w.IsPassable(5, 63, 5)
		_ = w.IsPassable(100000, 64, 100000)
	}
}

func BenchmarkWorldHasSolidGround(b *testing.B) {
	w := benchWorld(b, 8)
	pos := BlockPos{X: 5, Y: 64, Z: 5}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = w.HasSolidGround(pos)
	}
}

func BenchmarkWorldBlockUpdate(b *testing.B) {
	w := benchWorld(b, 8)
	id := uint16(mustStateID(b, "stone"))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = w.SetBlock(5, 70, 5, id)
	}
}

func BenchmarkWorldGetSurfaceY(b *testing.B) {
	w := benchWorld(b, 8)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = w.GetSurfaceY(5, 5)
	}
}

func BenchmarkWorldFindNearestBlockRadius16(b *testing.B) {
	w := benchWorld(b, 16)
	origin := Vec3{X: 0.5, Y: 64, Z: 0.5}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = w.FindNearestBlock(origin, "diamond_ore", 16)
	}
}

func BenchmarkWorldFindNearestBlockRadius64(b *testing.B) {
	w := benchWorld(b, 16)
	origin := Vec3{X: 0.5, Y: 64, Z: 0.5}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = w.FindNearestBlock(origin, "diamond_ore", 64)
	}
}

func BenchmarkWorldFindNearestBlockSparseMiss(b *testing.B) {
	// A name that does not exist in the loaded world: worst case full scan.
	w := benchWorld(b, 16)
	origin := Vec3{X: 0.5, Y: 64, Z: 0.5}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = w.FindNearestBlock(origin, "emerald_ore", 32)
	}
}

func benchEntityStore(n int) *EntityStore {
	s := NewEntityStore()
	for i := 0; i < n; i++ {
		s.Upsert(&Entity{
			ID:   int32(i),
			Type: "zombie",
			X:    float64(i%32) + 0.5,
			Y:    64,
			Z:    float64(i/32) + 0.5,
		})
	}
	return s
}

func BenchmarkEntityStoreGet(b *testing.B) {
	s := benchEntityStore(256)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = s.Get(int32(i % 256))
	}
}

func BenchmarkEntityStoreSnapshot(b *testing.B) {
	s := benchEntityStore(256)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = s.All()
	}
}

func benchCollisionWorld(n int) (*World, AABB) {
	w := NewWorld()
	store := benchEntityStore(n)
	w.SetEntityStore(store)
	w.SetBotEntityID(-1)
	box := AABB{MinX: 4.0, MinY: 64.0, MinZ: 4.0, MaxX: 6.0, MaxY: 66.0, MaxZ: 6.0}
	return w, box
}

func BenchmarkEntityCollisionFewEntities(b *testing.B) {
	w, box := benchCollisionWorld(8)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = w.IsEntityBlocking(box)
	}
}

func BenchmarkEntityCollisionManyEntities(b *testing.B) {
	w, box := benchCollisionWorld(512)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = w.IsEntityBlocking(box)
	}
}
