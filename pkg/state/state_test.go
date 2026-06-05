package state

import (
	"bytes"
	"testing"

	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/protocol/consts"
)

func TestFSMIllegalTransition(t *testing.T) {
	fsm := NewFSM()
	if err := fsm.Transition(StatePlay); err == nil {
		t.Fatalf("expected illegal transition error")
	}
	if fsm.Current() != StateHandshaking {
		t.Fatalf("state changed on illegal transition")
	}
}

func TestEventBusHandlers(t *testing.T) {
	bus := NewEventBus()
	called := 0
	bus.On("chat", func(e Event) {
		ce, ok := e.(ChatEvent)
		if !ok {
			t.Fatalf("wrong event type")
		}
		if ce.Message != "hello" {
			t.Fatalf("unexpected message: %s", ce.Message)
		}
		called++
	})

	bus.Emit(ChatEvent{Sender: "system", Message: "hello"})
	if called != 1 {
		t.Fatalf("handler not called")
	}
}

func TestDispatcherLoginSuccessEmitsLoginEvent(t *testing.T) {
	bus := NewEventBus()
	d := NewDispatcher(bus)

	var got LoginEvent
	seen := false
	bus.On("login", func(e Event) {
		got = e.(LoginEvent)
		seen = true
	})

	pkt := &protocol.LoginClientboundLoginSuccessPacket{Username: "FeastBot", UUID: [16]byte{1, 2, 3, 4}}
	var body bytes.Buffer
	if err := pkt.Marshal(protocol.NewWriter(&body)); err != nil {
		t.Fatalf("marshal: %v", err)
	}

	raw := &protocol.RawPacket{ID: consts.LoginClientboundLoginSuccess, Data: body.Bytes()}
	if err := d.Dispatch(StateLogin, raw); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !seen {
		t.Fatalf("login event not emitted")
	}
	if got.Username != "FeastBot" {
		t.Fatalf("unexpected username: %s", got.Username)
	}
}

func TestDispatcherPlaySyncPositionEmitsSpawnEvent(t *testing.T) {
	bus := NewEventBus()
	d := NewDispatcher(bus)

	var got SpawnEvent
	seen := false
	bus.On("spawn", func(e Event) {
		got = e.(SpawnEvent)
		seen = true
	})

	var body bytes.Buffer
	w := protocol.NewWriter(&body)
	_ = w.WriteDouble(10.5)
	_ = w.WriteDouble(64.0)
	_ = w.WriteDouble(-3.25)
	_ = w.WriteFloat(0)
	_ = w.WriteFloat(0)
	_ = w.WriteByte(0)
	_ = w.WriteVarInt(1)

	raw := &protocol.RawPacket{ID: consts.PlayClientboundSynchronizePlayerPosition, Data: body.Bytes()}
	if err := d.Dispatch(StatePlay, raw); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !seen {
		t.Fatalf("spawn event not emitted")
	}
	if got.X != 10.5 || got.Y != 64.0 || got.Z != -3.25 {
		t.Fatalf("unexpected position: %+v", got)
	}
}

func TestDispatcherPlayLoginEmitsPlayLoginEvent(t *testing.T) {
	bus := NewEventBus()
	d := NewDispatcher(bus)

	var got PlayLoginEvent
	seen := false
	bus.On("play_login", func(e Event) {
		got = e.(PlayLoginEvent)
		seen = true
	})

	pkt := &protocol.PlayClientboundLoginPacket{EntityID: 42, TailData: []byte{1, 2, 3}}
	var body bytes.Buffer
	if err := pkt.Marshal(protocol.NewWriter(&body)); err != nil {
		t.Fatalf("marshal: %v", err)
	}

	raw := &protocol.RawPacket{ID: consts.PlayClientboundLogin, Data: body.Bytes()}
	if err := d.Dispatch(StatePlay, raw); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !seen {
		t.Fatalf("play login event not emitted")
	}
	if got.EntityID != 42 {
		t.Fatalf("unexpected entity id: %d", got.EntityID)
	}
}

func TestDispatcherPlaySyncPositionEmitsPositionEvent(t *testing.T) {
	bus := NewEventBus()
	d := NewDispatcher(bus)

	var got PositionEvent
	seen := false
	bus.On("position", func(e Event) {
		got = e.(PositionEvent)
		seen = true
	})

	pkt := &protocol.PlayClientboundSynchronizePlayerPositionPacket{
		X: 1.5, Y: 70, Z: -4.25, Yaw: 12, Pitch: 34, TeleportID: 9,
	}
	var body bytes.Buffer
	if err := pkt.Marshal(protocol.NewWriter(&body)); err != nil {
		t.Fatalf("marshal: %v", err)
	}

	raw := &protocol.RawPacket{ID: consts.PlayClientboundSynchronizePlayerPosition, Data: body.Bytes()}
	if err := d.Dispatch(StatePlay, raw); err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !seen {
		t.Fatalf("position event not emitted")
	}
	if got.X != 1.5 || got.Y != 70 || got.Z != -4.25 || got.TeleportID != 9 {
		t.Fatalf("unexpected position event: %+v", got)
	}
}

func TestDispatcherPlayHealthAndKeepAliveEvents(t *testing.T) {
	bus := NewEventBus()
	d := NewDispatcher(bus)

	var health HealthEvent
	var keepAlive KeepAliveEvent
	bus.On("health", func(e Event) { health = e.(HealthEvent) })
	bus.On("keep_alive", func(e Event) { keepAlive = e.(KeepAliveEvent) })

	var healthBody bytes.Buffer
	if err := (&protocol.PlayClientboundSetHealthPacket{Health: 19.5, Food: 18, Saturation: 4}).Marshal(protocol.NewWriter(&healthBody)); err != nil {
		t.Fatalf("marshal health: %v", err)
	}
	if err := d.Dispatch(StatePlay, &protocol.RawPacket{ID: consts.PlayClientboundSetHealth, Data: healthBody.Bytes()}); err != nil {
		t.Fatalf("dispatch health: %v", err)
	}
	if health.Health != 19.5 || health.Food != 18 || health.Saturation != 4 {
		t.Fatalf("unexpected health event: %+v", health)
	}

	var keepAliveBody bytes.Buffer
	if err := (&protocol.PlayClientboundKeepAlivePacket{KeepAliveID: 123}).Marshal(protocol.NewWriter(&keepAliveBody)); err != nil {
		t.Fatalf("marshal keepalive: %v", err)
	}
	if err := d.Dispatch(StatePlay, &protocol.RawPacket{ID: consts.PlayClientboundClientboundKeepAlive, Data: keepAliveBody.Bytes()}); err != nil {
		t.Fatalf("dispatch keepalive: %v", err)
	}
	if keepAlive.ID != 123 {
		t.Fatalf("unexpected keepalive event: %+v", keepAlive)
	}
}
