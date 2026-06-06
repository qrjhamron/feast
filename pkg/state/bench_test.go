package state

import (
	"testing"

	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/protocol/consts"
)

// noopHandler is a handler that does the minimum observable work so the
// compiler cannot optimize the dispatch away.
func benchNoop(sink *int) Handler {
	return func(Event) { *sink++ }
}

func BenchmarkBusDispatchNoHandlers(b *testing.B) {
	bus := NewEventBus()
	ev := ChatEvent{Sender: "system", Message: "hello world"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Emit(ev)
	}
}

func BenchmarkBusDispatchOneHandler(b *testing.B) {
	bus := NewEventBus()
	sink := 0
	_, _ = bus.On("chat", benchNoop(&sink))
	ev := ChatEvent{Sender: "system", Message: "hello world"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Emit(ev)
	}
	_ = sink
}

func BenchmarkBusDispatchManyHandlers(b *testing.B) {
	bus := NewEventBus()
	sink := 0
	for i := 0; i < 16; i++ {
		_, _ = bus.On("chat", benchNoop(&sink))
	}
	ev := ChatEvent{Sender: "system", Message: "hello world"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bus.Emit(ev)
	}
	_ = sink
}

func BenchmarkBusSubscribeUnsubscribe(b *testing.B) {
	bus := NewEventBus()
	sink := 0
	h := benchNoop(&sink)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		id, _ := bus.On("chat", h)
		bus.Off(id)
	}
}

// Dispatcher benchmarks intentionally do NOT subscribe to the catch-all
// "packet" event, mirroring the production feast client (which only subscribes
// to typed events). This measures the realistic per-packet decode + emit cost.

func benchDispatcher(b *testing.B, eventType string, raw *protocol.RawPacket) {
	b.Helper()
	bus := NewEventBus()
	d := NewDispatcher(bus)
	sink := 0
	_, _ = bus.On(eventType, benchNoop(&sink))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = d.Dispatch(StatePlay, raw)
	}
	_ = sink
}

func BenchmarkDispatcherBlockUpdate(b *testing.B) {
	raw := marshalRaw(consts.PlayClientboundBlockUpdate, &protocol.PlayClientboundBlockUpdatePacket{
		Position: protocol.BlockPos{X: -133, Y: 67, Z: 412}, StateID: 124,
	})
	benchDispatcher(b, "block_update", raw)
}

func BenchmarkDispatcherSectionBlockUpdate(b *testing.B) {
	changes := make([]protocol.SectionBlockChange, 0, 32)
	for i := 0; i < 32; i++ {
		changes = append(changes, protocol.SectionBlockChange{LocalPos: uint16(i*0x111) & 0xFFF, StateID: int32(i + 1)})
	}
	raw := marshalRaw(consts.PlayClientboundUpdateSectionBlocks, &protocol.PlayClientboundUpdateSectionBlocksPacket{
		SectionPos: protocol.ChunkSectionPos{X: 2, Y: 4, Z: -3}, Changes: changes,
	})
	benchDispatcher(b, "section_blocks_update", raw)
}

func BenchmarkDispatcherEntityMove(b *testing.B) {
	raw := marshalRaw(consts.PlayClientboundUpdateEntityPositionAndRotation, &protocol.PlayClientboundUpdateEntityPositionAndRotationPacket{
		EntityID: 42, DX: 64, DY: -32, DZ: 16, Yaw: 64, Pitch: 32, OnGround: true,
	})
	// This packet emits both move + rotate; subscribe to move.
	benchDispatcher(b, "entity_move_delta", raw)
}

func BenchmarkDispatcherSetSlot(b *testing.B) {
	raw := marshalRaw(consts.PlayClientboundSetContainerSlot, &protocol.PlayClientboundSetContainerSlotPacket{
		WindowID: 0, StateID: 7, Slot: 36,
		Item: protocol.ItemStack{Present: true, ItemID: 1, Count: 64},
	})
	benchDispatcher(b, "inventory_slot", raw)
}
