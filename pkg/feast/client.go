package feast

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"time"

	feastconn "github.com/user/feastgo/pkg/conn"
	"github.com/user/feastgo/pkg/protocol"
	"github.com/user/feastgo/pkg/protocol/consts"
	"github.com/user/feastgo/pkg/state"
)

const protocolVersion765 int32 = 765

// Options configures an internal Feast client.
type Options struct {
	Host         string
	Port         string
	Username     string
	Debug        bool
	DebugPackets bool
	Logger       func(LogEvent)
}

// Client orchestrates conn/state/dispatch flows.
type Client struct {
	opts       Options
	conn       *feastconn.Conn
	fsm        *state.FSM
	bus        *state.EventBus
	dispatcher *state.Dispatcher

	dialFunc func(network, address string) (net.Conn, error)

	stopCh  chan struct{}
	once    sync.Once
	wg      sync.WaitGroup
	writeMu sync.Mutex
	stateMu sync.RWMutex
	statsMu sync.RWMutex
	player  PlayerState
	stats   Stats
}

// LogEvent is a structured debug log entry emitted by the client.
type LogEvent struct {
	Time     time.Time
	Message  string
	State    state.State
	PacketID int32
	Error    error
}

// Stats is a point-in-time runtime snapshot.
type Stats struct {
	ConnectedAt        time.Time
	ConnectedFor       time.Duration
	PacketsReceived    uint64
	PacketsSent        uint64
	LastKeepAliveAt    time.Time
	LastPositionSyncAt time.Time
	CurrentState       state.State
}

// PlayerState is the client's current best-known player state.
type PlayerState struct {
	UUID       [16]byte
	EntityID   int32
	X          float64
	Y          float64
	Z          float64
	Yaw        float32
	Pitch      float32
	Health     float32
	Food       int32
	Saturation float32
}

// NewClient creates a new internal orchestrator client.
func NewClient(opts Options) *Client {
	c := &Client{
		opts:       opts,
		fsm:        state.NewFSM(),
		bus:        state.NewEventBus(),
		dispatcher: state.NewDispatcher(state.NewEventBus()),
		dialFunc:   net.Dial,
	}
	c.registerStateHandlers()
	return c
}

// On registers a state event handler.
func (c *Client) On(eventType string, handler func(state.Event)) {
	c.bus.On(eventType, handler)
}

// Connect dials server, performs login/config, and starts play loops.
func (c *Client) Connect() error {
	address := net.JoinHostPort(c.opts.Host, c.opts.Port)
	nc, err := c.dialFunc("tcp", address)
	if err != nil {
		return err
	}

	c.conn = feastconn.New(nc)
	c.dispatcher = state.NewDispatcher(c.bus)
	c.stopCh = make(chan struct{})
	c.statsMu.Lock()
	c.stats.ConnectedAt = time.Now()
	c.statsMu.Unlock()
	c.log("tcp connected", -1, nil)

	if err := c.fsm.Transition(state.StateLogin); err != nil {
		_ = c.conn.Close()
		return err
	}
	c.log("state transition: login", -1, nil)

	if err := c.writePacket(&handshakePacket{
		ProtocolVersion: protocolVersion765,
		ServerAddress:   c.opts.Host,
		ServerPort:      mustPort(c.opts.Port),
		NextState:       2,
	}); err != nil {
		_ = c.conn.Close()
		return err
	}

	if err := c.runLoginFlow(); err != nil {
		_ = c.conn.Close()
		return err
	}
	if err := c.fsm.Transition(state.StateConfiguration); err != nil {
		_ = c.conn.Close()
		return err
	}
	c.log("state transition: configuration", -1, nil)

	if err := c.runConfigFlow(); err != nil {
		_ = c.conn.Close()
		return err
	}
	if err := c.fsm.Transition(state.StatePlay); err != nil {
		_ = c.conn.Close()
		return err
	}
	c.log("state transition: play", -1, nil)

	c.startPlayLoops()
	return nil
}

// Disconnect stops loops and closes the underlying transport.
func (c *Client) Disconnect() error {
	c.once.Do(func() {
		if c.stopCh != nil {
			close(c.stopCh)
		}
		if c.conn != nil {
			_ = c.conn.Close()
		}
	})
	c.wg.Wait()
	return nil
}

// Close closes the client connection.
func (c *Client) Close() error {
	return c.Disconnect()
}

// CurrentState returns the current protocol state.
func (c *Client) CurrentState() state.State {
	return c.currentState()
}

// Events returns the client's event bus.
func (c *Client) Events() *state.EventBus {
	return c.bus
}

// PlayerState returns a snapshot of the current player state.
func (c *Client) PlayerState() PlayerState {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.player
}

// Stats returns a snapshot of runtime counters and timestamps.
func (c *Client) Stats() Stats {
	c.statsMu.RLock()
	stats := c.stats
	c.statsMu.RUnlock()
	stats.CurrentState = c.currentState()
	if !stats.ConnectedAt.IsZero() {
		stats.ConnectedFor = time.Since(stats.ConnectedAt)
	}
	return stats
}

func mustPort(port string) uint16 {
	p, err := net.LookupPort("tcp", port)
	if err != nil {
		return 25565
	}
	if p < 0 || p > 65535 {
		return 25565
	}
	return uint16(p)
}

