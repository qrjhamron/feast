package feast

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	feastconn "github.com/qrjhamron/feast/pkg/conn"
	"github.com/qrjhamron/feast/pkg/nav/hpa"
	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/state"
	"github.com/qrjhamron/feast/pkg/world"
)

const protocolVersion765 int32 = 765

// Options configures a FeastGo client connection and behavior.
type Options struct {
	// Host is the server hostname or IP address.
	Host string
	// Port is the server port (typically "25565").
	Port string
	// Username is the player's in-game name.
	Username string
	// Debug enables verbose client logging.
	Debug bool
	// DebugPackets enables logging of all sent/received packets.
	DebugPackets bool
	// Logger is an optional custom log handler.
	Logger func(LogEvent)
}

// Client represents an active FeastGo bot connection. It provides methods to
// interact with the world, entities, and inventory, and dispatches events.
type Client struct {
	opts       Options
	conn       *feastconn.Conn
	fsm        *state.FSM
	bus        *state.EventBus
	dispatcher *state.Dispatcher

	dialFunc func(network, address string) (net.Conn, error)

	stopCh           chan struct{}
	stopMu           sync.Mutex
	stopped          bool
	once             sync.Once
	teardownOnce     sync.Once
	teardownDone     chan struct{}
	managedGoids     sync.Map
	runtimeCtx       context.Context
	runtimeCancel    context.CancelFunc
	shutdownStatus   ShutdownStatus
	wg               sync.WaitGroup
	writeMu          sync.Mutex
	stateMu          sync.RWMutex
	statsMu          sync.RWMutex
	inventoryMu      sync.RWMutex
	hpaMu            sync.RWMutex
	navMu            sync.Mutex
	moveMu           sync.Mutex
	teleportMu       sync.Mutex
	navCtx           context.Context
	navCancel        context.CancelFunc
	moving           bool
	player           PlayerState
	stats            Stats
	inventory        InventoryState
	world            *world.World
	entities         *world.EntityStore
	hpaNav           *hpa.HPANavigator
	hpaUpdater       *hpa.GraphUpdater
	hpaGraph         *hpa.AbstractGraph
	hpaClusters      *hpa.ClusterManager
	hpaBuilder       *hpa.GraphBuilder
	hpaChunkLoads    int
	hpaRebuiltAll    bool
	chunkSem         chan struct{}
	hpaInvalidations int32
	positionSyncSeq  uint64
	positionSynced   bool

	movementProfile   MovementProfile
	activeOptions     MovementOptions
	lastMovementStats MovementStats
	statsTrackMu      sync.RWMutex

	activeContainer   *Container
	containerSlots    map[int32]map[int]ItemStack
	containerStateIDs map[int32]int32
	containerMu       sync.RWMutex
}

// LogEvent is a structured debug log entry emitted by the client.
type LogEvent struct {
	Time     time.Time
	Message  string
	State    state.State
	PacketID int32
	Error    error
}

