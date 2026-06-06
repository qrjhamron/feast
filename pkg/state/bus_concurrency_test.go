package state

import (
	"bytes"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/protocol/consts"
)

func TestBusUnsubscribeIdempotent(t *testing.T) {
	bus := NewEventBus()
	id, err := bus.On("chat", func(Event) {})
	if err != nil {
		t.Fatalf("On: %v", err)
	}
	bus.Off(id)
	bus.Off(id)                 // repeat: must be a no-op, not panic
	bus.Unsubscribe("chat", id) // already gone
	bus.Unsubscribe("nope", 999999)
	if bus.HandlerCount() != 0 {
		t.Fatalf("HandlerCount=%d want 0 after idempotent unsubscribe", bus.HandlerCount())
	}
}

func TestBusDispatchDoesNotHoldLockDuringHandler(t *testing.T) {
	bus := NewEventBus()
	done := make(chan struct{})
	_, _ = bus.On("chat", func(Event) {
		// These calls take the bus lock. If Emit held a lock across handler
		// invocation, they would deadlock forever.
		_ = bus.HandlerCount()
		_, _ = bus.On("other", func(Event) {})
		close(done)
	})
	go bus.Emit(ChatEvent{Message: "x"})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler ran while bus lock was held (deadlock)")
	}
}

func TestBusHandlerCanUnsubscribeItself(t *testing.T) {
	bus := NewEventBus()
	calls := 0
	var id int
	id, _ = bus.On("chat", func(Event) {
		calls++
		bus.Off(id)
	})
	bus.Emit(ChatEvent{})
	bus.Emit(ChatEvent{})
	if calls != 1 {
		t.Fatalf("handler should run once then remove itself, ran %d times", calls)
	}
}

func TestBusDeterministicHandlerOrder(t *testing.T) {
	for trial := 0; trial < 25; trial++ {
		bus := NewEventBus()
		var order []int
		for i := 0; i < 8; i++ {
			i := i
			_, _ = bus.On("chat", func(Event) { order = append(order, i) })
		}
		bus.Emit(ChatEvent{})
		for i := 0; i < 8; i++ {
			if order[i] != i {
				t.Fatalf("handlers must run in registration order, got %v", order)
			}
		}
	}
}

func TestBusConcurrentSubscribeDispatchUnsubscribe(t *testing.T) {
	bus := NewEventBus()
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ev := ChatEvent{Message: "x"}
			for {
				select {
				case <-stop:
					return
				default:
					bus.Emit(ev)
				}
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 3000; j++ {
				id, _ := bus.On("chat", func(Event) {})
				bus.Off(id)
			}
		}()
	}
	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()
}

func TestDispatcherEventOrderForChunkThenBlockUpdate(t *testing.T) {
	bus := NewEventBus()
	d := NewDispatcher(bus)
	var order []string
	_, _ = bus.On("chunk_load", func(Event) { order = append(order, "chunk_load") })
	_, _ = bus.On("block_update", func(Event) { order = append(order, "block_update") })

	chunkRaw := marshalRaw(consts.PlayClientboundChunkDataAndUpdateLight, &protocol.PlayClientboundChunkDataAndUpdateLightPacket{
		ChunkX: 1, ChunkZ: 2, TailData: []byte{0x00},
	})
	blockRaw := marshalRaw(consts.PlayClientboundBlockUpdate, &protocol.PlayClientboundBlockUpdatePacket{
		Position: protocol.BlockPos{X: 16, Y: 64, Z: 32}, StateID: 1,
	})

	if err := d.Dispatch(StatePlay, chunkRaw); err != nil {
		t.Fatalf("dispatch chunk: %v", err)
	}
	if err := d.Dispatch(StatePlay, blockRaw); err != nil {
		t.Fatalf("dispatch block: %v", err)
	}
	if len(order) != 2 || order[0] != "chunk_load" || order[1] != "block_update" {
		t.Fatalf("events delivered out of order: %v", order)
	}
}

