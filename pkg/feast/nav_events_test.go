package feast

import (
	"io"
	"net"
	"testing"
	"time"

	feastconn "github.com/qrjhamron/feast/pkg/conn"
	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/state"
)

func TestNavEventingClient_EmitsNavStep(t *testing.T) {
	c := NewClient(Options{})

	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	c.conn = feastconn.New(a)
	go func() { _, _ = io.Copy(io.Discard, b) }()

	stepCh := make(chan state.NavStepEvent, 8)
	if _, err := c.bus.On("nav_step", func(e state.Event) {
		if ev, ok := e.(state.NavStepEvent); ok {
			stepCh <- ev
		}
	}); err != nil {
		t.Fatalf("On: %v", err)
	}

	w := newNavEventingClient(c)
	err := w.WritePacket(&protocol.PlayServerboundSetPlayerPositionAndRotationPacket{
		X: 1, Y: 2, Z: 3, OnGround: true,
	})
	if err != nil {
		t.Fatalf("WritePacket: %v", err)
	}

	select {
	case ev := <-stepCh:
		if ev.X != 1 || ev.Y != 2 || ev.Z != 3 {
			t.Fatalf("nav_step coords = (%.1f,%.1f,%.1f) want (1,2,3)", ev.X, ev.Y, ev.Z)
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatalf("expected nav_step event")
	}
}