func heartbeatPacket() *protocol.PlayServerboundSetPlayerPositionAndRotationPacket {
	return &protocol.PlayServerboundSetPlayerPositionAndRotationPacket{
		OnGround: true,
	}
}

func (c *Client) heartbeatPacket() *protocol.PlayServerboundSetPlayerPositionAndRotationPacket {
	st := c.PlayerState()
	return &protocol.PlayServerboundSetPlayerPositionAndRotationPacket{
		X:        st.X,
		Y:        st.Y,
		Z:        st.Z,
		Yaw:      st.Yaw,
		Pitch:    st.Pitch,
		OnGround: true,
	}
}

func (c *Client) emitDisconnect(reason string) {
	c.bus.Emit(state.DisconnectEvent{Reason: reason})
}

func (c *Client) emitDisconnectWithClean(reason string, clean bool) {
	c.bus.Emit(state.DisconnectEvent{Reason: reason, Clean: clean})
}

func (c *Client) currentState() state.State {
	if c.fsm == nil {
		return state.StateHandshaking
	}
	return c.fsm.Current()
}

func (c *Client) requireConn() error {
	if c.conn == nil {
		return fmt.Errorf("connection not initialized")
	}
	return nil
}

func ticker50ms() *time.Ticker {
	return time.NewTicker(50 * time.Millisecond)
}

func (c *Client) registerStateHandlers() {
	c.bus.On("login", func(e state.Event) {
		ev, ok := e.(state.LoginEvent)
		if !ok {
			return
		}
		c.stateMu.Lock()
		c.player.UUID = ev.RawUUID
		c.stateMu.Unlock()
	})
	c.bus.On("play_login", func(e state.Event) {
		ev, ok := e.(state.PlayLoginEvent)
		if !ok {
			return
		}
		c.stateMu.Lock()
		c.player.EntityID = ev.EntityID
		c.stateMu.Unlock()
	})
	c.bus.On("position", func(e state.Event) {
		ev, ok := e.(state.PositionEvent)
		if !ok {
			return
		}
		c.stateMu.Lock()
		c.player.X = ev.X
		c.player.Y = ev.Y
		c.player.Z = ev.Z
		c.player.Yaw = ev.Yaw
		c.player.Pitch = ev.Pitch
		c.stateMu.Unlock()
		c.statsMu.Lock()
		c.stats.LastPositionSyncAt = time.Now()
		c.statsMu.Unlock()
		c.log("position sync received", -1, nil)
	})
	c.bus.On("health", func(e state.Event) {
		ev, ok := e.(state.HealthEvent)
		if !ok {
			return
		}
		c.stateMu.Lock()
		c.player.Health = ev.Health
		c.player.Food = ev.Food
		c.player.Saturation = ev.Saturation
		c.stateMu.Unlock()
	})
	c.bus.On("keep_alive", func(e state.Event) {
		if _, ok := e.(state.KeepAliveEvent); !ok {
			return
		}
		c.statsMu.Lock()
		c.stats.LastKeepAliveAt = time.Now()
		c.statsMu.Unlock()
		c.log("keepalive received", consts.PlayClientboundClientboundKeepAlive, nil)
	})
	c.bus.On("kick", func(e state.Event) {
		ev, ok := e.(state.KickEvent)
		if !ok {
			return
		}
		c.log("kick received: "+ev.Reason, -1, nil)
	})
}

func (c *Client) writePacket(p protocol.Packet) error {
	if c.conn == nil {
		return fmt.Errorf("connection not initialized")
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	err := c.conn.WritePacket(p)
	if err != nil {
		c.log("packet write failed", p.PacketID(), err)
		return err
	}
	c.statsMu.Lock()
	c.stats.PacketsSent++
	c.statsMu.Unlock()
	if c.opts.DebugPackets {
		c.log("packet sent", p.PacketID(), nil)
	}
	return nil
}

func (c *Client) readPacket() (*protocol.RawPacket, error) {
	if c.conn == nil {
		return nil, fmt.Errorf("connection not initialized")
	}
	raw, err := c.conn.ReadPacket()
	if err != nil {
		return nil, err
	}
	c.statsMu.Lock()
	c.stats.PacketsReceived++
	c.statsMu.Unlock()
	if c.opts.DebugPackets {
		c.log("packet received", raw.ID, nil)
	}
	return raw, nil
}

func (c *Client) log(message string, packetID int32, err error) {
	if !c.opts.Debug && !(c.opts.DebugPackets && packetID >= 0) {
		return
	}
	ev := LogEvent{
		Time: time.Now(), Message: message, State: c.currentState(),
		PacketID: packetID, Error: err,
	}
	if c.opts.Logger != nil {
		c.opts.Logger(ev)
		return
	}
	if err != nil {
		log.Printf("[feast] state=%v packet=0x%02x %s: %v", ev.State, ev.PacketID, ev.Message, err)
		return
	}
	log.Printf("[feast] state=%v packet=0x%02x %s", ev.State, ev.PacketID, ev.Message)
}

func (c *Client) closing() bool {
	if c.stopCh == nil {
		return false
	}
	select {
	case <-c.stopCh:
		return true
	default:
		return false
	}
}

func classifyReadError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, io.EOF) {
		return "eof"
	}
	if errors.Is(err, net.ErrClosed) {
		return "closed"
	}
	return "read_error"
}