func TestDispatcherErrorEventDoesNotDeadlock(t *testing.T) {
	bus := NewEventBus()
	done := make(chan struct{})
	var id int
	id, _ = bus.On("error", func(Event) {
		// Mirror teardown handlers that touch the bus from within a handler.
		bus.Off(id)
		_ = bus.HandlerCount()
		close(done)
	})
	go bus.Emit(ErrorEvent{Op: "read_loop", Error: errors.New("boom")})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("error event handler deadlocked")
	}
}

func TestDispatcherUnloadChunkEmitsEvent(t *testing.T) {
	bus := NewEventBus()
	d := NewDispatcher(bus)
	var got ChunkUnloadEvent
	seen := false
	_, _ = bus.On("chunk_unload", func(e Event) {
		got = e.(ChunkUnloadEvent)
		seen = true
	})

	// 1.20.2+ Unload Chunk: 8-byte big-endian long, Chunk Z in the high 32 bits,
	// Chunk X in the low 32 bits. 0x00000001_00000002 => Z=1, X=2.
	raw := &protocol.RawPacket{
		ID:   consts.PlayClientboundUnloadChunk,
		Data: []byte{0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x02},
	}
	if err := d.Dispatch(StatePlay, raw); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !seen {
		t.Fatal("expected chunk_unload event")
	}
	if got.ChunkX != 2 || got.ChunkZ != 1 {
		t.Fatalf("unload coords got X=%d Z=%d want X=2 Z=1", got.ChunkX, got.ChunkZ)
	}

	// Negative coordinates: Z=-5 (0xFFFFFFFB) high, X=-1 (0xFFFFFFFF) low.
	got = ChunkUnloadEvent{}
	rawNeg := &protocol.RawPacket{
		ID:   consts.PlayClientboundUnloadChunk,
		Data: []byte{0xFF, 0xFF, 0xFF, 0xFB, 0xFF, 0xFF, 0xFF, 0xFF},
	}
	if err := d.Dispatch(StatePlay, rawNeg); err != nil {
		t.Fatalf("dispatch neg: %v", err)
	}
	if got.ChunkX != -1 || got.ChunkZ != -5 {
		t.Fatalf("negative unload coords got X=%d Z=%d want X=-1 Z=-5", got.ChunkX, got.ChunkZ)
	}

	// A short payload must error cleanly rather than panic.
	if err := d.Dispatch(StatePlay, &protocol.RawPacket{ID: consts.PlayClientboundUnloadChunk, Data: []byte{1, 2, 3}}); err == nil {
		t.Fatal("expected error on short unload payload")
	}
}

func TestDispatcherPacketEventGuard(t *testing.T) {
	raw := marshalRaw(consts.PlayClientboundBlockUpdate, &protocol.PlayClientboundBlockUpdatePacket{
		Position: protocol.BlockPos{X: 1, Y: 64, Z: 1}, StateID: 1,
	})

	// No "packet"/"*" subscriber: the typed event must still fire.
	bus := NewEventBus()
	d := NewDispatcher(bus)
	blockSeen := false
	_, _ = bus.On("block_update", func(Event) { blockSeen = true })
	if err := d.Dispatch(StatePlay, raw); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !blockSeen {
		t.Fatal("typed event must fire even without a packet subscriber")
	}

	// With a "packet" subscriber: PacketEvent fires with an independent copy.
	bus2 := NewEventBus()
	d2 := NewDispatcher(bus2)
	var pe PacketEvent
	pktSeen := false
	_, _ = bus2.On("packet", func(e Event) {
		pe = e.(PacketEvent)
		pktSeen = true
	})
	if err := d2.Dispatch(StatePlay, raw); err != nil {
		t.Fatalf("dispatch2: %v", err)
	}
	if !pktSeen || pe.ID != consts.PlayClientboundBlockUpdate {
		t.Fatal("packet event should fire for a packet subscriber")
	}
	orig := append([]byte(nil), raw.Data...)
	for i := range raw.Data {
		raw.Data[i] ^= 0xFF
	}
	if !bytes.Equal(pe.Data, orig) {
		t.Fatal("PacketEvent.Data must be an independent copy of the packet payload")
	}
}
