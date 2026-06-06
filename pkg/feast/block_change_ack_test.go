package feast

import (
	"bytes"
	"context"
	"net"
	"sync"
	"testing"
	"time"

	feastconn "github.com/qrjhamron/feast/pkg/conn"
	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/protocol/consts"
	"github.com/qrjhamron/feast/pkg/state"
)

// serverboundCapture wires a client to an in-memory pipe and continuously drains
// everything the client writes. stop() flushes briefly, halts the reader, and
// returns every captured packet.
func serverboundCapture(t *testing.T, c *Client) (stop func() []*protocol.RawPacket) {
	t.Helper()
	a, b := net.Pipe()
	c.conn = feastconn.New(a)
	s := feastconn.New(b)
	t.Cleanup(func() {
		_ = a.Close()
		_ = b.Close()
	})

	var (
		mu   sync.Mutex
		pkts []*protocol.RawPacket
	)
	go func() {
		for {
			raw, err := s.ReadPacket()
			if err != nil {
				return
			}
			mu.Lock()
			pkts = append(pkts, raw)
			mu.Unlock()
		}
	}()

	return func() []*protocol.RawPacket {
		time.Sleep(30 * time.Millisecond) // let trailing writes flush
		_ = b.SetReadDeadline(time.Now())
		mu.Lock()
		defer mu.Unlock()
		out := make([]*protocol.RawPacket, len(pkts))
		copy(out, pkts)
		return out
	}
}

func playerActionSequences(t *testing.T, pkts []*protocol.RawPacket) []int32 {
	t.Helper()
	var out []int32
	for _, raw := range pkts {
		if raw.ID != consts.PlayServerboundPlayerAction {
			continue
		}
		var pkt protocol.PlayServerboundPlayerActionPacket
		if err := pkt.Unmarshal(protocol.NewReader(bytes.NewReader(raw.Data))); err != nil {
			t.Fatalf("decode player action: %v", err)
		}
		out = append(out, pkt.Sequence)
	}
	return out
}

func useItemOnSequences(t *testing.T, pkts []*protocol.RawPacket) []int32 {
	t.Helper()
	var out []int32
	for _, raw := range pkts {
		if raw.ID != consts.PlayServerboundUseItemOn {
			continue
		}
		var pkt protocol.PlayServerboundUseItemOnPacket
		if err := pkt.Unmarshal(protocol.NewReader(bytes.NewReader(raw.Data))); err != nil {
			t.Fatalf("decode use item on: %v", err)
		}
		out = append(out, pkt.Sequence)
	}
	return out
}

// collapseConsecutive removes runs of equal values (a break sends Start and
// Finish with the same sequence).
func collapseConsecutive(in []int32) []int32 {
	var out []int32
	for _, v := range in {
		if len(out) == 0 || out[len(out)-1] != v {
			out = append(out, v)
		}
	}
	return out
}

func assertStrictlyIncreasing(t *testing.T, seqs []int32) {
	t.Helper()
	for i := 1; i < len(seqs); i++ {
		if seqs[i] <= seqs[i-1] {
			t.Fatalf("sequences not strictly increasing: %v", seqs)
		}
	}
}

// sendCreativeBreak runs one creative break with a short context. The look,
// Start and Finish packets are written synchronously before the call waits for
// confirmation, so capture is deterministic; the missing confirmation simply
// makes the call return on the context deadline.
func sendCreativeBreak(c *Client, pos protocol.BlockPos) {
	c.world.SetBlock(int(pos.X), int(pos.Y), int(pos.Z), 10) // make it breakable
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	_ = c.BreakBlock(ctx, pos, BreakOptions{Creative: true})
}

