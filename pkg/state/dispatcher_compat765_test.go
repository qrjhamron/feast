package state

import (
	"bytes"
	"sync"
	"testing"

	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/protocol/consts"
)

// bundleDelim is a convenience for a clientbound Bundle Delimiter raw packet.
func bundleDelim() *protocol.RawPacket {
	return &protocol.RawPacket{ID: consts.PlayClientboundBundleDelimiter, Data: nil}
}

// --- Bundle Delimiter --------------------------------------------------------

func TestDispatcherBundleDelimiterDoesNotError(t *testing.T) {
	bus := NewEventBus()
	d := NewDispatcher(bus)
	if err := d.Dispatch(StatePlay, bundleDelim()); err != nil {
		t.Fatalf("dispatch bundle delimiter: %v", err)
	}
	// A misframed delimiter carrying a stray byte must also be non-fatal.
	if err := d.Dispatch(StatePlay, &protocol.RawPacket{ID: consts.PlayClientboundBundleDelimiter, Data: []byte{0x00}}); err != nil {
		t.Fatalf("dispatch misframed bundle delimiter: %v", err)
	}
}

func TestDispatcherEmptyBundleSafe(t *testing.T) {
	bus := NewEventBus()
	d := NewDispatcher(bus)

	var blockEvents int
	if _, err := bus.On("block_update", func(Event) { blockEvents++ }); err != nil {
		t.Fatalf("On: %v", err)
	}
	// An empty bundle is two adjacent delimiters with nothing in between.
	if err := d.Dispatch(StatePlay, bundleDelim()); err != nil {
		t.Fatalf("dispatch open delimiter: %v", err)
	}
	if err := d.Dispatch(StatePlay, bundleDelim()); err != nil {
		t.Fatalf("dispatch close delimiter: %v", err)
	}
	if blockEvents != 0 {
		t.Fatalf("empty bundle produced %d block events, want 0", blockEvents)
	}
}

func TestDispatcherBundleBlockUpdateStillEmitsEvent(t *testing.T) {
	bus := NewEventBus()
	d := NewDispatcher(bus)

	got := make(chan BlockUpdateEvent, 1)
	if _, err := bus.On("block_update", func(e Event) {
		if ev, ok := e.(BlockUpdateEvent); ok {
			got <- ev
		}
	}); err != nil {
		t.Fatalf("On: %v", err)
	}

	seq := []*protocol.RawPacket{
		bundleDelim(),
		rawFromPacket(t, &protocol.PlayClientboundBlockUpdatePacket{Position: protocol.BlockPos{X: 4, Y: 64, Z: 5}, StateID: 77}),
		bundleDelim(),
	}
	for _, p := range seq {
		if err := d.Dispatch(StatePlay, p); err != nil {
			t.Fatalf("dispatch: %v", err)
		}
	}
	select {
	case ev := <-got:
		if ev.X != 4 || ev.Y != 64 || ev.Z != 5 || ev.StateID != 77 {
			t.Fatalf("unexpected block update inside bundle: %+v", ev)
		}
	default:
		t.Fatal("expected block_update event from inside a bundle")
	}
}

