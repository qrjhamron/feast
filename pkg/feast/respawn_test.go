package feast

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/state"
	"github.com/qrjhamron/feast/pkg/world"
)

// emitRespawn drives a respawn through the registered feast handler.
func emitRespawn(c *Client, dimension string) {
	c.bus.Emit(state.RespawnEvent{
		DimensionType: dimension, DimensionName: dimension,
		HashedSeed: 1, GameMode: 0, PreviousGameMode: 1,
	})
}

// dispatchRespawnPacket exercises the full decode+dispatch path on a client by
// wiring its dispatcher to its own bus (as Connect does) and feeding a real
// Respawn packet.
func dispatchRespawnPacket(t *testing.T, c *Client, pkt *protocol.PlayClientboundRespawnPacket) {
	t.Helper()
	c.dispatcher = state.NewDispatcher(c.bus)
	var buf bytes.Buffer
	if err := pkt.Marshal(protocol.NewWriter(&buf)); err != nil {
		t.Fatalf("marshal respawn: %v", err)
	}
	raw := &protocol.RawPacket{ID: pkt.PacketID(), Data: buf.Bytes()}
	if err := c.dispatcher.Dispatch(state.StatePlay, raw); err != nil {
		t.Fatalf("dispatch respawn: %v", err)
	}
}

func TestClientRespawnResetsWorldChunks(t *testing.T) {
	c := NewClient(Options{})
	addFlatActionChunk(c)
	c.world.AddChunk(world.NewChunk(1, 0))
	if c.world.ChunkCount() == 0 {
		t.Fatal("precondition: expected loaded chunks before respawn")
	}
	emitRespawn(c, "minecraft:the_nether")
	if got := c.world.ChunkCount(); got != 0 {
		t.Fatalf("chunk count after respawn = %d, want 0", got)
	}
}

func TestClientRespawnClearsBlockEntities(t *testing.T) {
	c := NewClient(Options{})
	ch := world.NewChunk(0, 0)
	ch.BlockEntities = []world.BlockEntity{{X: 2, Y: 64, Z: 2, TypeID: 7}}
	c.world.AddChunk(ch)
	if len(c.world.GetBlockEntities(0, 0)) != 1 {
		t.Fatal("precondition: expected one block entity before respawn")
	}
	emitRespawn(c, "minecraft:the_end")
	if be := c.world.GetBlockEntities(0, 0); len(be) != 0 {
		t.Fatalf("block entities after respawn = %d, want 0", len(be))
	}
}

// TestRespawnClearsBlockEntities exercises the same guarantee through the full
// packet decode + dispatch path rather than a direct event emit.
func TestRespawnClearsBlockEntities(t *testing.T) {
	c := NewClient(Options{})
	ch := world.NewChunk(0, 0)
	ch.BlockEntities = []world.BlockEntity{{X: 1, Y: 70, Z: 1, TypeID: 3}}
	c.world.AddChunk(ch)
	if len(c.world.GetBlockEntities(0, 0)) != 1 {
		t.Fatal("precondition: expected one block entity before respawn")
	}
	dispatchRespawnPacket(t, c, &protocol.PlayClientboundRespawnPacket{
		DimensionType: "minecraft:overworld", DimensionName: "minecraft:overworld",
		PreviousGameMode: 0xFF,
	})
	if be := c.world.GetBlockEntities(0, 0); len(be) != 0 {
		t.Fatalf("block entities after respawn = %d, want 0", len(be))
	}
}

func TestClientRespawnClearsEntities(t *testing.T) {
	c := NewClient(Options{})
	c.entities.Upsert(&world.Entity{ID: 100, X: 1, Y: 2, Z: 3})
	c.entities.Upsert(&world.Entity{ID: 101, X: 4, Y: 5, Z: 6})
	if len(c.entities.All()) != 2 {
		t.Fatal("precondition: expected two tracked entities before respawn")
	}
	emitRespawn(c, "minecraft:the_nether")
	if got := len(c.entities.All()); got != 0 {
		t.Fatalf("tracked entities after respawn = %d, want 0", got)
	}
}