// TestClientSendsAckForBlockChangeSequence verifies the client emits the
// serverbound half of the block-change acknowledgement handshake: a Player
// Action carrying a sequence id (Start and Finish share one), which the server
// later acknowledges with Acknowledge Block Change.
func TestClientSendsAckForBlockChangeSequence(t *testing.T) {
	c := NewClient(Options{})
	markCoreActionReady(t, c)
	addFlatActionChunk(c)
	stop := serverboundCapture(t, c)

	sendCreativeBreak(c, protocol.BlockPos{X: 2, Y: 64, Z: 2})

	seqs := playerActionSequences(t, stop())
	if len(seqs) == 0 {
		t.Fatal("expected at least one Player Action carrying a sequence id")
	}
	for _, s := range seqs {
		if s != seqs[0] {
			t.Fatalf("Start/Finish sequences differ within one break: %v", seqs)
		}
	}
	if seqs[0] <= 0 {
		t.Fatalf("expected a positive monotonic sequence id, got %d", seqs[0])
	}
}

func TestMultipleSequentialBreaksAckAllSequences(t *testing.T) {
	c := NewClient(Options{})
	markCoreActionReady(t, c)
	addFlatActionChunk(c)
	stop := serverboundCapture(t, c)

	// All targets are within the 4.5-block break reach of the player at
	// (0.5,64,0.5) so each break passes its reach check and sends packets.
	positions := []protocol.BlockPos{{X: 1, Y: 64, Z: 1}, {X: 2, Y: 64, Z: 1}, {X: 2, Y: 64, Z: 2}}
	for _, pos := range positions {
		sendCreativeBreak(c, pos)
	}

	seqs := collapseConsecutive(playerActionSequences(t, stop()))
	if len(seqs) != len(positions) {
		t.Fatalf("got %d distinct break sequences, want %d (%v)", len(seqs), len(positions), seqs)
	}
	assertStrictlyIncreasing(t, seqs)
}

func TestMultipleSequentialPlacementsAckAllSequences(t *testing.T) {
	c := NewClient(Options{})
	markCoreActionReady(t, c)
	addFlatActionChunk(c)
	c.stateMu.Lock()
	c.player.X = 5.5
	c.player.Z = 5.5
	c.stateMu.Unlock()
	c.trackSelectedHotbarSlot(0)
	stop := serverboundCapture(t, c)

	targets := []protocol.BlockPos{{X: 2, Y: 64, Z: 2}, {X: 3, Y: 64, Z: 2}, {X: 4, Y: 64, Z: 2}}
	for i, target := range targets {
		// Keep a stocked hotbar slot so the placement passes its pre-checks and
		// sends Use Item On. The Use Item On is written before the call waits for
		// confirmation, so capture is deterministic on a short deadline.
		c.trackInventorySlot(36, protocol.ItemStack{Present: true, ItemID: 1, Count: byte(8 - i)})
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
		_ = c.PlaceBlockSurvival(ctx, target, protocol.DirectionUp)
		cancel()
	}

	seqs := collapseConsecutive(useItemOnSequences(t, stop()))
	if len(seqs) != len(targets) {
		t.Fatalf("got %d distinct placement sequences, want %d (%v)", len(seqs), len(targets), seqs)
	}
	assertStrictlyIncreasing(t, seqs)
}

// TestAckDoesNotMarkPlacementSuccessWithoutBlockUpdate proves an Acknowledge
// Block Change alone never changes the world: only a real Block Update does.
func TestAckDoesNotMarkPlacementSuccessWithoutBlockUpdate(t *testing.T) {
	c := NewClient(Options{})
	addFlatActionChunk(c)
	target := protocol.BlockPos{X: 2, Y: 64, Z: 2} // air in the flat chunk

	before, err := c.world.GetBlock(int(target.X), int(target.Y), int(target.Z))
	if err != nil {
		t.Fatalf("get block: %v", err)
	}

	// An ack carries only a sequence id; feast has no handler that mutates state.
	c.bus.Emit(state.BlockChangeAckEvent{SequenceID: 99})

	after, err := c.world.GetBlock(int(target.X), int(target.Y), int(target.Z))
	if err != nil {
		t.Fatalf("get block: %v", err)
	}
	if after != before {
		t.Fatalf("ack mutated world: before=%+v after=%+v", before, after)
	}
}

