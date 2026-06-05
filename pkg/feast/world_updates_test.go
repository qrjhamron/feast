package feast

import (
	"bytes"
	"testing"

	"github.com/user/feastgo/pkg/protocol"
	"github.com/user/feastgo/pkg/state"
	"github.com/user/feastgo/pkg/world"
)

func TestClientAppliesBlockUpdateEventsToWorld(t *testing.T) {
	c := NewClient(Options{})
	ch := world.NewChunk(0, 0)
	ch.SetBlock(1, 64, 1, world.BlockState{ID: 1, Name: "stone", Solid: true})
	c.World().AddChunk(ch)

	c.bus.Emit(state.BlockUpdateEvent{X: 1, Y: 64, Z: 1, StateID: 0})

	got, err := c.World().GetBlock(1, 64, 1)
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if got.Name != "air" || got.ID != 0 {
		t.Fatalf("block update not applied: %+v", got)
	}
}

func TestClientAppliesSectionBlockUpdateEventsToWorld(t *testing.T) {
	c := NewClient(Options{})
	ch := world.NewChunk(2, -3)
	ch.SetBlock(1, 67, 2, world.BlockState{ID: 1, Name: "stone", Solid: true})
	c.World().AddChunk(ch)

	c.bus.Emit(state.SectionBlocksUpdateEvent{Updates: []state.SectionBlockUpdate{
		{X: 33, Y: 67, Z: -46, StateID: 0},
	}})

	got, err := c.World().GetBlock(33, 67, -46)
	if err != nil {
		t.Fatalf("GetBlock: %v", err)
	}
	if got.Name != "air" || got.ID != 0 {
		t.Fatalf("section block update not applied: %+v", got)
	}
}

func TestServerboundPlayerActionPacketEncodesTypedDigFields(t *testing.T) {
	pkt := &protocol.PlayServerboundPlayerActionPacket{
		Status:   protocol.PlayerActionStartDigging,
		Position: protocol.BlockPos{X: -1, Y: 64, Z: 2},
		Face:     protocol.BlockFaceTop,
		Sequence: 7,
	}

	var decoded protocol.PlayServerboundPlayerActionPacket
	mustRoundTripPacket(t, pkt, &decoded)

	if decoded.Status != protocol.PlayerActionStartDigging ||
		decoded.Position.X != -1 ||
		decoded.Position.Y != 64 ||
		decoded.Position.Z != 2 ||
		decoded.Face != protocol.BlockFaceTop ||
		decoded.Sequence != 7 {
		t.Fatalf("decoded packet mismatch: %+v", decoded)
	}
}

func mustRoundTripPacket(t *testing.T, in protocol.Packet, out protocol.Packet) {
	t.Helper()
	var body bytes.Buffer
	if err := in.Marshal(protocol.NewWriter(&body)); err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := out.Unmarshal(protocol.NewReader(bytes.NewReader(body.Bytes()))); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}
