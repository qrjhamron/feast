package feast

import (
	"testing"

	"github.com/qrjhamron/feast/pkg/world"
)

// buildSurfaceChunk creates a chunk at (0,0) whose cached surface Y for column
// (8,8) is high, while leaving the underground area as air. This lets us prove
// the planner start node ignores the surface cache.
func buildSurfaceChunk(surfaceY int) *world.Chunk {
	ch := world.NewChunk(0, 0)
	ch.SetBlock(8, surfaceY, 8, world.BlockState{Name: "grass_block", Solid: true})
	idx := 8*world.ChunkWidth + 8
	if idx >= 0 && idx < len(ch.SurfaceY) {
		ch.SurfaceY[idx] = int16(surfaceY)
	}
	return ch
}

func TestNavigateStartYFromCurrentFeet(t *testing.T) {
	c := NewClient(Options{})
	c.World().AddChunk(buildSurfaceChunk(63))

	// Bot resting on the surface at feet Y=64.
	planY, ok := c.resolveStartY(8, 64, 8)
	if !ok {
		t.Fatal("expected chunk loaded -> ok")
	}
	if planY != 64 {
		t.Fatalf("planY=%d; want current feet Y 64", planY)
	}
}

func TestNavigateDoesNotSnapUndergroundBotToSurface(t *testing.T) {
	c := NewClient(Options{})
	c.World().AddChunk(buildSurfaceChunk(63))

	// Bot is underground at feet Y=30. The old code snapped to surface+1=64.
	planY, ok := c.resolveStartY(8, 30, 8)
	if !ok {
		t.Fatal("expected chunk loaded -> ok")
	}
	if planY != 30 {
		t.Fatalf("planY=%d; underground bot must NOT be snapped to surface (want 30)", planY)
	}

	// Slightly-above-surface case (e.g. on a 2-block platform) must also keep feet Y.
	planY, _ = c.resolveStartY(8, 68, 8)
	if planY != 68 {
		t.Fatalf("planY=%d; elevated bot must keep feet Y 68", planY)
	}
}

func TestResolveStartYRejectsUnloadedColumn(t *testing.T) {
	c := NewClient(Options{})
	// No chunks loaded.
	if _, ok := c.resolveStartY(8, 64, 8); ok {
		t.Fatal("unloaded column must report ok=false")
	}
}