func TestDispatcherBundlePreservesInnerPacketOrder(t *testing.T) {
	bus := NewEventBus()
	d := NewDispatcher(bus)

	var (
		mu    sync.Mutex
		order []int32
	)
	if _, err := bus.On("block_update", func(e Event) {
		if ev, ok := e.(BlockUpdateEvent); ok {
			mu.Lock()
			order = append(order, ev.StateID)
			mu.Unlock()
		}
	}); err != nil {
		t.Fatalf("On: %v", err)
	}

	seq := []*protocol.RawPacket{
		bundleDelim(),
		rawFromPacket(t, &protocol.PlayClientboundBlockUpdatePacket{Position: protocol.BlockPos{X: 0, Y: 1, Z: 0}, StateID: 10}),
		rawFromPacket(t, &protocol.PlayClientboundBlockUpdatePacket{Position: protocol.BlockPos{X: 0, Y: 2, Z: 0}, StateID: 20}),
		rawFromPacket(t, &protocol.PlayClientboundBlockUpdatePacket{Position: protocol.BlockPos{X: 0, Y: 3, Z: 0}, StateID: 30}),
		bundleDelim(),
	}
	for _, p := range seq {
		if err := d.Dispatch(StatePlay, p); err != nil {
			t.Fatalf("dispatch: %v", err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	want := []int32{10, 20, 30}
	if len(order) != len(want) {
		t.Fatalf("got %d events, want %d (%v)", len(order), len(want), order)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("event order = %v, want %v", order, want)
		}
	}
}

func TestDispatcherBundleUnknownInnerPacketDoesNotDesync(t *testing.T) {
	bus := NewEventBus()
	d := NewDispatcher(bus)

	got := make(chan BlockUpdateEvent, 1)
	if _, err := bus.On("block_update", func(e Event) {
		if ev, ok := e.(BlockUpdateEvent); ok {
			got <- ev
		}
	}); err != nil {
		t.Fatalf("On: %v", err)
	}

	// 0x7E is not a handled clientbound play id; it stands in for an
	// unrecognized packet inside the bundle.
	unknown := &protocol.RawPacket{ID: 0x7E, Data: []byte{0xDE, 0xAD, 0xBE, 0xEF}}
	seq := []*protocol.RawPacket{
		bundleDelim(),
		unknown,
		rawFromPacket(t, &protocol.PlayClientboundBlockUpdatePacket{Position: protocol.BlockPos{X: 7, Y: 8, Z: 9}, StateID: 5}),
		bundleDelim(),
	}
	for _, p := range seq {
		if err := d.Dispatch(StatePlay, p); err != nil {
			t.Fatalf("dispatch (id 0x%02X) desynced with error: %v", p.ID, err)
		}
	}
	select {
	case ev := <-got:
		if ev.X != 7 || ev.Y != 8 || ev.Z != 9 || ev.StateID != 5 {
			t.Fatalf("block update after unknown packet corrupted: %+v", ev)
		}
	default:
		t.Fatal("expected block_update after an unknown inner packet (no desync)")
	}
}

// --- Acknowledge Block Change ------------------------------------------------

func TestDispatcherBlockChangeAckEmitsEvent(t *testing.T) {
	bus := NewEventBus()
	d := NewDispatcher(bus)

	got := make(chan BlockChangeAckEvent, 1)
	if _, err := bus.On("block_change_ack", func(e Event) {
		if ev, ok := e.(BlockChangeAckEvent); ok {
			got <- ev
		}
	}); err != nil {
		t.Fatalf("On: %v", err)
	}
	// A block_update listener proves the ack does NOT masquerade as a world change.
	blockUpdates := 0
	if _, err := bus.On("block_update", func(Event) { blockUpdates++ }); err != nil {
		t.Fatalf("On: %v", err)
	}

	raw := rawFromPacket(t, &protocol.PlayClientboundAcknowledgeBlockChangePacket{SequenceID: 4242})
	if raw.ID != consts.PlayClientboundAcknowledgeBlockChange {
		t.Fatalf("expected packet id 0x05, got 0x%02X", raw.ID)
	}
	if err := d.Dispatch(StatePlay, raw); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	select {
	case ev := <-got:
		if ev.SequenceID != 4242 {
			t.Fatalf("ack sequence = %d, want 4242", ev.SequenceID)
		}
	default:
		t.Fatal("expected block_change_ack event")
	}
	if blockUpdates != 0 {
		t.Fatalf("ack emitted %d block_update events, want 0", blockUpdates)
	}
}

func TestAckWriteErrorDoesNotPanic(t *testing.T) {
	bus := NewEventBus()
	d := NewDispatcher(bus)
	// A truncated ack body (no VarInt) must surface as a decode error, never a
	// panic, and must not desync subsequent packets.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("dispatching a truncated ack panicked: %v", r)
		}
	}()
	if err := d.Dispatch(StatePlay, &protocol.RawPacket{ID: consts.PlayClientboundAcknowledgeBlockChange, Data: nil}); err == nil {
		t.Fatal("expected decode error for truncated ack payload")
	}
	// A well-formed packet right after still dispatches cleanly.
	if err := d.Dispatch(StatePlay, rawFromPacket(t, &protocol.PlayClientboundAcknowledgeBlockChangePacket{SequenceID: 1})); err != nil {
		t.Fatalf("dispatch after truncated ack: %v", err)
	}
}

// --- Respawn -----------------------------------------------------------------