func TestClientRespawnMarksPositionUnsynced(t *testing.T) {
	c := NewClient(Options{})
	c.stateMu.Lock()
	c.positionSynced = true
	c.stateMu.Unlock()
	if !c.PositionSynced() {
		t.Fatal("precondition: expected positionSynced=true before respawn")
	}
	emitRespawn(c, "minecraft:the_nether")
	if c.PositionSynced() {
		t.Fatal("expected positionSynced=false after respawn")
	}
}

func TestClientRespawnCancelsNavigation(t *testing.T) {
	c := NewClient(Options{})
	ctx, cancel := context.WithCancel(context.Background())
	c.navMu.Lock()
	c.navCtx = ctx
	c.navCancel = cancel
	c.navMu.Unlock()

	emitRespawn(c, "minecraft:the_end")

	// StopNavigation must have cancelled the active context and cleared it.
	select {
	case <-ctx.Done():
	default:
		t.Fatal("expected active navigation context to be cancelled by respawn")
	}
	c.navMu.Lock()
	cleared := c.navCtx == nil && c.navCancel == nil
	c.navMu.Unlock()
	if !cleared {
		t.Fatal("expected navigation context to be cleared after respawn")
	}
}

func TestClientReadyAfterRespawnPositionSync(t *testing.T) {
	c := NewClient(Options{})
	c.stateMu.Lock()
	c.positionSynced = true
	c.stateMu.Unlock()

	emitRespawn(c, "minecraft:the_nether")
	if c.PositionSynced() {
		t.Fatal("expected not-ready immediately after respawn")
	}

	// The post-respawn position sync makes the bot ready again.
	c.bus.Emit(state.PositionEvent{X: 10, Y: 64, Z: -7})
	if !c.PositionSynced() {
		t.Fatal("expected positionSynced=true after post-respawn position sync")
	}
}

func TestRespawnDuringActionDoesNotDeadlock(t *testing.T) {
	c := NewClient(Options{})
	ctx, cancel := context.WithCancel(context.Background())
	c.navMu.Lock()
	c.navCtx = ctx
	c.navCancel = cancel
	c.navMu.Unlock()

	// Simulate an in-flight action goroutine that unwinds on cancellation.
	navStopped := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(navStopped)
	}()

	// The respawn handler runs synchronously; guard against it blocking.
	emitted := make(chan struct{})
	go func() {
		emitRespawn(c, "minecraft:the_end")
		close(emitted)
	}()
	select {
	case <-emitted:
	case <-time.After(2 * time.Second):
		t.Fatal("respawn handler deadlocked while an action was active")
	}
	select {
	case <-navStopped:
	case <-time.After(time.Second):
		t.Fatal("respawn did not cancel the active action")
	}
}

func TestRespawnInvalidatesWorldAndNavCache(t *testing.T) {
	c := NewClient(Options{})
	addFlatActionChunk(c)
	before := c.HPAInvalidations()

	emitRespawn(c, "minecraft:the_nether")

	if c.world.ChunkCount() != 0 {
		t.Fatalf("world not cleared on respawn: %d chunks remain", c.world.ChunkCount())
	}
	if got := c.HPAInvalidations(); got != before+1 {
		t.Fatalf("HPA invalidations = %d, want %d", got, before+1)
	}
}

func TestRespawnNoDebugSpamByDefault(t *testing.T) {
	c := NewClient(Options{}) // Debug defaults to false.
	out := captureStdout(t, func() {
		dispatchRespawnPacket(t, c, &protocol.PlayClientboundRespawnPacket{
			DimensionType: "minecraft:the_nether", DimensionName: "minecraft:the_nether",
			PreviousGameMode: 0xFF, DataKept: 0x03,
		})
	})
	if out != "" {
		t.Fatalf("expected no stdout for respawn with debug=false, got %q", out)
	}
}
