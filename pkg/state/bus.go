package state

import "fmt"

// Handler handles one event.
type Handler func(Event)

// EventBus dispatches typed events to handlers.
type EventBus struct {
	handlers map[string][]Handler
}

// NewEventBus creates an empty EventBus.
func NewEventBus() *EventBus {
	return &EventBus{handlers: make(map[string][]Handler)}
}

// On registers a handler for an event type.
func (b *EventBus) On(eventType string, handler Handler) {
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

// Emit dispatches an event synchronously to all handlers.
func (b *EventBus) Emit(event Event) {
	t := event.EventType()
	for _, h := range b.handlers[t] {
		h(event)
	}
	for _, h := range b.handlers["*"] {
		h(event)
	}
}

func mustString(v any) string {
	s, ok := v.(string)
	if !ok {
		return fmt.Sprint(v)
	}
	return s
}
