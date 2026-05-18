package feast

import (
	"bytes"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	feastconn "github.com/user/feastgo/pkg/conn"
	"github.com/user/feastgo/pkg/protocol"
	"github.com/user/feastgo/pkg/protocol/consts"
	"github.com/user/feastgo/pkg/state"
)

func TestConnectMinimalLoginSequence(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	c := NewClient(Options{Host: "localhost", Port: "25565", Username: "FeastBot"})
	c.dialFunc = func(_, _ string) (net.Conn, error) { return clientConn, nil }

	loginSeen := make(chan state.LoginEvent, 1)
	c.On("login", func(e state.Event) {
		if le, ok := e.(state.LoginEvent); ok {
			loginSeen <- le
		}
	})

	errCh := make(chan error, 1)
	go func() {
		s := feastconn.New(serverConn)

		// Expect handshake then login start.
		if _, err := s.ReadPacket(); err != nil {
			errCh <- err
			return
		}
		if raw, err := s.ReadPacket(); err != nil {
			errCh <- err
			return
		} else if raw.ID != consts.LoginServerboundLoginStart {
			errCh <- errUnexpectedID(raw.ID, consts.LoginServerboundLoginStart)
			return
		}

		// Send Login Success.
		if err := s.WritePacket(&protocol.LoginClientboundLoginSuccessPacket{UUID: [16]byte{1, 2, 3}, Username: "FeastBot"}); err != nil {
			errCh <- err
			return
		}

		// Expect Login Acknowledged.
		if raw, err := s.ReadPacket(); err != nil {
			errCh <- err
			return
		} else if raw.ID != consts.LoginServerboundLoginAcknowledged {
			errCh <- errUnexpectedID(raw.ID, consts.LoginServerboundLoginAcknowledged)
			return
		}

		// Send Finish Configuration.
		if err := s.WritePacket(&protocol.ConfigClientboundFinishPacket{}); err != nil {
			errCh <- err
			return
		}

		// Expect Ack Finish.
		if raw, err := s.ReadPacket(); err != nil {
			errCh <- err
			return
		} else if raw.ID != consts.ConfigurationServerboundAcknowledgeFinishConfiguration {
			errCh <- errUnexpectedID(raw.ID, consts.ConfigurationServerboundAcknowledgeFinishConfiguration)
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
		t.Fatal("timeout waiting for mock server flow")
	}

	select {
	case ev := <-loginSeen:
		if ev.Username != "FeastBot" {
			t.Fatalf("unexpected login username: %s", ev.Username)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("login event not emitted")
	}

	_ = c.Disconnect()
}

func TestDisconnectIsIdempotentBeforeConnect(t *testing.T) {
	c := NewClient(Options{})
	if err := c.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestStatsDefaultsBeforeConnect(t *testing.T) {
	c := NewClient(Options{})
	stats := c.Stats()
	if stats.CurrentState != state.StateHandshaking {
		t.Fatalf("unexpected state: %v", stats.CurrentState)
	}
	if stats.PacketsReceived != 0 || stats.PacketsSent != 0 || stats.ConnectedFor != 0 {
		t.Fatalf("unexpected initial stats: %+v", stats)
	}
}

type idError struct{ got, want int32 }

func (e idError) Error() string { return "unexpected packet id" }

func errUnexpectedID(got, want int32) error { return idError{got: got, want: want} }

func TestHeartbeatPacketEncoding(t *testing.T) {
	pkt := heartbeatPacket()
	var b bytes.Buffer
	if err := pkt.Marshal(protocol.NewWriter(&b)); err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(b.Bytes()) == 0 {
		t.Fatal("empty heartbeat payload")
	}
}

func TestClientPlayLoopRepliesToKeepAliveAndTracksState(t *testing.T) {
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
		if _, err := s.ReadPacket(); err != nil {
			errCh <- err
			return
		}
		if err := s.WritePacket(&protocol.ConfigClientboundFinishPacket{}); err != nil {
			errCh <- err
			return
		}
		if _, err := s.ReadPacket(); err != nil {
			errCh <- err
			return
		}

		if err := s.WritePacket(&protocol.PlayClientboundLoginPacket{EntityID: 99}); err != nil {
			errCh <- err
			return
		}
		if err := s.WritePacket(&protocol.PlayClientboundSynchronizePlayerPositionPacket{X: 4, Y: 65, Z: -8, TeleportID: 11}); err != nil {
			errCh <- err
			return
		}
		if err := s.WritePacket(&protocol.PlayClientboundSetHealthPacket{Health: 17, Food: 19, Saturation: 3}); err != nil {
			errCh <- err
			return
		}
		if err := s.WritePacket(&protocol.PlayClientboundKeepAlivePacket{KeepAliveID: 456}); err != nil {
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
			if raw.ID == consts.PlayServerboundServerboundKeepAlive {
				r := protocol.NewReader(bytes.NewReader(raw.Data))
				id, err := r.ReadLong()
				if err != nil {
					errCh <- err
					return
				}
				if id != 456 {
					errCh <- errUnexpectedID(int32(id), 456)
					return
				}
				errCh <- nil
				return
			}
		}
	}()

	if err := c.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Disconnect()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("mock server: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for keepalive response")
	}

	st := c.PlayerState()
	if st.EntityID != 99 || st.X != 4 || st.Y != 65 || st.Z != -8 || st.Health != 17 || st.Food != 19 {
		t.Fatalf("unexpected player state: %+v", st)
	}
	if c.CurrentState() != state.StatePlay {
		t.Fatalf("unexpected current state: %v", c.CurrentState())
	}
	if c.Events() == nil {
		t.Fatal("nil event bus")
	}
	stats := c.Stats()
	if stats.PacketsReceived == 0 || stats.PacketsSent == 0 {
		t.Fatalf("expected packet counters to be updated: %+v", stats)
	}
	if stats.LastKeepAliveAt.IsZero() {
		t.Fatalf("expected keepalive timestamp: %+v", stats)
	}
	if stats.LastPositionSyncAt.IsZero() {
		t.Fatalf("expected position timestamp: %+v", stats)
	}
}

func TestReadLoopEOFEmitsErrorAndDisconnect(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()

	c := NewClient(Options{Host: "localhost", Port: "25565", Username: "FeastBot"})
	c.dialFunc = func(_, _ string) (net.Conn, error) { return clientConn, nil }

	errSeen := make(chan state.ErrorEvent, 1)
	disconnectSeen := make(chan state.DisconnectEvent, 1)
	c.On("error", func(e state.Event) {
		if ev, ok := e.(state.ErrorEvent); ok {
			errSeen <- ev
		}
	})
	c.On("disconnect", func(e state.Event) {
		if ev, ok := e.(state.DisconnectEvent); ok {
			disconnectSeen <- ev
		}
	})

	go func() {
		s := feastconn.New(serverConn)
		_, _ = s.ReadPacket()
		_, _ = s.ReadPacket()
		_ = s.WritePacket(&protocol.LoginClientboundLoginSuccessPacket{UUID: [16]byte{1}, Username: "FeastBot"})
		_, _ = s.ReadPacket()
		_ = s.WritePacket(&protocol.ConfigClientboundFinishPacket{})
		_, _ = s.ReadPacket()
		_ = serverConn.Close()
	}()

	if err := c.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Disconnect()

	select {
	case ev := <-errSeen:
		if ev.Op != "read_loop" || ev.Error == nil {
			t.Fatalf("unexpected error event: %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for error event")
	}
	select {
	case ev := <-disconnectSeen:
		if ev.Clean {
			t.Fatalf("expected unclean disconnect: %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for disconnect event")
	}
}

func TestConcurrentSendChatAndCloseDoesNotRaceOrPanic(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()

	c := NewClient(Options{})
	c.conn = feastconn.New(clientConn)

	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 512)
		for {
			if _, err := serverConn.Read(buf); err != nil {
				return
			}
		}
	}()

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = c.SendChat("hello")
		}()
	}
	_ = c.Close()
	wg.Wait()
	_ = serverConn.Close()
	<-done
}

func TestClassifyReadError(t *testing.T) {
	if classifyReadError(io.EOF) == "" {
		t.Fatal("expected EOF classification")
	}
}
