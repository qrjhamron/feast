package state

import (
	"fmt"
	"log"
	"sync"
)

// Handler handles one event.
type Handler func(Event)

// EventBus dispatches typed events to handlers.
type EventBus struct {
	mu       sync.RWMutex
	handlers map[string]map[int]Handler
	nextID   int
	index    map[int]string
}

// NewEventBus creates an empty EventBus.
func NewEventBus() *EventBus {
	return &EventBus{
		handlers: make(map[string]map[int]Handler),
		index:    make(map[int]string),
	}
}

// On registers a handler for an event type.
func (b *EventBus) On(eventType string, handler Handler) (int, error) {
	if eventType == "" {
		return 0, fmt.Errorf("event type is required")
	}
	if handler == nil {
		return 0, fmt.Errorf("handler is required")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.handlers[eventType]; !ok {
		b.handlers[eventType] = make(map[int]Handler)
	}
	b.nextID++
	id := b.nextID
	b.handlers[eventType][id] = handler
	b.index[id] = eventType
	return id, nil
}

// Off unregisters a handler by id.
//
// Current implementation keeps compatibility with handler-id API surface.
func (b *EventBus) Off(id int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	eventType, ok := b.index[id]
	if !ok {
		return
	}
	delete(b.index, id)
	if hs, exists := b.handlers[eventType]; exists {
		delete(hs, id)
		if len(hs) == 0 {
			delete(b.handlers, eventType)
		}
	}
}

// Unsubscribe unregisters the given id from the provided event type.
func (b *EventBus) Unsubscribe(eventType string, id int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if hs, ok := b.handlers[eventType]; ok {
		delete(hs, id)
		if len(hs) == 0 {
			delete(b.handlers, eventType)
		}
	}
	delete(b.index, id)
}

// HandlerCount returns the total number of registered handlers.
func (b *EventBus) HandlerCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	count := 0
	for _, hs := range b.handlers {
		count += len(hs)
	}
	return count
}

// Reset clears all non-internal handlers.
func (b *EventBus) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = make(map[string]map[int]Handler)
	b.index = make(map[int]string)
	b.nextID = 0
}

// Emit dispatches an event synchronously to all handlers.
func (b *EventBus) Emit(event Event) {
	t := event.EventType()
	b.mu.RLock()
	target := snapshotHandlers(b.handlers[t])
	wildcard := snapshotHandlers(b.handlers["*"])
	b.mu.RUnlock()
	for _, h := range target {
		safeInvoke(h, event)
	}
	for _, h := range wildcard {
		safeInvoke(h, event)
	}
}

func safeInvoke(h Handler, event Event) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("panic in event handler: %v", r)
		}
	}()
	h(event)
}

func snapshotHandlers(hs map[int]Handler) []Handler {
	if len(hs) == 0 {
		return nil
	}
	out := make([]Handler, 0, len(hs))
	for _, h := range hs {
		out = append(out, h)
	}
	return out
}

func mustString(v any) string {
	s, ok := v.(string)
	if !ok {
		return fmt.Sprint(v)
	}
	return s
}
