package feast

import (
	"testing"
	"time"

	"github.com/qrjhamron/feast/pkg/state"
	"github.com/qrjhamron/feast/pkg/world"
)

func TestNavigateTo_BlockUpdateHandlersUnsubscribed(t *testing.T) {
	c := NewClient(Options{})
	base := c.bus.HandlerCount()

	ch := world.NewChunk(0, 0)
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			ch.SetBlock(x, 63, z, world.BlockState{ID: 1, Name: "stone", Solid: true})
			ch.SetBlock(x, 64, z, world.BlockState{ID: 0, Name: "air", Solid: false})
			ch.SetBlock(x, 65, z, world.BlockState{ID: 0, Name: "air", Solid: false})
		}
	}
	c.World().AddChunk(ch)

	c.stateMu.Lock()
	c.player.X = 1
	c.player.Y = 64
	c.player.Z = 1
	c.positionSynced = true
	c.stateMu.Unlock()

	_ = c.NavigateTo(2, 64, 1)
	time.Sleep(250 * time.Millisecond)
	c.StopNavigation()
	time.Sleep(100 * time.Millisecond)

	if got := c.bus.HandlerCount(); got != base {
		t.Fatalf("navigation handlers leaked: got %d want %d", got, base)
	}
}

func TestNavigateToRequiresPositionSync(t *testing.T) {
	c := NewClient(Options{})

	err := c.NavigateTo(2, 64, 1)
	if err == nil || err.Error() != "position not synced yet" {
		t.Fatalf("NavigateTo error=%v want position not synced yet", err)
	}
}

func TestNavigationGoalYZeroUsesXZOnlyGoal(t *testing.T) {
	c := NewClient(Options{})
	ch := world.NewChunk(0, 0)
	ch.SurfaceY[1*world.ChunkWidth+2] = 66
	ch.SetBlock(2, 66, 1, world.BlockState{ID: 1, Name: "stone", Solid: true})
	ch.SetBlock(2, 67, 1, world.AirBlockState)
	ch.SetBlock(2, 68, 1, world.AirBlockState)
	c.World().AddChunk(ch)

	g, goalY := c.navigationGoal(2, 0, 1, 64)
	if goalY != 67 {
		t.Fatalf("goalY=%d want 67", goalY)
	}
	if !g.Satisfied(2, 64, 1) {
		t.Fatalf("y=0 navigation goal should be satisfied by matching X/Z even when Y differs")
	}

	explicit, explicitY := c.navigationGoal(2, 67, 1, 64)
	if explicitY != 67 {
		t.Fatalf("explicitY=%d want 67", explicitY)
	}
	if explicit.Satisfied(2, 64, 1) {
		t.Fatalf("explicit-Y navigation goal should not ignore Y")
	}
}

func TestDisconnect_ResetsWorldEntitiesAndBusHandlers(t *testing.T) {
	c := NewClient(Options{})
	base := c.bus.HandlerCount()

	ch := world.NewChunk(0, 0)
	c.World().AddChunk(ch)
	c.entities.Upsert(&world.Entity{ID: 7, X: 1, Y: 64, Z: 1})
	if _, err := c.bus.On("custom", func(_ state.Event) {}); err != nil {
		t.Fatalf("On: %v", err)
	}

	if err := c.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	if _, err := c.World().GetBlock(0, 0, 0); err == nil {
		t.Fatalf("expected world to be reset and chunk removed")
	}
	if got := len(c.Entities().All()); got != 0 {
		t.Fatalf("expected empty entity store, got %d", got)
	}
	if got := c.bus.HandlerCount(); got != base {
		t.Fatalf("unexpected handler count after disconnect reset: got %d want %d", got, base)
	}
}
