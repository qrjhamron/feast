package state

import "testing"

func TestEventBus_OnOffUnsubscribeReset(t *testing.T) {
	bus := NewEventBus()
	called := 0

	id1, err := bus.On("chat", func(e Event) { called++ })
	if err != nil {
		t.Fatalf("unexpected On error: %v", err)
	}
	id2, err := bus.On("chat", func(e Event) { called++ })
	if err != nil {
		t.Fatalf("unexpected On error: %v", err)
	}
	id3, err := bus.On("*", func(e Event) { called++ })
	if err != nil {
		t.Fatalf("unexpected On error: %v", err)
	}

	if bus.HandlerCount() != 3 {
		t.Fatalf("unexpected handler count: got %d want 3", bus.HandlerCount())
	}

	bus.Emit(ChatEvent{Sender: "a", Message: "x"})
	if called != 3 {
		t.Fatalf("unexpected called count after first emit: got %d want 3", called)
	}

	bus.Off(id1)
	bus.Unsubscribe("chat", id2)
	bus.Emit(ChatEvent{Sender: "a", Message: "x"})
	if called != 4 {
		t.Fatalf("unexpected called count after unsubscribe: got %d want 4", called)
	}

	bus.Off(id3)
	if bus.HandlerCount() != 0 {
		t.Fatalf("unexpected handler count after off: got %d want 0", bus.HandlerCount())
	}

	_, err = bus.On("chat", func(e Event) { called++ })
	if err != nil {
		t.Fatalf("unexpected On error: %v", err)
	}
	if bus.HandlerCount() != 1 {
		t.Fatalf("unexpected handler count before reset: got %d want 1", bus.HandlerCount())
	}
	bus.Reset()
	if bus.HandlerCount() != 0 {
		t.Fatalf("unexpected handler count after reset: got %d want 0", bus.HandlerCount())
	}
}

func TestEventBus_OnRejectsNilHandler(t *testing.T) {
	bus := NewEventBus()
	if _, err := bus.On("chat", nil); err == nil {
		t.Fatalf("expected error for nil handler")
	}
}
