package feast

import (
	"context"
	"net"
	"testing"
	"time"

	feastconn "github.com/qrjhamron/feast/pkg/conn"
	"github.com/qrjhamron/feast/pkg/state"
)

func TestDisconnectWhileTickLoopActive(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()

	c := NewClient(Options{})
	c.conn = feastconn.New(clientConn)

	// Simulate starting a tick loop goroutine registered in c.wg
	c.stopCh = make(chan struct{})
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-c.stopCh:
				return
			case <-ticker.C:
				// tick
			}
		}
	}()

	// Disconnect should close stopCh and wait for goroutine to exit
	if err := c.Disconnect(); err != nil {
		t.Fatalf("Disconnect failed: %v", err)
	}

	status := c.ShutdownStatus()
	if !status.TickLoopStopped {
		t.Errorf("expected TickLoopStopped to be true")
	}
}

func TestDisconnectWhileNavExecutorActive(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()

	c := NewClient(Options{})
	c.conn = feastconn.New(clientConn)

	// Simulate active navigation
	c.stopCh = make(chan struct{})
	c.moving = true
	ctx, cancel := context.WithCancel(context.Background())
	c.navCtx = ctx
	c.navCancel = cancel

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		select {
		case <-ctx.Done():
			c.moving = false
			return
		case <-c.stopCh:
			c.moving = false
			return
		}
	}()

	// Disconnect should cancel navCtx and wait
	if err := c.Disconnect(); err != nil {
		t.Fatalf("Disconnect failed: %v", err)
	}

	status := c.ShutdownStatus()
	if !status.NavLoopStopped {
		t.Errorf("expected NavLoopStopped to be true")
	}
	if c.moving {
		t.Errorf("expected moving to be false after disconnect")
	}
}

func TestDisconnectWhileWaitingForBlockUpdate(t *testing.T) {
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()

	c := NewClient(Options{})
	c.conn = feastconn.New(clientConn)
	c.stopCh = make(chan struct{})

	// Simulate goroutine waiting for block update
	updateCh := make(chan state.BlockUpdateEvent, 1)
	done := make(chan bool)

	go func() {
		select {
		case <-updateCh:
			done <- true
		case <-c.stopCh:
			done <- false
		case <-time.After(500 * time.Millisecond):
			done <- false
		}
	}()

	// Disconnect should trigger c.stopCh
	if err := c.Disconnect(); err != nil {
		t.Fatalf("Disconnect failed: %v", err)
	}

	select {
	case reachedUpdate := <-done:
		if reachedUpdate {
			t.Errorf("should not have gotten update, client was disconnected")
		}
	case <-time.After(200 * time.Millisecond):
		t.Errorf("timeout waiting for block update wait exit")
	}
}
