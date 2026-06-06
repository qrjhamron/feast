package feast

import (
	"github.com/qrjhamron/feast/pkg/state"
	"testing"
)

func TestEventHooks_Unsubscribe(t *testing.T) {
	c := NewClient(Options{})

	called1 := 0
	called2 := 0

	unsub1 := c.OnChat(func(e ChatEvent) {
		called1++
	})
	unsub2 := c.OnChat(func(e ChatEvent) {
		called2++
	})

	if unsub1 == nil || unsub2 == nil {
		t.Fatal("expected non-nil unsubscribe functions")
	}

	c.bus.Emit(state.ChatEvent{Sender: "test", Message: "hello"})
	if called1 != 1 || called2 != 1 {
		t.Fatalf("expected both handlers to be called once, got %d, %d", called1, called2)
	}

	unsub1()

	c.bus.Emit(state.ChatEvent{Sender: "test", Message: "hello again"})
	if called1 != 1 {
		t.Fatalf("expected handler 1 not to be called again, got %d", called1)
	}
	if called2 != 2 {
		t.Fatalf("expected handler 2 to be called twice, got %d", called2)
	}
}
