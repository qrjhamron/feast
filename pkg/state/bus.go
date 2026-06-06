package state

import (
	"fmt"
	"log"
	"sync"
)

// Handler handles one event.
type Handler func(Event)

// handlerEntry pairs a handler with the id returned by On so it can be removed
// later. Entries are stored in copy-on-write slices keyed by event type.
type handlerEntry struct {
	id int
	fn Handler
}

// EventBus dispatches typed events to handlers.
//
// Handlers for a given event type are stored as an immutable, copy-on-write
// slice. Subscribe/unsubscribe replace the slice under the write lock, while
// Emit reads the current slice pointer under a short read lock and then invokes
// handlers WITHOUT holding any lock. This gives three properties relied upon by
// the runtime:
//
//   - Emit allocates nothing on the dispatch path (no per-event snapshot copy).
//   - Handlers run in deterministic registration order.
//   - A handler may safely (un)subscribe or call back into the bus from within
//     its own invocation without deadlocking.
type EventBus struct {
	mu       sync.RWMutex
	handlers map[string][]handlerEntry
	index    map[int]string
	nextID   int
}

// NewEventBus creates an empty EventBus.
func NewEventBus() *EventBus {
	return &EventBus{
		handlers: make(map[string][]handlerEntry),
		index:    make(map[int]string),
	}
}

// On registers a handler for an event type and returns its subscription id.
func (b *EventBus) On(eventType string, handler Handler) (int, error) {
	if eventType == "" {
		return 0, fmt.Errorf("event type is required")
	}
	if handler == nil {
		return 0, fmt.Errorf("handler is required")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	id := b.nextID
	old := b.handlers[eventType]
	// Copy-on-write: never mutate a slice that a concurrent Emit may be reading.
	next := make([]handlerEntry, len(old)+1)
	copy(next, old)
	next[len(old)] = handlerEntry{id: id, fn: handler}
	b.handlers[eventType] = next
	b.index[id] = eventType
	return id, nil
}

// Off unregisters a handler by id. Calling Off with an unknown or already
// removed id is a no-op, so it is idempotent.
func (b *EventBus) Off(id int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	eventType, ok := b.index[id]
	if !ok {
		return
	}
	delete(b.index, id)
	b.removeLocked(eventType, id)
}

// Unsubscribe unregisters the given id from the provided event type. It is
// idempotent and tolerates a mismatched event type.
func (b *EventBus) Unsubscribe(eventType string, id int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.index, id)
	b.removeLocked(eventType, id)
}

// removeLocked rebuilds the handler slice for eventType without id. The caller
// must hold b.mu for writing.
func (b *EventBus) removeLocked(eventType string, id int) {
	old, ok := b.handlers[eventType]
	if !ok {
		return
	}
	idx := -1
	for i := range old {
		if old[i].id == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	if len(old) == 1 {
		// Removing the last handler: drop the key so has()/Emit see no handlers.
		delete(b.handlers, eventType)
		return
	}
	next := make([]handlerEntry, 0, len(old)-1)
	next = append(next, old[:idx]...)
	next = append(next, old[idx+1:]...)
	b.handlers[eventType] = next
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

// Reset clears ALL registered handlers. The feast runtime re-registers its
// internal handlers immediately after calling Reset during reconnect teardown,
// so a full clear (rather than a partial one) is the intended contract.
func (b *EventBus) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers = make(map[string][]handlerEntry)
	b.index = make(map[int]string)
	b.nextID = 0
}

// has reports whether at least one handler is registered for eventType. Because
// removeLocked deletes empty keys, presence of the key implies a non-empty list.
func (b *EventBus) has(eventType string) bool {
	b.mu.RLock()
	_, ok := b.handlers[eventType]
	b.mu.RUnlock()
	return ok
}

// Emit dispatches an event synchronously to all handlers for its type and to
// wildcard ("*") handlers, in registration order. The bus lock is not held
// while handlers run.
func (b *EventBus) Emit(event Event) {
	t := event.EventType()
	b.mu.RLock()
	target := b.handlers[t]
	wildcard := b.handlers["*"]
	b.mu.RUnlock()
	// target and wildcard are immutable copy-on-write snapshots; ranging them
	// after releasing the lock is race-free even if a handler mutates the bus.
	for i := range target {
		safeInvoke(target[i].fn, event)
	}
	for i := range wildcard {
		safeInvoke(wildcard[i].fn, event)
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
