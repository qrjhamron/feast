package feast

import (
	"errors"
	"io"
	"net"
	"testing"
	"time"

	feastconn "github.com/qrjhamron/feast/pkg/conn"
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

	_ = c.NavigateTo2(2, 64, 1)
	time.Sleep(250 * time.Millisecond)
	c.StopNavigation()
	time.Sleep(100 * time.Millisecond)

	if got := c.bus.HandlerCount(); got != base {
		t.Fatalf("navigation handlers leaked: got %d want %d", got, base)
	}
}

func TestNavigateToRequiresPositionSync(t *testing.T) {
	c := NewClient(Options{})

	err := c.NavigateTo2(2, 64, 1)
	if !errors.Is(err, ErrPositionNotSynced) {
		t.Fatalf("NavigateTo2 error=%v want ErrPositionNotSynced", err)
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

func TestNavigateTo_PathOverRemovedSupportReplans(t *testing.T) {
	c := NewClient(Options{})
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	c.conn = feastconn.New(a)
	go func() { _, _ = io.Copy(io.Discard, b) }()

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
	c.player.X = 1.5
	c.player.Y = 64.0
	c.player.Z = 1.5
	c.positionSynced = true
	c.stateMu.Unlock()

	failedCh := make(chan state.NavFailedEvent, 1)
	if _, err := c.bus.On("nav_failed", func(e state.Event) {
		if ev, ok := e.(state.NavFailedEvent); ok {
			failedCh <- ev
		}
	}); err != nil {
		t.Fatalf("On: %v", err)
	}

	err := c.NavigateTo2(5, 64, 1)
	if err != nil {
		t.Fatalf("NavigateTo2 failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Remove support at start pos (1, 63, 1)
	ch.SetBlock(1, 63, 1, world.BlockState{ID: 0, Name: "air", Solid: false})

	// Emit block update to trigger footprint invalidation
	c.bus.Emit(state.BlockUpdateEvent{
		X: 1, Y: 63, Z: 1,
		StateID: 0,
	})

	select {
	case ev := <-failedCh:
		if ev.Reason == "" {
			t.Fatalf("expected failure reason, got empty")
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("expected navigation to fail after support was removed, but it did not")
	}
}
