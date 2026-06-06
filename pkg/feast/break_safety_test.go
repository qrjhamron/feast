package feast

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/state"
	"github.com/qrjhamron/feast/pkg/world"
)

func TestBreakUnderFeetFails(t *testing.T) {
	c, _ := readyCoreActionClient(t)
	addFlatActionChunk(c)

	// Bot is at (0.5, 64.0, 0.5)
	c.stateMu.Lock()
	c.player.X = 0.5
	c.player.Y = 64.0
	c.player.Z = 0.5
	c.stateMu.Unlock()

	// Directly under feet
	err := c.BreakBlock(context.Background(), protocol.BlockPos{X: 0, Y: 63, Z: 0})
	if !errors.Is(err, ErrBreakTargetUnsafe) {
		t.Fatalf("expected ErrBreakTargetUnsafe for under feet, got %v", err)
	}

	// In footprint support (overlapping X/Z but not floor center)
	c.stateMu.Lock()
	c.player.X = 0.9
	c.player.Y = 64.0
	c.player.Z = 0.5
	c.stateMu.Unlock()

	// Footprint bounds for X: [0.6, 1.2]
	// Block pos (1, 63, 0) X bounds: [1.0, 2.0]
	// Intersects! But floor(X) = 0. So it is not "directly under feet" center-wise, but is in footprint support.
	err = c.BreakBlock(context.Background(), protocol.BlockPos{X: 1, Y: 63, Z: 0})
	if !errors.Is(err, ErrBreakTargetUnsafe) {
		t.Fatalf("expected ErrBreakTargetUnsafe for footprint support, got %v", err)
	}
}

func TestBreakSideBlockSucceeds(t *testing.T) {
	c, _ := readyCoreActionClient(t)
	addFlatActionChunk(c)

	c.stateMu.Lock()
	c.player.X = 0.5
	c.player.Y = 64.0
	c.player.Z = 0.5
	c.stateMu.Unlock()

	c.world.SetBlock(2, 64, 2, 1) // stone

	go func() {
		time.Sleep(50 * time.Millisecond)
		c.world.SetBlock(2, 64, 2, 0) // air
		c.bus.Emit(state.BlockUpdateEvent{X: 2, Y: 64, Z: 2, StateID: 0})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := c.BreakBlock(ctx, protocol.BlockPos{X: 2, Y: 64, Z: 2}, BreakOptions{Creative: true})
	if err != nil {
		t.Fatalf("expected BreakBlock to succeed, got %v", err)
	}
}

func TestBreakUnloadedChunkFails(t *testing.T) {
	c, _ := readyCoreActionClient(t)

	err := c.BreakBlock(context.Background(), protocol.BlockPos{X: 100, Y: 64, Z: 100})
	if !errors.Is(err, ErrChunkNotLoaded) {
		t.Fatalf("expected ErrChunkNotLoaded, got %v", err)
	}
}

func TestBreakOutOfReachFails(t *testing.T) {
	c, _ := readyCoreActionClient(t)
	addFlatActionChunk(c)

	c.stateMu.Lock()
	c.player.X = 0.5
	c.player.Y = 64.0
	c.player.Z = 0.5
	c.stateMu.Unlock()

	c.world.SetBlock(10, 64, 10, 1) // stone (loaded but distance ~14 blocks)

	err := c.BreakBlock(context.Background(), protocol.BlockPos{X: 10, Y: 64, Z: 10})
	if !errors.Is(err, ErrBreakOutOfReach) {
		t.Fatalf("expected ErrBreakOutOfReach, got %v", err)
	}
}

func addNegativeFlatActionChunk(c *Client) {
	ch := world.NewChunk(-1, -1)
	for x := 0; x < world.ChunkWidth; x++ {
		for z := 0; z < world.ChunkDepth; z++ {
			ch.SetBlock(x, 63, z, world.BlockState{Name: "stone", Solid: true, ID: 1})
			ch.SetBlock(x, 64, z, world.BlockState{Name: "air", ID: 0})
			ch.SetBlock(x, 65, z, world.BlockState{Name: "air", ID: 0})
		}
	}
	c.world.AddChunk(ch)
}

func TestBlockUpdateWaiterNegativeCoordinates(t *testing.T) {
	c, _ := readyCoreActionClient(t)
	addNegativeFlatActionChunk(c)

	c.stateMu.Lock()
	c.player.X = -0.5
	c.player.Y = 64.0
	c.player.Z = -0.5
	c.stateMu.Unlock()

	c.world.SetBlock(-2, 64, -2, 1) // stone

	go func() {
		time.Sleep(50 * time.Millisecond)
		c.world.SetBlock(-2, 64, -2, 0) // air
		c.bus.Emit(state.BlockUpdateEvent{X: -2, Y: 64, Z: -2, StateID: 0})
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := c.BreakBlock(ctx, protocol.BlockPos{X: -2, Y: 64, Z: -2}, BreakOptions{Creative: true})
	if err != nil {
		t.Fatalf("expected BreakBlock at negative coordinate to succeed, got %v", err)
	}
}
