package feast

import (
	"context"
	"io"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	feastconn "github.com/qrjhamron/feast/pkg/conn"
	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/state"
	"github.com/qrjhamron/feast/pkg/world"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = old
	}()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	return string(out)
}

func runSuccessfulBreakForLogTest(t *testing.T, c *Client) {
	t.Helper()
	addFlatActionChunk(c)
	c.world.SetBlock(2, 64, 2, 10)
	go func() {
		time.Sleep(25 * time.Millisecond)
		c.world.SetBlock(2, 64, 2, 0)
		c.bus.Emit(state.BlockUpdateEvent{X: 2, Y: 64, Z: 2, StateID: 0})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.BreakBlock(ctx, protocol.BlockPos{X: 2, Y: 64, Z: 2}, BreakOptions{Creative: true}); err != nil {
		t.Fatalf("BreakBlock: %v", err)
	}
}

func runSuccessfulPlaceForLogTest(t *testing.T, c *Client) {
	t.Helper()
	addFlatActionChunk(c)
	c.stateMu.Lock()
	c.player.X = 5.5
	c.player.Z = 5.5
	c.stateMu.Unlock()
	c.trackInventorySlot(36, protocol.ItemStack{Present: true, ItemID: 1, Count: 4})
	c.trackSelectedHotbarSlot(0)
	go func() {
		time.Sleep(25 * time.Millisecond)
		c.world.SetBlock(2, 64, 2, 1)
		c.bus.Emit(state.BlockUpdateEvent{X: 2, Y: 64, Z: 2, StateID: 1})
		c.bus.Emit(state.InventorySlotEvent{WindowID: 0, Slot: 36, Item: protocol.ItemStack{Present: true, ItemID: 1, Count: 3}})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := c.PlaceBlockSurvival(ctx, protocol.BlockPos{X: 2, Y: 64, Z: 2}, protocol.DirectionUp); err != nil {
		t.Fatalf("PlaceBlockSurvival: %v", err)
	}
}

func readyGravityLogClient(t *testing.T, debug bool) *Client {
	t.Helper()
	c := NewClient(Options{Debug: debug})
	ch := world.NewChunk(0, 0)
	c.World().AddChunk(ch)
	c.stateMu.Lock()
	c.player.X = 8.5
	c.player.Y = 20.0
	c.player.Z = 8.5
	c.player.OnGround = true
	c.stateMu.Unlock()

	clientConn, serverConn := net.Pipe()
	c.conn = feastconn.New(clientConn)
	t.Cleanup(func() {
		_ = clientConn.Close()
		_ = serverConn.Close()
	})
	go func() {
		s := feastconn.New(serverConn)
		for {
			if _, err := s.ReadPacket(); err != nil {
				return
			}
		}
	}()
	return c
}

func TestNoBreakLogsWhenDebugFalse(t *testing.T) {
	c, _ := readyCoreActionClient(t)
	out := captureStdout(t, func() {
		runSuccessfulBreakForLogTest(t, c)
	})
	if out != "" {
		t.Fatalf("expected no stdout with debug=false, got %q", out)
	}
}

func TestBreakLogsWhenDebugTrue(t *testing.T) {
	c, _ := readyCoreActionClient(t)
	c.opts.Debug = true
	out := captureStdout(t, func() {
		runSuccessfulBreakForLogTest(t, c)
	})
	if !strings.Contains(out, "[break]") || !strings.Contains(out, "[break-debug]") {
		t.Fatalf("expected break debug logs, got %q", out)
	}
}

func TestNoPlaceLogsWhenDebugFalse(t *testing.T) {
	c, _ := readyCoreActionClient(t)
	out := captureStdout(t, func() {
		runSuccessfulPlaceForLogTest(t, c)
	})
	if out != "" {
		t.Fatalf("expected no stdout with debug=false, got %q", out)
	}
}

func TestPlaceLogsWhenDebugTrue(t *testing.T) {
	c, _ := readyCoreActionClient(t)
	c.opts.Debug = true
	out := captureStdout(t, func() {
		runSuccessfulPlaceForLogTest(t, c)
	})
	if !strings.Contains(out, "[place-survival]") {
		t.Fatalf("expected place survival debug logs, got %q", out)
	}
}

func TestNoLookLogsWhenDebugFalse(t *testing.T) {
	c, _ := readyCoreActionClient(t)
	out := captureStdout(t, func() {
		runSuccessfulBreakForLogTest(t, c)
	})
	if strings.Contains(out, "[look]") {
		t.Fatalf("expected no look stdout with debug=false, got %q", out)
	}
}

func TestNoGravityLogsWhenDebugFalse(t *testing.T) {
	c := readyGravityLogClient(t, false)
	out := captureStdout(t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_ = c.WaitForGround(ctx)
	})
	if out != "" {
		t.Fatalf("expected no gravity stdout with debug=false, got %q", out)
	}
}

func TestGravityLogsWhenDebugTrue(t *testing.T) {
	c := readyGravityLogClient(t, true)
	out := captureStdout(t, func() {
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_ = c.WaitForGround(ctx)
	})
	if !strings.Contains(out, "[gravity]") {
		t.Fatalf("expected gravity debug logs, got %q", out)
	}
}