func TestDispatcherRespawnEmitsEvent(t *testing.T) {
	bus := NewEventBus()
	d := NewDispatcher(bus)

	got := make(chan RespawnEvent, 1)
	if _, err := bus.On("respawn", func(e Event) {
		if ev, ok := e.(RespawnEvent); ok {
			got <- ev
		}
	}); err != nil {
		t.Fatalf("On: %v", err)
	}

	raw := rawFromPacket(t, &protocol.PlayClientboundRespawnPacket{
		DimensionType: "minecraft:the_nether", DimensionName: "minecraft:the_nether",
		HashedSeed: 7, GameMode: 0, PreviousGameMode: 1,
		IsDebug: false, IsFlat: false, HasDeathLocation: false,
		PortalCooldown: 0, DataKept: 0x02,
	})
	if raw.ID != consts.PlayClientboundRespawn {
		t.Fatalf("expected packet id 0x45, got 0x%02X", raw.ID)
	}
	if err := d.Dispatch(StatePlay, raw); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	select {
	case ev := <-got:
		if ev.DimensionName != "minecraft:the_nether" {
			t.Fatalf("respawn dimension = %q, want minecraft:the_nether", ev.DimensionName)
		}
		if !ev.CopyMetadata {
			t.Fatal("expected CopyMetadata true for DataKept 0x02")
		}
	default:
		t.Fatal("expected respawn event")
	}
}

// --- Regression guards for existing world packets ----------------------------

func TestExistingBlockUpdateStillWorks(t *testing.T) {
	bus := NewEventBus()
	d := NewDispatcher(bus)
	got := make(chan BlockUpdateEvent, 1)
	if _, err := bus.On("block_update", func(e Event) {
		if ev, ok := e.(BlockUpdateEvent); ok {
			got <- ev
		}
	}); err != nil {
		t.Fatalf("On: %v", err)
	}
	raw := rawFromPacket(t, &protocol.PlayClientboundBlockUpdatePacket{Position: protocol.BlockPos{X: -3, Y: 5, Z: 9}, StateID: 12})
	if err := d.Dispatch(StatePlay, raw); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	select {
	case ev := <-got:
		if ev.X != -3 || ev.Y != 5 || ev.Z != 9 || ev.StateID != 12 {
			t.Fatalf("unexpected block update: %+v", ev)
		}
	default:
		t.Fatal("expected block_update event")
	}
}

func TestExistingSectionBlockUpdateStillWorks(t *testing.T) {
	bus := NewEventBus()
	d := NewDispatcher(bus)
	got := make(chan SectionBlocksUpdateEvent, 1)
	if _, err := bus.On("section_blocks_update", func(e Event) {
		if ev, ok := e.(SectionBlocksUpdateEvent); ok {
			got <- ev
		}
	}); err != nil {
		t.Fatalf("On: %v", err)
	}
	raw := rawFromPacket(t, &protocol.PlayClientboundUpdateSectionBlocksPacket{
		SectionPos: protocol.ChunkSectionPos{X: 1, Y: 0, Z: 1},
		Changes:    []protocol.SectionBlockChange{{LocalPos: 0x0000, StateID: 3}},
	})
	if err := d.Dispatch(StatePlay, raw); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	select {
	case ev := <-got:
		if len(ev.Updates) != 1 || ev.Updates[0].StateID != 3 {
			t.Fatalf("unexpected section update: %+v", ev)
		}
	default:
		t.Fatal("expected section_blocks_update event")
	}
}

// --- Benchmarks --------------------------------------------------------------

func BenchmarkDispatchBundleDelimiterNoHandlers(b *testing.B) {
	bus := NewEventBus()
	d := NewDispatcher(bus)
	raw := bundleDelim()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = d.Dispatch(StatePlay, raw)
	}
}

func BenchmarkDispatchBlockChangeAck(b *testing.B) {
	bus := NewEventBus()
	d := NewDispatcher(bus)
	_, _ = bus.On("block_change_ack", func(Event) {})
	var buf []byte
	{
		raw := rawFromPacketB(b, &protocol.PlayClientboundAcknowledgeBlockChangePacket{SequenceID: 9001})
		buf = raw.Data
	}
	raw := &protocol.RawPacket{ID: consts.PlayClientboundAcknowledgeBlockChange, Data: buf}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = d.Dispatch(StatePlay, raw)
	}
}

func rawFromPacketB(b *testing.B, pkt protocol.Packet) *protocol.RawPacket {
	b.Helper()
	var buf bytes.Buffer
	w := protocol.NewWriter(&buf)
	if err := pkt.Marshal(w); err != nil {
		b.Fatalf("marshal: %v", err)
	}
	return &protocol.RawPacket{ID: pkt.PacketID(), Data: buf.Bytes()}
}
