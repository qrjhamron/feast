package feast

import (
	"bytes"
	"io"
	"log"
	"net"
	"testing"
	"time"

	feastconn "github.com/user/feastgo/pkg/conn"
	"github.com/user/feastgo/pkg/protocol"
	"github.com/user/feastgo/pkg/protocol/consts"
	"github.com/user/feastgo/pkg/state"
	"github.com/user/feastgo/pkg/world"
)

func TestNavigateHandlersCountUnchangedAfterStop(t *testing.T) {
	prevLogOut := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(prevLogOut)

	c := NewClient(Options{})

	baseHandlers := c.Events().HandlerCount()

	ch := world.NewChunk(0, 0)
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			ch.SetBlock(x, 63, z, world.BlockState{Name: "stone", Solid: true})
			ch.SetBlock(x, 64, z, world.AirBlockState)
			ch.SetBlock(x, 65, z, world.AirBlockState)
		}
	}
	c.World().AddChunk(ch)

	c.stateMu.Lock()
	c.player.X = 0
	c.player.Y = 64
	c.player.Z = 0
	c.positionSynced = true
	c.stateMu.Unlock()

	if err := c.NavigateTo(10, 64, 0); err != nil {
		t.Fatalf("navigate: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	c.StopNavigation()
	time.Sleep(20 * time.Millisecond)

	gotHandlers := c.Events().HandlerCount()
	if gotHandlers != baseHandlers {
		t.Fatalf("handler leak detected: handlers=%d->%d", baseHandlers, gotHandlers)
	}
}

func TestDisconnectThenReconnectWorldEmpty(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	c := NewClient(Options{Host: "localhost", Port: "25565", Username: "FeastBot"})
	c.dialFunc = func(_, _ string) (net.Conn, error) { return clientConn, nil }

	errCh := make(chan error, 1)
	go func() {
		s := feastconn.New(serverConn)
		if _, err := s.ReadPacket(); err != nil {
			errCh <- err
			return
		}
		if _, err := s.ReadPacket(); err != nil {
			errCh <- err
			return
		}
		if err := s.WritePacket(&protocol.LoginClientboundLoginSuccessPacket{UUID: [16]byte{1}, Username: "FeastBot"}); err != nil {
			errCh <- err
			return
		}
		if _, err := s.ReadPacket(); err != nil { // LoginAck
			errCh <- err
			return
		}
		if _, err := s.ReadPacket(); err != nil { // ClientInfo
			errCh <- err
			return
		}
		if err := s.WritePacket(&protocol.ConfigClientboundFinishPacket{}); err != nil {
			errCh <- err
			return
		}
		if _, err := s.ReadPacket(); err != nil { // AckFinish
			errCh <- err
			return
		}
		errCh <- nil
	}()

	if err := c.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("server flow: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting login/config")
	}

	ch := world.NewChunk(0, 0)
	ch.SetBlock(0, 64, 0, world.BlockState{Name: "stone", Solid: true})
	c.World().AddChunk(ch)
	if _, err := c.World().GetBlock(0, 64, 0); err != nil {
		t.Fatalf("precondition chunk missing: %v", err)
	}

	if err := c.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	if _, err := c.World().GetBlock(0, 64, 0); err == nil {
		t.Fatalf("expected world to be empty after disconnect")
	}
}

func TestMalformedPacketDoesNotDisconnectClient(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	c := NewClient(Options{Host: "localhost", Port: "25565", Username: "FeastBot"})
	c.dialFunc = func(_, _ string) (net.Conn, error) { return clientConn, nil }

	disconnectSeen := make(chan struct{}, 1)
	if _, err := c.On("disconnect", func(state.Event) {
		select {
		case disconnectSeen <- struct{}{}:
		default:
		}
	}); err != nil {
		t.Fatalf("On: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		s := feastconn.New(serverConn)
		if _, err := s.ReadPacket(); err != nil {
			errCh <- err
			return
		}
		if _, err := s.ReadPacket(); err != nil {
			errCh <- err
			return
		}
		if err := s.WritePacket(&protocol.LoginClientboundLoginSuccessPacket{UUID: [16]byte{1}, Username: "FeastBot"}); err != nil {
			errCh <- err
			return
		}
		if _, err := s.ReadPacket(); err != nil { // LoginAck
			errCh <- err
			return
		}
		if _, err := s.ReadPacket(); err != nil { // ClientInfo
			errCh <- err
			return
		}
		if err := s.WritePacket(&protocol.ConfigClientboundFinishPacket{}); err != nil {
			errCh <- err
			return
		}
		if _, err := s.ReadPacket(); err != nil { // AckFinish
			errCh <- err
			return
		}

		// Send malformed keepalive payload (missing 8-byte long).
		if err := s.WritePacket(&playClientboundMalformedKeepAlivePacket{}); err != nil {
			errCh <- err
			return
		}
		time.Sleep(100 * time.Millisecond)

		// Follow with valid keepalive; if client remained connected it will reply.
		if err := s.WritePacket(&protocol.PlayClientboundKeepAlivePacket{KeepAliveID: 999}); err != nil {
			errCh <- err
			return
		}

		deadline := time.After(2 * time.Second)
		for {
			select {
			case <-deadline:
				errCh <- errUnexpectedID(-1, consts.PlayServerboundServerboundKeepAlive)
				return
			default:
			}

			raw, err := s.ReadPacket()
			if err != nil {
				errCh <- err
				return
			}
			if raw.ID != consts.PlayServerboundServerboundKeepAlive {
				continue
			}
			r := protocol.NewReader(bytes.NewReader(raw.Data))
			id, err := r.ReadLong()
			if err != nil {
				errCh <- err
				return
			}
			if id != 999 {
				errCh <- errUnexpectedID(int32(id), 999)
				return
			}
			errCh <- nil
			return
		}
	}()

	if err := c.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Disconnect()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("server flow: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting malformed recovery flow")
	}

	select {
	case <-disconnectSeen:
		t.Fatal("unexpected disconnect after malformed packet")
	default:
	}
}

type playClientboundStartConfigurationTestPacket struct{}

func (p *playClientboundStartConfigurationTestPacket) PacketID() int32 {
	return consts.PlayClientboundStartConfiguration
}

func (p *playClientboundStartConfigurationTestPacket) Marshal(*protocol.Writer) error { return nil }
func (p *playClientboundStartConfigurationTestPacket) Unmarshal(*protocol.Reader) error {
	return nil
}

type playClientboundMalformedKeepAlivePacket struct{}

func (p *playClientboundMalformedKeepAlivePacket) PacketID() int32 {
	return consts.PlayClientboundClientboundKeepAlive
}

func (p *playClientboundMalformedKeepAlivePacket) Marshal(w *protocol.Writer) error {
	return w.WriteByte(0x01)
}

func (p *playClientboundMalformedKeepAlivePacket) Unmarshal(*protocol.Reader) error {
	return nil
}