// ShutdownStatus records which phases of client shutdown completed.
type ShutdownStatus struct {
	Requested       bool
	TickLoopStopped bool
	ReadLoopStopped bool
	NavLoopStopped  bool
	SocketClosed    bool
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

// HPAStats is a point-in-time HPA* graph/debug snapshot.
type HPAStats struct {
	Chunks        int
	Clusters      int
	BuiltClusters int
	Entrances     int
	GraphNodes    int
	GraphEdges    int
}

// PlayerState is the client's current best-known player state.
type PlayerState struct {
	// UUID is the player's unique identifier.
	UUID [16]byte
	// EntityID is the runtime ID of the player entity.
	EntityID int32
	// X, Y, Z are the player's current coordinates.
	X float64
	Y float64
	Z float64
	// Yaw and Pitch define the player's rotation.
	Yaw   float32
	Pitch float32
	// VelocityY tracks vertical falling speed.
	VelocityY float64
	// OnGround is true if the player is touching solid blocks.
	OnGround bool
	// Health tracks the player's current health.
	Health float32
	// Food tracks the player's food level.
	Food int32
	// Saturation tracks food saturation.
	Saturation float32
}

// NewClient creates a new internal orchestrator client.
func NewClient(opts Options) *Client {
	c := &Client{
		opts:              opts,
		fsm:               state.NewFSM(),
		bus:               state.NewEventBus(),
		dispatcher:        state.NewDispatcher(state.NewEventBus()),
		dialFunc:          net.Dial,
		world:             world.NewWorld(),
		entities:          world.NewEntityStore(),
		chunkSem:          make(chan struct{}, 8),
		movementProfile:   MovementBotLike,
		containerSlots:    make(map[int32]map[int]ItemStack),
		containerStateIDs: make(map[int32]int32),
		teardownDone:      make(chan struct{}),
	}
	c.world.SetEntityStore(c.entities)
	c.initHPA()
	c.registerStateHandlers()
	return c
}

func (c *Client) initHPA() {
	graph := hpa.NewAbstractGraph()
	clusters := hpa.NewClusterManager(c.bus)
	builder := hpa.NewGraphBuilder(c.world, graph, clusters)
	updater := hpa.NewGraphUpdater(c.world, graph, clusters, builder, c.bus)
	updater.Start()
	c.hpaMu.Lock()
	c.hpaUpdater = updater
	c.hpaGraph = graph
	c.hpaClusters = clusters
	c.hpaBuilder = builder
	c.hpaChunkLoads = 0
	c.hpaRebuiltAll = false
	c.hpaNav = hpa.NewHPANavigator(c.world, hpa.NewHPAPlanner(c.world, graph, clusters), updater)
	c.hpaMu.Unlock()
	c.bus.On("chunk_load", func(e state.Event) {
		ev, ok := e.(state.ChunkLoadEvent)
		if !ok {
			return
		}
		c.hpaClusters.MarkDirty(int(ev.ChunkX), int(ev.ChunkZ))
		c.hpaMu.Lock()
		c.hpaChunkLoads++
		c.hpaMu.Unlock()
		if c.opts.DebugPackets {
			log.Printf("[hpa] chunk_load chunk=(%d,%d) clusters=%d built=%d world_chunks=%d", ev.ChunkX, ev.ChunkZ, c.hpaClusters.Count(), c.hpaClusters.BuiltCount(), c.world.ChunkCount())
		}
	})
}

// Advanced: On registers a raw state event handler.
func (c *Client) On(eventType string, handler func(state.Event)) (int, error) {
	return c.bus.On(eventType, handler)
}

// World returns the current world state handle.
// Advanced: This provides direct access to the chunk array and raw block data.
func (c *Client) World() *world.World {
	return c.world
}

// Advanced: HPANav returns the internal HPA* navigator handle.
func (c *Client) HPANav() *hpa.HPANavigator {
	return c.hpaNav
}

// Entities returns the tracked entity store.
// Advanced: This provides direct access to the raw entity map.
func (c *Client) Entities() *world.EntityStore {
	return c.entities
}

// GetPosition returns the client's current player transform snapshot.
func (c *Client) GetPosition() (x, y, z float64, yaw, pitch float32) {
	st := c.PlayerState()
	return st.X, st.Y, st.Z, st.Yaw, st.Pitch
}

func (c *Client) PositionSynced() bool {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.positionSynced
}

// EntityID returns the player's current entity ID.
func (c *Client) EntityID() int32 {
	st := c.PlayerState()
	return st.EntityID
}

// Advanced: WritePacket sends one raw packet through the current transport.
func (c *Client) WritePacket(p protocol.Packet) error {
	return c.writePacket(p)
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
	c.runtimeCtx, c.runtimeCancel = context.WithCancel(context.Background())
	c.stopMu.Lock()
	c.stopped = false
	c.shutdownStatus = ShutdownStatus{}
	c.stopMu.Unlock()
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

// ErrClientClosed is returned when a write is attempted after the client has been shut down.
// ErrClientClosed is defined in errors.go

// Disconnect stops loops and closes the underlying transport.
//
// Disconnect is idempotent and safe to call multiple times, including
// concurrently. When called from a non-managed goroutine (the common case) it
// blocks until all client goroutines have drained and the socket is closed, so
// [Client.ShutdownStatus] is fully complete on return.
//
// When called from within a client-managed goroutine — for example from an
// event handler that runs on the read/heartbeat/chunk/navigation loop —
// Disconnect cannot wait for that goroutine to finish without deadlocking on
// itself. In that case it signals shutdown, runs teardown in the background,
// and returns immediately; teardown completes once the calling goroutine
// unwinds and the loops drain.
//
// Shutdown ordering:
//  1. Set client state to disconnected (prevents new writes).
//  2. Cancel runtime context and signal the stop channel (stops all loops).
//  3. Stop navigation and unblock any in-flight socket read/write.
//  4. Wait for all goroutines to finish.
//  5. Close the socket last.
//  6. Reset world/entity/event-bus state.
func (c *Client) Disconnect() error {
	c.signalShutdown()
	if c.inManagedGoroutine() {
		// Re-entrant call from a client-owned goroutine: waiting on c.wg here
		// would wait on the caller itself. Run teardown asynchronously instead.
		go c.runTeardown()
		return nil
	}
	c.runTeardown()
	<-c.teardownDone
	return nil
}

// signalShutdown performs the non-blocking shutdown signalling steps exactly
// once: marking the client stopped, cancelling the runtime context, closing the
// stop channel, stopping navigation, and unblocking any in-flight socket I/O.
func (c *Client) signalShutdown() {
	c.once.Do(func() {
		// Step 1: Mark disconnected state.
		c.stopMu.Lock()
		c.stopped = true
		c.shutdownStatus.Requested = true
		c.stopMu.Unlock()

		if c.fsm != nil {
			_ = c.fsm.Transition(state.StateDisconnected)
		}

		// Step 2: Cancel runtime context and signal stop to all loops.
		if c.runtimeCancel != nil {
			c.runtimeCancel()
		}
		if c.stopCh != nil {
			close(c.stopCh)
		}

		// Step 3: Stop navigation.
		c.StopNavigation()

		// Unblock pending ReadPacket and WritePacket without closing the socket yet.
		if c.conn != nil {
			_ = c.conn.SetDeadline(time.Now())
		}
	})
}

// runTeardown waits for all client goroutines to drain, closes the socket, and
// resets runtime state exactly once. It closes c.teardownDone on completion so
// that blocking callers of Disconnect can observe full shutdown.
func (c *Client) runTeardown() {
	c.teardownOnce.Do(func() {
		// Step 4: Wait for loops to drain (they check stopCh).
		c.wg.Wait()
		c.stopMu.Lock()
		c.shutdownStatus.TickLoopStopped = true
		c.shutdownStatus.ReadLoopStopped = true
		c.shutdownStatus.NavLoopStopped = true
		c.stopMu.Unlock()

		// Step 5: Stop packet writers and close socket last.
		c.writeMu.Lock()
		if c.conn != nil {
			_ = c.conn.Close()
		}
		c.writeMu.Unlock()
		c.stopMu.Lock()
		c.shutdownStatus.SocketClosed = true
		c.stopMu.Unlock()

		// Step 6: Reset runtime state.
		if c.hpaUpdater != nil {
			c.hpaUpdater.Stop()
		}
		c.stateMu.Lock()
		c.world = world.NewWorld()
		c.world.SetEntityStore(c.entities)
		c.entities.Reset()
		c.positionSynced = false
		c.stateMu.Unlock()
		c.bus.Reset()
		c.initHPA()
		c.registerStateHandlers()

		close(c.teardownDone)
	})
}

// Close closes the client connection.
func (c *Client) Close() error {
	return c.Disconnect()
}

// CurrentState returns the current protocol state.
func (c *Client) CurrentState() state.State {
	return c.currentState()
}

// Advanced: HPAInvalidations returns the count of triggered HPA invalidations.
func (c *Client) HPAInvalidations() int32 {
	return atomic.LoadInt32(&c.hpaInvalidations)
}

// Advanced: WorldGraph returns the internal AbstractGraph handle.
func (c *Client) WorldGraph() *hpa.AbstractGraph {
	c.hpaMu.RLock()
	defer c.hpaMu.RUnlock()
	return c.hpaGraph
}

// Advanced: WorldClusters returns the internal ClusterManager handle.
func (c *Client) WorldClusters() *hpa.ClusterManager {
	c.hpaMu.RLock()
	defer c.hpaMu.RUnlock()
	return c.hpaClusters
}

// Advanced: Events returns the client's internal event bus.
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

// Advanced: HPAStats returns a snapshot of the current HPA* graph state.
func (c *Client) HPAStats() HPAStats {
	stats := HPAStats{}
	if c.world != nil {
		stats.Chunks = c.world.ChunkCount()
	}
	c.hpaMu.RLock()
	defer c.hpaMu.RUnlock()
	if c.hpaClusters != nil {
		stats.Clusters = c.hpaClusters.Count()
		stats.BuiltClusters = c.hpaClusters.BuiltCount()
		stats.Entrances = c.hpaClusters.EntranceCount()
	}
	if c.hpaGraph != nil {
		stats.GraphNodes, stats.GraphEdges = c.hpaGraph.Stats()
	}
	return stats
}

// ShutdownStatus returns a snapshot of the latest shutdown phases.
func (c *Client) ShutdownStatus() ShutdownStatus {
	c.stopMu.Lock()
	defer c.stopMu.Unlock()
	return c.shutdownStatus
}

// Advanced: RebuildHPA rebuilds all currently tracked HPA* clusters.
func (c *Client) RebuildHPA() {
	_ = c.RebuildHPAWithContext(context.Background())
}

// Advanced: RebuildHPAWithContext rebuilds all currently tracked HPA* clusters.
func (c *Client) RebuildHPAWithContext(ctx context.Context) error {
	c.hpaMu.RLock()
	builder := c.hpaBuilder
	c.hpaMu.RUnlock()
	if builder != nil {
		return builder.RebuildAllWithContext(ctx)
	}
	return nil
}

// Advanced: RebuildHPAAround rebuilds HPA* clusters for loaded chunks near a block position.
func (c *Client) RebuildHPAAround(blockX, blockZ, radiusChunks int) {
	_ = c.RebuildHPAAroundWithContext(context.Background(), blockX, blockZ, radiusChunks)
}

// Advanced: RebuildHPAAroundWithContext rebuilds HPA* clusters for loaded chunks near a block position.
func (c *Client) RebuildHPAAroundWithContext(ctx context.Context, blockX, blockZ, radiusChunks int) error {
	if c.world == nil || radiusChunks < 0 {
		return nil
	}
	centerX := floorDiv(blockX, world.ChunkWidth)
	centerZ := floorDiv(blockZ, world.ChunkDepth)
	coords := c.world.ChunkCoords()

	c.hpaMu.RLock()
	builder := c.hpaBuilder
	clusters := c.hpaClusters
	c.hpaMu.RUnlock()
	if builder == nil || clusters == nil {
		return nil
	}
	for _, coord := range coords {
		if err := ctx.Err(); err != nil {
			return err
		}
		if absInt(coord[0]-centerX) <= radiusChunks && absInt(coord[1]-centerZ) <= radiusChunks {
			clusters.MarkDirty(coord[0], coord[1])
		}
	}
	for _, coord := range coords {
		if err := ctx.Err(); err != nil {
			return err
		}
		if absInt(coord[0]-centerX) <= radiusChunks && absInt(coord[1]-centerZ) <= radiusChunks {
			if err := builder.BuildClusterWithContext(ctx, clusters.GetOrCreate(coord[0], coord[1])); err != nil {
				return err
			}
		}
	}
	return nil
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

func (c *Client) heartbeatPacket() *protocol.PlayServerboundSetPlayerPositionAndRotationPacket {
	st := c.PlayerState()
	onGround := c.world.IsOnGround(world.Vec3{X: st.X, Y: st.Y, Z: st.Z})
	return &protocol.PlayServerboundSetPlayerPositionAndRotationPacket{
		X:        st.X,
		Y:        st.Y,
		Z:        st.Z,
		Yaw:      st.Yaw,
		Pitch:    st.Pitch,
		OnGround: onGround,
	}
}

// Advanced: AcquireMovement grants exclusive movement authority to navigation paths.
func (c *Client) AcquireMovement() bool {
	c.moveMu.Lock()
	defer c.moveMu.Unlock()
	if c.moving {
		return false
	}
	c.moving = true
	return true
}

// Advanced: ReleaseMovement relinquishes navigation movement authority.
func (c *Client) ReleaseMovement() {
	c.moveMu.Lock()
	c.moving = false
	c.moveMu.Unlock()
}

// Advanced: AcquireMovementBy grants exclusive movement authority.
func (c *Client) AcquireMovementBy(_ string) bool {
	c.moveMu.Lock()
	defer c.moveMu.Unlock()
	if c.moving {
		return false
	}
	c.moving = true
	return true
}

// Advanced: ReleaseMovementBy relinquishes movement authority.
func (c *Client) ReleaseMovementBy(by string) {
	c.moveMu.Lock()
	c.moving = false
	c.moveMu.Unlock()
}

// IsMoving reports whether navigation currently owns movement authority.
func (c *Client) IsMoving() bool {
	c.moveMu.Lock()
	moving := c.moving
	c.moveMu.Unlock()
	if moving {
		return true
	}
	c.navMu.Lock()
	defer c.navMu.Unlock()
	return c.navCtx != nil
}

func (c *Client) onGroundAt(feetX, feetY, feetZ int) bool {
	if c.world == nil {
		return true
	}
	return c.world.IsOnGround(world.StandingCenter(world.BlockPos{X: int32(feetX), Y: int32(feetY), Z: int32(feetZ)}))
}

func floorDiv(a, b int) int {
	q := a / b
	r := a % b
	if r != 0 && ((r < 0) != (b < 0)) {
		q--
	}
	return q
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
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
		return ErrNotConnected
	}
	return nil
}

func ticker50ms() *time.Ticker {
	return time.NewTicker(50 * time.Millisecond)
}

func (c *Client) writePacket(p protocol.Packet) error {
	if c.conn == nil {
		return ErrNotConnected
	}
	// Check if we are shutting down before attempting a write.
	c.stopMu.Lock()
	stopped := c.stopped
	c.stopMu.Unlock()
	if stopped {
		return ErrClientClosed
	}
	if c.closing() {
		return ErrClientClosed
	}
	if posPkt, ok := p.(*protocol.PlayServerboundSetPlayerPositionAndRotationPacket); ok {
		c.stateMu.Lock()
		c.player.X = posPkt.X
		c.player.Y = posPkt.Y
		c.player.Z = posPkt.Z
		c.player.Yaw = posPkt.Yaw
		c.player.Pitch = posPkt.Pitch
		c.player.OnGround = posPkt.OnGround
		c.stateMu.Unlock()
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	err := c.conn.WritePacket(p)
	if err != nil {
		// If the client is shutting down, return a clean error instead of logging.
		if c.closing() || c.isStopped() {
			return ErrClientClosed
		}
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
		return nil, ErrNotConnected
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

func (c *Client) isStopped() bool {
	c.stopMu.Lock()
	defer c.stopMu.Unlock()
	return c.stopped
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
