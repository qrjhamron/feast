package planner

import (
	"context"
	"testing"
	"time"

	"github.com/user/feastgo/pkg/nav/goal"
	"github.com/user/feastgo/pkg/world"
)

func buildOpenWorldForBench(length int) *world.World {
	w := world.NewWorld()

	// Create a flat, unobstructed strip along +X at y=64 with a solid floor at y=63.
	// We allocate chunks along X to cover [0,length].
	chunksX := (length / 16) + 2
	for cx := 0; cx < chunksX; cx++ {
		ch := world.NewChunk(cx, 0)
		for x := 0; x < 16; x++ {
			for z := 0; z < 16; z++ {
				ch.SetBlock(x, 63, z, world.BlockState{ID: 1, Name: "stone", Solid: true})
				// Everything else defaults to air in the chunk constructor.
			}
		}
		w.AddChunk(ch)
	}

	return w
}

func BenchmarkPlan_200Steps(b *testing.B) {
	w := buildOpenWorldForBench(220)
	g := goal.NewGoalBlock(200, 64, 0)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		InvalidateCache()
		_ = Plan(ctx, 0, 64, 0, g, w)
		cancel()
	}
}

func BenchmarkPlan_200Steps_Cached(b *testing.B) {
	w := buildOpenWorldForBench(220)
	g := goal.NewGoalBlock(200, 64, 0)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	_ = Plan(ctx, 0, 64, 0, g, w)
	cancel()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_ = Plan(ctx, 0, 64, 0, g, w)
		cancel()
	}
}
