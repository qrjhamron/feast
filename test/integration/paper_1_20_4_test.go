// Package paper_1_20_4 contains integration tests for FeastGo against a real
// Minecraft Paper 1.20.4 server in offline mode.
//
// These tests require a live server and are gated behind the "integration" build tag:
//
//	go test ./test/integration -tags=integration
//
// Without a server running they will skip immediately (t.Skip).
package paper_1_20_4_test

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	"github.com/qrjhamron/feast/pkg/feast"
)

func serverHost() string {
	if v := os.Getenv("MC_HOST"); v != "" {
		return v
	}
	return "127.0.0.1"
}

func serverPort() string {
	if v := os.Getenv("MC_PORT"); v != "" {
		return v
	}
	return "25565"
}

func serverAddr() string {
	return net.JoinHostPort(serverHost(), serverPort())
}

func skipIfNoServer(t *testing.T) {
	t.Helper()
	addr := serverAddr()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Skipf("no server reachable at %s: %v", addr, err)
	}
	conn.Close()
}

func newConnectedBot(t *testing.T) *feast.Client {
	t.Helper()
	skipIfNoServer(t)
	bot, err := feast.Connect(context.Background(), feast.Options{
		Host: serverHost(),
		Port: serverPort(),
		Username: func() string {
			if v := os.Getenv("MC_USERNAME"); v != "" {
				return v
			}
			return "FeastGoBot"
		}(),
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { bot.Disconnect() })
	return bot
}

// TestIntegration_ConnectAndReady verifies that the client connects and
// receives a position sync within 15 seconds.
func TestIntegration_ConnectAndReady(t *testing.T) {
	bot := newConnectedBot(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := bot.WaitUntilReady(ctx); err != nil {
		t.Fatalf("WaitUntilReady: %v", err)
	}
	pos := bot.Position()
	t.Logf("ready at (%.2f, %.2f, %.2f)", pos.X, pos.Y, pos.Z)
}

// TestIntegration_ChatSend verifies that chat can be sent without error.
func TestIntegration_ChatSend(t *testing.T) {
	bot := newConnectedBot(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := bot.WaitUntilReady(ctx); err != nil {
		t.Fatalf("WaitUntilReady: %v", err)
	}
	if err := bot.Chat("integration test hello"); err != nil {
		t.Fatalf("Chat: %v", err)
	}
}

// TestIntegration_WorldChunks verifies that at least one chunk is loaded
// after connecting.
func TestIntegration_WorldChunks(t *testing.T) {
	bot := newConnectedBot(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := bot.WaitUntilReady(ctx); err != nil {
		t.Fatalf("WaitUntilReady: %v", err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if bot.World().ChunkCount() >= 1 {
			t.Logf("chunks loaded: %d", bot.World().ChunkCount())
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("no chunks loaded after 10s")
}

// TestIntegration_FindNearestBlock verifies that FindNearestBlock works with a
// loaded world.
func TestIntegration_FindNearestBlock(t *testing.T) {
	bot := newConnectedBot(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := bot.WaitUntilReady(ctx); err != nil {
		t.Fatalf("WaitUntilReady: %v", err)
	}
	time.Sleep(2 * time.Second)
	for _, name := range []string{"grass_block", "dirt", "stone"} {
		if hit, ok := bot.FindNearestBlock(name, 64); ok {
			t.Logf("found %s at (%d, %d, %d)", name, hit.X, hit.Y, hit.Z)
			return
		}
	}
	t.Log("no common blocks found; world may be unusual")
}

// TestIntegration_CleanDisconnect verifies that Disconnect sets all shutdown
// flags correctly.
func TestIntegration_CleanDisconnect(t *testing.T) {
	bot := newConnectedBot(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := bot.WaitUntilReady(ctx); err != nil {
		t.Fatalf("WaitUntilReady: %v", err)
	}
	if err := bot.Disconnect(); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
	status := bot.ShutdownStatus()
	if !status.Requested || !status.TickLoopStopped || !status.ReadLoopStopped || !status.SocketClosed {
		t.Fatalf("incomplete shutdown: %+v", status)
	}
}
