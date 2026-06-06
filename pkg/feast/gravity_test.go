package feast

import (
	"context"
	"net"
	"testing"
	"time"

	feastconn "github.com/qrjhamron/feast/pkg/conn"
	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/world"
)

// TestSupportBelowBotRemovedTriggersGravity tests that removing the support below the bot triggers gravity
func TestSupportBelowBotRemovedTriggersGravity(t *testing.T) {
	c := NewClient(Options{})
	ch := world.NewChunk(0, 0)
	// Make a 3x3 of solid stone at Y=19
	for x := -1; x <= 1; x++ {
		for z := -1; z <= 1; z++ {
			ch.SetBlock(x+8, 19, z+8, world.BlockState{Name: "stone", Solid: true})
		}
	}
	c.World().AddChunk(ch)

	// Bot is at X=8.5, Y=20, Z=8.5 (standing on stone block at 8, 19, 8)
	c.stateMu.Lock()
	c.player.X = 8.5
	c.player.Y = 20.0
	c.player.Z = 8.5
	c.player.OnGround = true
	c.stateMu.Unlock()

	// Initial check: support should remain
	if c.IsSupportLost() {
		t.Fatal("expected support to NOT be lost initially")
	}

	// Remove support (make (8, 19, 8) air)
	ch.SetBlock(8, 19, 8, world.AirBlockState)

	// In XZ radius 0.3 around 8.5, 8.5:
	// minX = floor(8.2) = 8
	// maxX = floor(8.8) = 8
	// minZ = floor(8.2) = 8
	// maxZ = floor(8.8) = 8
	// So footprint check only covers (8, 19, 8)
	if !c.IsSupportLost() {
		t.Fatal("expected support to be lost after replacing with air")
	}

	// Verify that gravity loop executes and starts falling
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	c.conn = feastconn.New(clientConn)
	c.stopMu.Lock()
	c.stopCh = make(chan struct{})
	c.stopped = false
	c.stopMu.Unlock()

	// Mock server side reading position packets
	errCh := make(chan error, 1)
	go func() {
		s := feastconn.New(serverConn)
		// Read one position packet sent during the fall
		raw, err := s.ReadPacket()
		if err != nil {
			errCh <- err
			return
		}
		var pkt protocol.PlayServerboundSetPlayerPositionAndRotationPacket
		if err := unmarshalRaw(&pkt, raw); err != nil {
			errCh <- err
			return
		}
		errCh <- nil

		// Drain any subsequent writes to prevent blocking
		for {
			_, err := s.ReadPacket()
			if err != nil {
				return
			}
		}
	}()

	// Run WaitForGround with a short timeout to see if it ticks
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_ = c.WaitForGround(ctx)

	// Should have sent a packet
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("failed to read packet: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timeout waiting for position packet to be sent")
	}
}

// TestFallStopsAtSolidGround tests that a fall terminates upon reaching solid ground
func TestFallStopsAtSolidGround(t *testing.T) {
	c := NewClient(Options{})
	ch := world.NewChunk(0, 0)
	// Set (8, 19, 8) as air, but (8, 17, 8) as solid stone
	ch.SetBlock(8, 19, 8, world.AirBlockState)
	ch.SetBlock(8, 18, 8, world.AirBlockState)
	ch.SetBlock(8, 17, 8, world.BlockState{Name: "stone", Solid: true})
	c.World().AddChunk(ch)

	c.stateMu.Lock()
	c.player.X = 8.5
	c.player.Y = 20.0 // standing level would be 20.0, falling below
	c.player.Z = 8.5
	c.player.OnGround = false
	c.stateMu.Unlock()

	// Make a mock pipe connection to absorb packets
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	c.conn = feastconn.New(clientConn)
	c.stopMu.Lock()
	c.stopCh = make(chan struct{})
	c.stopped = false
	c.stopMu.Unlock()

	go func() {
		s := feastconn.New(serverConn)
		for {
			_, err := s.ReadPacket()
			if err != nil {
				return
			}
		}
	}()

	// Run WaitForGround
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := c.WaitForGround(ctx)
	if err != nil {
		t.Fatalf("WaitForGround failed: %v", err)
	}

	st := c.PlayerState()
	// Foot should snap to Y=18.0 (since solid stone block is at 17, so standing floor Y is 18.0)
	if st.Y != 18.0 {
		t.Fatalf("expected bot to land at Y=18.0, got %v", st.Y)
	}
	if !st.OnGround {
		t.Fatal("expected bot to be on ground")
	}
}

