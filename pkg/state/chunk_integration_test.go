package state

import (
	"bytes"
	"testing"

	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/protocol/consts"
)

func TestChunkDataPacketUnmarshalWithHardcodedPayload(t *testing.T) {
	// Hardcoded payload: ChunkX=2, ChunkZ=-5, followed by opaque tail bytes.
	rawBody := []byte{
		0x00, 0x00, 0x00, 0x02,
		0xFF, 0xFF, 0xFF, 0xFB,
		0x09, 0x08, 0x07, 0x06,
	}

	pkt := &protocol.PlayClientboundChunkDataAndUpdateLightPacket{}
	if err := pkt.Unmarshal(protocol.NewReader(bytes.NewReader(rawBody))); err != nil {
		t.Fatalf("unmarshal chunk packet: %v", err)
	}

	if pkt.ChunkX != 2 || pkt.ChunkZ != -5 {
		t.Fatalf("unexpected chunk coords: x=%d z=%d", pkt.ChunkX, pkt.ChunkZ)
	}

	wantTail := []byte{0x09, 0x08, 0x07, 0x06}
	if !bytes.Equal(pkt.TailData, wantTail) {
		t.Fatalf("unexpected tail data: got=%x want=%x", pkt.TailData, wantTail)
	}
}

func TestDispatcherChunkDataWiringEmitsPacketEvent(t *testing.T) {
	bus := NewEventBus()
	d := NewDispatcher(bus)

	var got PacketEvent
	seen := false
	if _, err := bus.On("packet", func(e Event) {
		got = e.(PacketEvent)
		seen = true
	}); err != nil {
		t.Fatalf("On: %v", err)
	}

	body := []byte{
		0x00, 0x00, 0x00, 0x02,
		0x00, 0x00, 0x00, 0x03,
		0xAA, 0xBB,
	}
	raw := &protocol.RawPacket{
		ID:   consts.PlayClientboundChunkDataAndUpdateLight,
		Data: body,
	}

	if err := d.Dispatch(StatePlay, raw); err != nil {
		t.Fatalf("dispatch chunk data: %v", err)
	}
	if !seen {
		t.Fatalf("packet event not emitted")
	}
	if got.ID != consts.PlayClientboundChunkDataAndUpdateLight {
		t.Fatalf("unexpected packet id: got=%d", got.ID)
	}
	if !bytes.Equal(got.Data, body) {
		t.Fatalf("unexpected packet data: got=%x want=%x", got.Data, body)
	}
}

func TestDispatcherUnloadChunkWiringEmitsPacketEvent(t *testing.T) {
	bus := NewEventBus()
	d := NewDispatcher(bus)

	var got PacketEvent
	seen := false
	if _, err := bus.On("packet", func(e Event) {
		got = e.(PacketEvent)
		seen = true
	}); err != nil {
		t.Fatalf("On: %v", err)
	}

	body := []byte{
		0x00, 0x00, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x02,
	}
	raw := &protocol.RawPacket{
		ID:   consts.PlayClientboundUnloadChunk,
		Data: body,
	}

	if err := d.Dispatch(StatePlay, raw); err != nil {
		t.Fatalf("dispatch unload chunk: %v", err)
	}
	if !seen {
		t.Fatalf("packet event not emitted")
	}
	if got.ID != consts.PlayClientboundUnloadChunk {
		t.Fatalf("unexpected packet id: got=%d", got.ID)
	}
	if !bytes.Equal(got.Data, body) {
		t.Fatalf("unexpected packet data: got=%x want=%x", got.Data, body)
	}
}
