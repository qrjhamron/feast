package state

import (
	"bytes"
	"testing"

	"github.com/user/feastgo/pkg/protocol"
	"github.com/user/feastgo/pkg/protocol/consts"
)

func mustRawPacket(t *testing.T, packetID int32, pkt protocol.Packet) *protocol.RawPacket {
	t.Helper()
	var body bytes.Buffer
	if err := pkt.Marshal(protocol.NewWriter(&body)); err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return &protocol.RawPacket{ID: packetID, Data: body.Bytes()}
}

func TestDispatcherUpdateEntityPositionAndRotationEmitsMoveAndRotate(t *testing.T) {
	bus := NewEventBus()
	dispatcher := NewDispatcher(bus)

	var gotMove EntityMoveDeltaEvent
	var gotRotate EntityRotateEvent
	moveSeen := false
	rotateSeen := false

	if _, err := bus.On("entity_move_delta", func(event Event) {
		moveSeen = true
		gotMove = event.(EntityMoveDeltaEvent)
	}); err != nil {
		t.Fatalf("on move: %v", err)
	}
	if _, err := bus.On("entity_rotate", func(event Event) {
		rotateSeen = true
		gotRotate = event.(EntityRotateEvent)
	}); err != nil {
		t.Fatalf("on rotate: %v", err)
	}

	raw := mustRawPacket(t, consts.PlayClientboundUpdateEntityPositionAndRotation, &protocol.PlayClientboundUpdateEntityPositionAndRotationPacket{
		EntityID: 1, DX: 64, DY: -32, DZ: 16, Yaw: 64, Pitch: 32, OnGround: true,
	})
	if err := dispatcher.Dispatch(StatePlay, raw); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !moveSeen || !rotateSeen {
		t.Fatalf("expected move and rotate events, move=%v rotate=%v", moveSeen, rotateSeen)
	}
	if gotMove.EntityID != 1 || gotMove.DX != 64 || gotMove.DY != -32 || gotMove.DZ != 16 {
		t.Fatalf("unexpected move payload: %+v", gotMove)
	}
	if gotRotate.EntityID != 1 || !gotRotate.OnGround {
		t.Fatalf("unexpected rotate payload: %+v", gotRotate)
	}
}

func TestDispatcherUpdateTimeEmitsTimeUpdateEvent(t *testing.T) {
	bus := NewEventBus()
	dispatcher := NewDispatcher(bus)
	got := TimeUpdateEvent{}
	seen := false
	if _, err := bus.On("time_update", func(event Event) {
		seen = true
		got = event.(TimeUpdateEvent)
	}); err != nil {
		t.Fatalf("on time: %v", err)
	}

	raw := mustRawPacket(t, consts.PlayClientboundUpdateTime, &protocol.PlayClientboundUpdateTimePacket{
		WorldAge:  1234,
		TimeOfDay: 5678,
	})
	if err := dispatcher.Dispatch(StatePlay, raw); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !seen {
		t.Fatal("expected time_update event")
	}
	if got.WorldAge != 1234 || got.TimeOfDay != 5678 {
		t.Fatalf("unexpected payload: %+v", got)
	}
}

func TestDispatcherPlayerInfoUpdateEmitsTypedEvent(t *testing.T) {
	bus := NewEventBus()
	dispatcher := NewDispatcher(bus)
	got := PlayerInfoUpdateEvent{}
	seen := false
	if _, err := bus.On("player_info_update", func(event Event) {
		seen = true
		got = event.(PlayerInfoUpdateEvent)
	}); err != nil {
		t.Fatalf("on player_info_update: %v", err)
	}

	raw := mustRawPacket(t, consts.PlayClientboundPlayerInfoUpdate, &protocol.PlayClientboundPlayerInfoUpdatePacket{
		Actions: protocol.PlayerInfoActionUpdateLatency,
		Players: []protocol.PlayClientboundPlayerInfoUpdatePlayer{{
			UUID: [16]byte{9},
			Ping: 99,
		}},
	})
	if err := dispatcher.Dispatch(StatePlay, raw); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !seen {
		t.Fatal("expected player_info_update event")
	}
	if got.Actions != protocol.PlayerInfoActionUpdateLatency || len(got.Players) != 1 || got.Players[0].Ping != 99 {
		t.Fatalf("unexpected payload: %+v", got)
	}
}
