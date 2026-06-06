package feast

import (
	"context"
	"io"
	"log"
	"net"
	"sync"
	"testing"
	"time"

	feastconn "github.com/qrjhamron/feast/pkg/conn"
	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/state"
)

// connectLoopbackForTest drives the minimal login/configuration handshake over
// an in-memory pipe and returns a connected client in the Play state. The
// returned cancel closes the server side of the pipe (simulating the server
// dropping the connection), which drives the read loop to EOF.
func connectLoopbackForTest(t *testing.T) (c *Client, dropServer func()) {
	t.Helper()
	clientConn, serverConn := net.Pipe()

	c = NewClient(Options{Host: "localhost", Port: "25565", Username: "FeastBot"})
	c.dialFunc = func(_, _ string) (net.Conn, error) { return clientConn, nil }

	ready := make(chan struct{})
	go func() {
		s := feastconn.New(serverConn)
		_, _ = s.ReadPacket() // handshake
		_, _ = s.ReadPacket() // login start
		_ = s.WritePacket(&protocol.LoginClientboundLoginSuccessPacket{UUID: [16]byte{1}, Username: "FeastBot"})
		_, _ = s.ReadPacket() // login ack
		_, _ = s.ReadPacket() // client information
		_ = s.WritePacket(&protocol.ConfigClientboundFinishPacket{})
		_, _ = s.ReadPacket() // ack finish
		close(ready)
	}()

	if err := c.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("handshake timed out")
	}
	return c, func() { _ = serverConn.Close() }
}

// TestDisconnectFromEventHandlerDoesNotDeadlock verifies that calling
// Disconnect from within an event handler — which runs synchronously on the
// client-managed read-loop goroutine — does not deadlock on c.wg.Wait.
func TestDisconnectFromEventHandlerDoesNotDeadlock(t *testing.T) {
	prevLogOut := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(prevLogOut)

	c, dropServer := connectLoopbackForTest(t)

	handlerReturned := make(chan struct{})
	var once sync.Once
	c.OnDisconnect(func(error) {
		// This handler runs on the read-loop goroutine. Calling Disconnect
		// here must not block forever waiting for that very goroutine.
		_ = c.Disconnect()
		once.Do(func() { close(handlerReturned) })
	})

	// Drop the server to drive the read loop to EOF -> disconnect event.
	dropServer()

	select {
	case <-handlerReturned:
		// Good: the re-entrant Disconnect returned promptly.
	case <-time.After(3 * time.Second):
		t.Fatal("Disconnect from event handler deadlocked")
	}

	// Teardown should still complete on its own once the loops drain. A
	// subsequent Disconnect from the test (non-managed) goroutine must observe
	// a fully completed shutdown.
	done := make(chan error, 1)
	go func() { done <- c.Disconnect() }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("background teardown did not complete after handler Disconnect")
	}

	status := c.ShutdownStatus()
	if !status.Requested || !status.ReadLoopStopped || !status.SocketClosed {
		t.Fatalf("incomplete shutdown status after handler-triggered disconnect: %+v", status)
	}
}

// TestDisconnectFromChunkHandlerDoesNotDeadlock exercises the same re-entrancy
// guard from a handler that runs on a chunk-ingest worker goroutine.
func TestDisconnectFromChunkHandlerDoesNotDeadlock(t *testing.T) {
	prevLogOut := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(prevLogOut)

	c := NewClient(Options{})
	c.stopCh = make(chan struct{})
	c.runtimeCtx, c.runtimeCancel = context.WithCancel(context.Background())

	done := make(chan struct{})
	// Simulate a chunk-ingest worker: it is tracked by c.wg and marked as a
	// managed goroutine, then emits an event whose handler calls Disconnect.
	c.bus.On("chunk_load", func(state.Event) {
		_ = c.Disconnect()
		close(done)
	})

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer c.markManagedGoroutine()()
		c.bus.Emit(state.ChunkLoadEvent{ChunkX: 0, ChunkZ: 0})
	}()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Disconnect from chunk-handler goroutine deadlocked")
	}
}
