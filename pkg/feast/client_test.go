package feast

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	feastconn "github.com/qrjhamron/feast/pkg/conn"
	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/protocol/consts"
	"github.com/qrjhamron/feast/pkg/state"
	"github.com/qrjhamron/feast/pkg/world"
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

		// Expect Client Information in Configuration state.
		if raw, err := s.ReadPacket(); err != nil {
			errCh <- err
			return
		} else if raw.ID != consts.ConfigurationServerboundClientInformation {
			errCh <- errUnexpectedID(raw.ID, consts.ConfigurationServerboundClientInformation)
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

func TestConnectHonorsCanceledContextBeforeDial(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Connect(ctx, Options{Host: "127.0.0.1", Port: "1", Username: "FeastBot"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Connect error=%v, want %v", err, context.Canceled)
	}
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

func TestDisconnectStopsPlayLoopsAndClosesSocketLast(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()

	var logsMu sync.Mutex
	var logs []string
	c := NewClient(Options{
		Debug: true,
		Logger: func(e LogEvent) {
			logsMu.Lock()
			defer logsMu.Unlock()
			if e.Error != nil {
				logs = append(logs, e.Message+" "+e.Error.Error())
				return
			}
			logs = append(logs, e.Message)
		},
	})
	c.conn = feastconn.New(clientConn)
	c.stopCh = make(chan struct{})
	c.runtimeCtx, c.runtimeCancel = context.WithCancel(context.Background())
	if err := c.fsm.Transition(state.StateLogin); err != nil {
		t.Fatalf("state login: %v", err)
	}
	if err := c.fsm.Transition(state.StateConfiguration); err != nil {
		t.Fatalf("state config: %v", err)
	}
	if err := c.fsm.Transition(state.StatePlay); err != nil {
		t.Fatalf("state play: %v", err)
	}

	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		buf := make([]byte, 512)
		for {
			if _, err := serverConn.Read(buf); err != nil {
				return
			}
		}
	}()

	c.startPlayLoops()
	time.Sleep(75 * time.Millisecond)

	done := make(chan error, 1)
	go func() { done <- c.Disconnect() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("disconnect: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("disconnect timed out")
	}

	select {
	case <-serverDone:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not observe socket close")
	}

	status := c.ShutdownStatus()
	if !status.Requested || !status.TickLoopStopped || !status.ReadLoopStopped || !status.NavLoopStopped || !status.SocketClosed {
		t.Fatalf("incomplete shutdown status: %+v", status)
	}

	logsMu.Lock()
	defer logsMu.Unlock()
	for _, line := range logs {
		if strings.Contains(line, "packet write failed") || strings.Contains(line, "use of closed network connection") {
			t.Fatalf("unexpected noisy shutdown log: %q", line)
		}
	}
}

func TestWriteAfterDisconnectReturnsErrClientClosed(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()

	c := NewClient(Options{})
	c.conn = feastconn.New(clientConn)
	c.stopCh = make(chan struct{})
	if err := c.Disconnect(); err != nil {
		t.Fatalf("disconnect: %v", err)
	}

	err := c.WritePacket(&protocol.PlayServerboundKeepAlivePacket{KeepAliveID: 1})
	if !errors.Is(err, ErrClientClosed) {
		t.Fatalf("WritePacket after disconnect error=%v, want %v", err, ErrClientClosed)
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
	c := NewClient(Options{})
	pkt := c.heartbeatPacket()
	var b bytes.Buffer
	if err := pkt.Marshal(protocol.NewWriter(&b)); err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(b.Bytes()) == 0 {
		t.Fatal("empty heartbeat payload")
	}
}

func TestMovementAuthorityAcquireRelease(t *testing.T) {
	c := NewClient(Options{})
	if !c.AcquireMovement() {
		t.Fatalf("expected first acquire to succeed")
	}
	if c.AcquireMovement() {
		t.Fatalf("expected second acquire to fail while moving")
	}
	if !c.IsMoving() {
		t.Fatalf("expected IsMoving=true while authority held")
	}
	c.ReleaseMovement()
	if c.IsMoving() {
		t.Fatalf("expected IsMoving=false after release")
	}
}

func TestPositionEventMarksFirstSync(t *testing.T) {
	c := NewClient(Options{})

	c.bus.Emit(state.PositionEvent{X: 12.5, Y: 65, Z: -4.25, Yaw: 90, Pitch: 10, TeleportID: 7})

	if !c.positionSynced {
		t.Fatalf("expected positionSynced after position event")
	}
	st := c.PlayerState()
	if st.X != 12.5 || st.Y != 65 || st.Z != -4.25 || st.Yaw != 90 || st.Pitch != 10 {
		t.Fatalf("unexpected synced player state: %+v", st)
	}
}

func TestHeartbeatPacketOnGroundUnknownChunkDefaultsTrue(t *testing.T) {
	c := NewClient(Options{})
	c.stateMu.Lock()
	c.player.X, c.player.Y, c.player.Z = 100.5, 64, -20.5
	c.stateMu.Unlock()
	pkt := c.heartbeatPacket()
	if !pkt.OnGround {
		t.Fatalf("expected OnGround=true when feet chunk unknown")
	}
}

func TestHeartbeatPacketOnGroundFromWorld(t *testing.T) {
	c := NewClient(Options{})
	chunkX := int(math.Floor(0.5 / float64(world.ChunkWidth)))
	chunkZ := int(math.Floor(0.5 / float64(world.ChunkDepth)))
	ch := world.NewChunk(chunkX, chunkZ)
	ch.SetBlock(0, 63, 0, world.BlockState{Name: "minecraft:stone"})
	c.world.AddChunk(ch)
	c.stateMu.Lock()
	c.player.X, c.player.Y, c.player.Z = 0.5, 64, 0.5
	c.stateMu.Unlock()
	pkt := c.heartbeatPacket()
	if !pkt.OnGround {
		t.Fatalf("expected OnGround=true with solid block below")
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

		// Read Client Information
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
		_, _ = s.ReadPacket() // Handshake
		_, _ = s.ReadPacket() // LoginStart
		_ = s.WritePacket(&protocol.LoginClientboundLoginSuccessPacket{UUID: [16]byte{1}, Username: "FeastBot"})
		_, _ = s.ReadPacket() // LoginAck
		_, _ = s.ReadPacket() // ClientInfo
		_ = s.WritePacket(&protocol.ConfigClientboundFinishPacket{})
		_, _ = s.ReadPacket() // AckFinish
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
