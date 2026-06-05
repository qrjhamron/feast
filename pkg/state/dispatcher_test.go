package state

import (
	"bytes"
	"strings"
	"testing"

	"github.com/user/feastgo/pkg/protocol"
	"github.com/user/feastgo/pkg/protocol/consts"
)

func marshalRaw(id int32, p protocol.Packet) *protocol.RawPacket {
	var b bytes.Buffer
	w := protocol.NewWriter(&b)
	_ = p.Marshal(w)
	return &protocol.RawPacket{ID: id, Data: b.Bytes()}
}

func TestDispatcher_LoginEncryptionRequestRefusal(t *testing.T) {
	bus := NewEventBus()
	d := NewDispatcher(bus)

	raw := marshalRaw(consts.LoginClientboundEncryptionRequest, &protocol.LoginClientboundEncryptionRequestPacket{})
	err := d.Dispatch(StateLogin, raw)
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("expected explicit unsupported error for EncryptionRequest, got %v", err)
	}
}

func TestDispatcher_EntityEvents(t *testing.T) {
	bus := NewEventBus()
	d := NewDispatcher(bus)

	var spawnSeen *EntitySpawnEvent
	bus.On("entity_spawn", func(e Event) {
		ev := e.(EntitySpawnEvent)
		spawnSeen = &ev
	})

	var moveSeen *EntityMoveDeltaEvent
	bus.On("entity_move_delta", func(e Event) {
		ev := e.(EntityMoveDeltaEvent)
		moveSeen = &ev
	})

	var teleportSeen *EntityTeleportEvent
	bus.On("entity_teleport", func(e Event) {
		ev := e.(EntityTeleportEvent)
		teleportSeen = &ev
	})

	var velSeen *EntityVelocityEvent
	bus.On("entity_velocity", func(e Event) {
		ev := e.(EntityVelocityEvent)
		velSeen = &ev
	})

	// Spawn
	spawnRaw := marshalRaw(consts.PlayClientboundSpawnEntity, &protocol.PlayClientboundSpawnEntityPacket{EntityID: 5, Type: 1, X: 1, Y: 2, Z: 3})
	_ = d.Dispatch(StatePlay, spawnRaw)
	if spawnSeen == nil || spawnSeen.EntityID != 5 {
		t.Fatalf("spawn not emitted correctly")
	}

	// Move
	moveRaw := marshalRaw(consts.PlayClientboundUpdateEntityPosition, &protocol.PlayClientboundUpdateEntityPositionPacket{EntityID: 5, DX: 10, DY: 20, DZ: 30})
	_ = d.Dispatch(StatePlay, moveRaw)
	if moveSeen == nil || moveSeen.EntityID != 5 || moveSeen.DX != 10 {
		t.Fatalf("move not emitted correctly")
	}

	// Teleport
	teleportRaw := marshalRaw(consts.PlayClientboundTeleportEntity, &protocol.PlayClientboundTeleportEntityPacket{EntityID: 5, X: 100, Y: 200, Z: 300})
	_ = d.Dispatch(StatePlay, teleportRaw)
	if teleportSeen == nil || teleportSeen.EntityID != 5 || teleportSeen.X != 100 {
		t.Fatalf("teleport not emitted correctly")
	}

	// Velocity
	velRaw := marshalRaw(consts.PlayClientboundSetEntityVelocity, &protocol.PlayClientboundSetEntityVelocityPacket{EntityID: 5, VelocityX: 400})
	_ = d.Dispatch(StatePlay, velRaw)
	if velSeen == nil || velSeen.EntityID != 5 || velSeen.VelocityX != 400 {
		t.Fatalf("velocity not emitted correctly")
	}
}

func TestDispatcher_UnknownPacketNoCrash(t *testing.T) {
	bus := NewEventBus()
	d := NewDispatcher(bus)

	err := d.Dispatch(StatePlay, &protocol.RawPacket{ID: 0x999, Data: []byte{1, 2, 3}})
	if err != nil {
		t.Fatalf("unknown packet returned error: %v", err)
	}
}