func TestAckDoesNotMutateWorld(t *testing.T) {
	c := NewClient(Options{})
	addFlatActionChunk(c)
	c.dispatcher = state.NewDispatcher(c.bus)

	chunksBefore := c.world.ChunkCount()
	target := protocol.BlockPos{X: 2, Y: 64, Z: 2}
	before, _ := c.world.GetBlock(int(target.X), int(target.Y), int(target.Z))

	var buf bytes.Buffer
	if err := (&protocol.PlayClientboundAcknowledgeBlockChangePacket{SequenceID: 17}).Marshal(protocol.NewWriter(&buf)); err != nil {
		t.Fatalf("marshal ack: %v", err)
	}
	raw := &protocol.RawPacket{ID: consts.PlayClientboundAcknowledgeBlockChange, Data: buf.Bytes()}
	if err := c.dispatcher.Dispatch(state.StatePlay, raw); err != nil {
		t.Fatalf("dispatch ack: %v", err)
	}

	if c.world.ChunkCount() != chunksBefore {
		t.Fatalf("ack changed chunk count: %d -> %d", chunksBefore, c.world.ChunkCount())
	}
	after, _ := c.world.GetBlock(int(target.X), int(target.Y), int(target.Z))
	if after != before {
		t.Fatalf("ack mutated world block: before=%+v after=%+v", before, after)
	}
}

// TestBundleWrappedChunkThenBlockUpdateWorldMutation confirms that a block
// update delivered inside a bundle (between two delimiters) still mutates the
// loaded world exactly once and in order.
func TestBundleWrappedChunkThenBlockUpdateWorldMutation(t *testing.T) {
	c := NewClient(Options{})
	addFlatActionChunk(c) // stands in for the chunk that loaded earlier in the bundle
	c.dispatcher = state.NewDispatcher(c.bus)

	target := protocol.BlockPos{X: 2, Y: 64, Z: 2}
	mk := func(p protocol.Packet) *protocol.RawPacket {
		var buf bytes.Buffer
		if err := p.Marshal(protocol.NewWriter(&buf)); err != nil {
			t.Fatalf("marshal: %v", err)
		}
		return &protocol.RawPacket{ID: p.PacketID(), Data: buf.Bytes()}
	}

	seq := []*protocol.RawPacket{
		{ID: consts.PlayClientboundBundleDelimiter},
		mk(&protocol.PlayClientboundBlockUpdatePacket{Position: target, StateID: 1}),
		{ID: consts.PlayClientboundBundleDelimiter},
	}
	for _, p := range seq {
		if err := c.dispatcher.Dispatch(state.StatePlay, p); err != nil {
			t.Fatalf("dispatch: %v", err)
		}
	}

	got, err := c.world.GetBlock(int(target.X), int(target.Y), int(target.Z))
	if err != nil {
		t.Fatalf("get block: %v", err)
	}
	if got.ID != 1 {
		t.Fatalf("bundle-wrapped block update did not mutate world: got ID %d, want 1", got.ID)
	}
}

func TestBundleNoDebugSpamByDefault(t *testing.T) {
	c := NewClient(Options{})
	c.dispatcher = state.NewDispatcher(c.bus)
	out := captureStdout(t, func() {
		_ = c.dispatcher.Dispatch(state.StatePlay, &protocol.RawPacket{ID: consts.PlayClientboundBundleDelimiter})
	})
	if out != "" {
		t.Fatalf("expected no stdout for bundle delimiter with debug=false, got %q", out)
	}
}

func TestAckNoDebugSpamByDefault(t *testing.T) {
	c := NewClient(Options{})
	c.dispatcher = state.NewDispatcher(c.bus)
	var buf bytes.Buffer
	_ = (&protocol.PlayClientboundAcknowledgeBlockChangePacket{SequenceID: 3}).Marshal(protocol.NewWriter(&buf))
	raw := &protocol.RawPacket{ID: consts.PlayClientboundAcknowledgeBlockChange, Data: buf.Bytes()}
	out := captureStdout(t, func() {
		_ = c.dispatcher.Dispatch(state.StatePlay, raw)
	})
	if out != "" {
		t.Fatalf("expected no stdout for ack with debug=false, got %q", out)
	}
}
