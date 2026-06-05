package state

import (
	"bytes"
	"testing"

	"github.com/user/feastgo/pkg/protocol"
	"github.com/user/feastgo/pkg/protocol/consts"
)

func rawFromPacket(t *testing.T, pkt protocol.Packet) *protocol.RawPacket {
	t.Helper()
	var buf bytes.Buffer
	w := protocol.NewWriter(&buf)
	if err := pkt.Marshal(w); err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return &protocol.RawPacket{ID: pkt.PacketID(), Data: buf.Bytes()}
}

func TestDispatcher_BlockUpdate_EmitsEvent(t *testing.T) {
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

	raw := rawFromPacket(t, &protocol.PlayClientboundBlockUpdatePacket{
		Position: protocol.BlockPos{X: 1, Y: 64, Z: -2},
		StateID:  123,
	})
	if raw.ID != consts.PlayClientboundBlockUpdate {
		t.Fatalf("expected packet id 0x09, got 0x%02X", raw.ID)
	}

	if err := d.Dispatch(StatePlay, raw); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	select {
	case ev := <-got:
		if ev.X != 1 || ev.Y != 64 || ev.Z != -2 || ev.StateID != 123 {
			t.Fatalf("unexpected event: %+v", ev)
		}
	default:
		t.Fatalf("expected block_update event")
	}
}

func TestDispatcher_UpdateSectionBlocks_EmitsBatchEvent(t *testing.T) {
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
		SectionPos: protocol.ChunkSectionPos{X: 2, Y: 4, Z: -3},
		Changes: []protocol.SectionBlockChange{
			{LocalPos: 0x0123, StateID: 9},
		},
	})
	if raw.ID != consts.PlayClientboundUpdateSectionBlocks {
		t.Fatalf("expected packet id 0x47, got 0x%02X", raw.ID)
	}

	if err := d.Dispatch(StatePlay, raw); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	select {
	case ev := <-got:
		if len(ev.Updates) != 1 {
			t.Fatalf("expected 1 update, got %d", len(ev.Updates))
		}
		u := ev.Updates[0]
		// base = (2*16,4*16,-3*16) = (32,64,-48)
		// localPos 0x0123 -> x=1, z=2, y=3
		if u.X != 33 || u.Y != 67 || u.Z != -46 || u.StateID != 9 {
			t.Fatalf("unexpected update: %+v", u)
		}
	default:
		t.Fatalf("expected section_blocks_update event")
	}
}
