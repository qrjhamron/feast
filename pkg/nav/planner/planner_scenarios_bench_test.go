package planner

import (
	"context"
	"testing"
	"time"

	"github.com/qrjhamron/feast/pkg/nav/goal"
	"github.com/qrjhamron/feast/pkg/world"
)

// buildObstacleField builds a 3x3-chunk flat area with a regular grid of
// 2-high pillars, forcing the planner to weave around obstacles.
func buildObstacleField() *world.World {
	w := world.NewWorld()
	for cx := 0; cx < 3; cx++ {
		for cz := 0; cz < 3; cz++ {
			ch := world.NewChunk(cx, cz)
			for x := 0; x < 16; x++ {
				for z := 0; z < 16; z++ {
					ch.SetBlock(x, 63, z, world.BlockState{ID: 1, Name: "stone", Solid: true})
					ch.SurfaceY[z*world.ChunkWidth+x] = 63
				}
			}
			w.AddChunk(ch)
		}
	}
	// Pillars every 3 blocks across the 48x48 area (skip the start cell).
	for x := 2; x < 46; x += 3 {
		for z := 2; z < 46; z += 3 {
			w.SetBlock(x, 64, z, 1)
			w.SetBlock(x, 65, z, 1)
		}
	}
	return w
}

func buildSingleChunkFloor(cx, cz int) *world.World {
	w := world.NewWorld()
	ch := world.NewChunk(cx, cz)
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			ch.SetBlock(x, 63, z, world.BlockState{ID: 1, Name: "stone", Solid: true})
			ch.SurfaceY[z*world.ChunkWidth+x] = 63
		}
	}
	w.AddChunk(ch)
	return w
}

func BenchmarkPlan_ObstacleField(b *testing.B) {
	w := buildObstacleField()
	g := goal.NewGoalBlock(44, 64, 44)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		InvalidateCache()
		_ = Plan(ctx, 1, 64, 1, g, w)
		cancel()
	}
}

// BenchmarkPlan_Unreachable measures the cost of exhausting the frontier when
// the goal lies in unloaded chunks the planner cannot reach.
func BenchmarkPlan_Unreachable(b *testing.B) {
	w := buildSingleChunkFloor(0, 0)
	g := goal.NewGoalBlock(200, 64, 0) // far away, in unloaded chunks
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		InvalidateCache()
		_ = Plan(ctx, 8, 64, 8, g, w)
		cancel()
	}
}

func BenchmarkPlan_NegativeCoordinates(b *testing.B) {
	w := buildSingleChunkFloor(-1, -1) // covers blocks [-16,-1]
	g := goal.NewGoalBlock(-2, 64, -14)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		InvalidateCache()
		_ = Plan(ctx, -14, 64, -2, g, w)
		cancel()
	}
}
