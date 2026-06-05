package feast

import (
	"math"
	"net"
	"sync/atomic"
	"testing"
	"time"

	feastconn "github.com/user/feastgo/pkg/conn"
	"github.com/user/feastgo/pkg/protocol"
	"github.com/user/feastgo/pkg/state"
	"github.com/user/feastgo/pkg/world"
)

func TestApplyGravityTickFallsAndCapsVelocity(t *testing.T) {
	st := PlayerState{Y: 10, VelocityY: -0.95, OnGround: true}
	next := applyGravityTick(st, true)
	if next.VelocityY != -0.98 {
		t.Fatalf("expected velocity cap -0.98, got %v", next.VelocityY)
	}
	if math.Abs(next.Y-9.02) > 1e-9 {
		t.Fatalf("unexpected Y after fall: got %v want %v", next.Y, 9.02)
	}
	if next.OnGround {
		t.Fatal("expected airborne state")
	}
}

func TestApplyGravityTickSnapsToFloorOnGround(t *testing.T) {
	st := PlayerState{Y: 12.75, VelocityY: -0.5, OnGround: false}
	next := applyGravityTick(st, false)
	if next.Y != 12 {
		t.Fatalf("expected Y snapped to floor, got %v", next.Y)
	}
	if next.VelocityY != 0 {
		t.Fatalf("expected zero vertical velocity, got %v", next.VelocityY)
	}
	if !next.OnGround {
		t.Fatal("expected on-ground state")
	}
}

func TestGravityTickUsesWorldPassabilityCheck(t *testing.T) {
	c := NewClient(Options{})
	ch := world.NewChunk(0, 0)
	ch.SetBlock(0, 19, 0, world.AirBlockState)
	c.World().AddChunk(ch)
	c.stateMu.Lock()
	c.player.Y = 20
	c.player.OnGround = true
	c.stateMu.Unlock()

	c.gravityTick()
	st := c.PlayerState()

	if st.Y >= 20 {
		t.Fatalf("expected Y to decrease when below is passable, got %v", st.Y)
	}
	if st.VelocityY >= 0 {
		t.Fatalf("expected negative velocity while falling, got %v", st.VelocityY)
	}
	if st.OnGround {
		t.Fatal("expected airborne state after gravity tick")
	}
}

func TestChunkDispatchConcurrencyCapped(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	c := NewClient(Options{})
	c.conn = feastconn.New(clientConn)
	c.stopMu.Lock()
	c.stopCh = make(chan struct{})
	c.stopped = false
	c.stopMu.Unlock()

	c.bus.Reset()
	c.dispatcher = state.NewDispatcher(c.bus)

	if err := c.fsm.Transition(state.StateLogin); err != nil {
		t.Fatalf("transition login: %v", err)
	}
	if err := c.fsm.Transition(state.StateConfiguration); err != nil {
		t.Fatalf("transition configuration: %v", err)
	}
	if err := c.fsm.Transition(state.StatePlay); err != nil {
		t.Fatalf("transition play: %v", err)
	}

	started := make(chan struct{}, 64)
	release := make(chan struct{})
	var inFlight int32
	var maxInFlight int32
	if _, err := c.bus.On("chunk_load", func(e state.Event) {
		cur := atomic.AddInt32(&inFlight, 1)
		for {
			prev := atomic.LoadInt32(&maxInFlight)
			if cur <= prev {
				break
			}
			if atomic.CompareAndSwapInt32(&maxInFlight, prev, cur) {
				break
			}
		}
		started <- struct{}{}
		<-release
		atomic.AddInt32(&inFlight, -1)
	}); err != nil {
		t.Fatalf("On: %v", err)
	}

	c.wg.Add(1)
	go c.readLoop()

	writeDone := make(chan error, 1)
	go func() {
		s := feastconn.New(serverConn)
		for i := 0; i < 20; i++ {
			if err := s.WritePacket(&protocol.PlayClientboundChunkDataAndUpdateLightPacket{
				ChunkX: int32(i),
				ChunkZ: 0,
			}); err != nil {
				writeDone <- err
				return
			}
		}
		writeDone <- nil
	}()

	wantStarted := 8
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	got := 0
	for got < wantStarted {
		select {
		case <-started:
			got++
		case err := <-writeDone:
			if err != nil {
				t.Fatalf("write packets: %v", err)
			}
		case <-deadline.C:
			t.Fatalf("timeout waiting for %d concurrent chunk dispatches, got %d (maxInFlight=%d)", wantStarted, got, atomic.LoadInt32(&maxInFlight))
		}
	}

	// With the handler blocked, the chunk dispatch semaphore should cap the number
	// of concurrent dispatch goroutines reaching this handler at 8.
	select {
	case <-started:
		t.Fatalf("expected chunk dispatch to be capped at %d, but saw >%d concurrent starts", wantStarted, wantStarted)
	case <-time.After(150 * time.Millisecond):
	}

	close(release)

	close(c.stopCh)
	_ = c.conn.Close()
	c.wg.Wait()
}

type stateTransitionTestIDError struct {
	got  int32
	want int32
}

func (e stateTransitionTestIDError) Error() string {
	return "unexpected packet id in reconfiguration flow"
}