// TestNoInfiniteFallLoop tests that a fall terminates and does not loop infinitely
func TestNoInfiniteFallLoop(t *testing.T) {
	c := NewClient(Options{})
	// No chunks loaded -> isSupportLostForState returns false (unloaded is treated as solid support)
	// Let's load a chunk but with all air to simulate endless fall
	ch := world.NewChunk(0, 0)
	c.World().AddChunk(ch)

	c.stateMu.Lock()
	c.player.X = 8.5
	c.player.Y = 20.0
	c.player.Z = 8.5
	c.player.OnGround = true
	c.stateMu.Unlock()

	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	c.conn = feastconn.New(clientConn)
	c.stopMu.Lock()
	c.stopCh = make(chan struct{})
	c.stopped = false
	c.stopMu.Unlock()

	go func() {
		s := feastconn.New(serverConn)
		for {
			_, err := s.ReadPacket()
			if err != nil {
				return
			}
		}
	}()

	// WaitForGround should hit the timeout or ctx cancellation and return error
	// Let's pass a context with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	err := c.WaitForGround(ctx)
	if err == nil {
		t.Fatal("expected error from context deadline/timeout, got nil")
	}
}

// TestNoFallIfSupportRemains tests that gravity is not applied if any footprint support block remains solid
func TestNoFallIfSupportRemains(t *testing.T) {
	c := NewClient(Options{})
	ch := world.NewChunk(0, 0)
	// Bot X=8.2, Z=8.2
	// minX = floor(7.9) = 7
	// maxX = floor(8.5) = 8
	// minZ = floor(7.9) = 7
	// maxZ = floor(8.5) = 8
	// Support block directly under feet (8, 19, 8) is air.
	// But support block at (7, 19, 7) is solid stone.
	ch.SetBlock(8, 19, 8, world.AirBlockState)
	ch.SetBlock(7, 19, 7, world.BlockState{Name: "stone", Solid: true})
	// Keep other blocks inside footprint as air or loaded
	ch.SetBlock(7, 19, 8, world.AirBlockState)
	ch.SetBlock(8, 19, 7, world.AirBlockState)
	c.World().AddChunk(ch)

	c.stateMu.Lock()
	c.player.X = 8.2
	c.player.Y = 20.0
	c.player.Z = 8.2
	c.player.OnGround = true
	c.stateMu.Unlock()

	// Direct check
	isAirDirect := c.world.IsReplaceable(world.BlockPos{X: 8, Y: 19, Z: 8})
	if !isAirDirect {
		t.Fatal("expected block directly under feet to be air/replaceable")
	}

	// But because of XZ radius 0.3 footprint, (7, 19, 7) is solid stone.
	// So support remains!
	if c.IsSupportLost() {
		t.Fatal("expected support to remain because (7, 19, 7) is solid")
	}
}

// TestNegativeCoordinateSupportCheck tests that applyGravityTick snaps Y correctly using math.Floor for negative coordinates
func TestNegativeCoordinateSupportCheck(t *testing.T) {
	st := PlayerState{Y: -10.5, VelocityY: -0.5, OnGround: false}
	next := applyGravityTick(st, false)
	if next.Y != -11.0 {
		t.Fatalf("expected Y snapped to floor (-11.0), got %v", next.Y)
	}
	if next.VelocityY != 0 {
		t.Fatalf("expected zero vertical velocity, got %v", next.VelocityY)
	}
	if !next.OnGround {
		t.Fatal("expected on-ground state")
	}
}
