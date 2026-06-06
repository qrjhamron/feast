package feast

import (
	"context"
	"testing"
	"time"

	"github.com/qrjhamron/feast/pkg/state"
)

func TestWaitUntilReadyContextCancel(t *testing.T) {
	c := NewClient(Options{})
	base := c.Events().HandlerCount()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	err := c.WaitUntilReady(ctx)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}

	// The internal readiness handler must not leak after returning.
	if got := c.Events().HandlerCount(); got != base {
		t.Fatalf("WaitUntilReady leaked a handler: base=%d got=%d", base, got)
	}
}

func TestWaitUntilReadyReturnsWhenAlreadySynced(t *testing.T) {
	c := NewClient(Options{})
	base := c.Events().HandlerCount()

	// Simulate a received position sync.
	c.bus.Emit(state.PositionEvent{X: 1, Y: 64, Z: 1, TeleportID: 1})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.WaitUntilReady(ctx); err != nil {
		t.Fatalf("WaitUntilReady should return nil when already synced: %v", err)
	}
	if got := c.Events().HandlerCount(); got != base {
		t.Fatalf("WaitUntilReady leaked a handler on the already-synced path: base=%d got=%d", base, got)
	}
}

func TestWaitUntilReadyWakesOnPositionSync(t *testing.T) {
	c := NewClient(Options{})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- c.WaitUntilReady(ctx) }()

	// Allow the waiter to register before the sync arrives.
	time.Sleep(20 * time.Millisecond)
	c.bus.Emit(state.PositionEvent{X: 0, Y: 64, Z: 0, TeleportID: 2})

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WaitUntilReady returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("WaitUntilReady did not wake on position sync")
	}
}
