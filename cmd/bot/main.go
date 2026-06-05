// cmd/bot is a minimal demo bot that connects to a Minecraft server,
// prints events, echoes stdin as chat, and shuts down cleanly.
//
// Configuration is via environment variables:
//
//	MC_HOST      server hostname (default: localhost)
//	MC_PORT      server port (default: 25565)
//	MC_USERNAME  player name (default: FeastGoBot)
package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/qrjhamron/feast/pkg/feast"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	client := feast.NewClient(feast.Options{
		Host:     getenv("MC_HOST", "localhost"),
		Port:     getenv("MC_PORT", "25565"),
		Username: getenv("MC_USERNAME", "FeastGoBot"),
	})

	// Register event hooks before connecting.
	client.OnChat(func(e feast.ChatEvent) {
		fmt.Printf("[chat] %s: %s\n", e.Sender, e.Message)
	})
	client.OnHealth(func(e feast.HealthEvent) {
		fmt.Printf("[health] health=%.1f food=%d\n", e.Health, e.Food)
	})
	client.OnDisconnect(func(err error) {
		if err != nil {
			fmt.Printf("[disconnect] err=%v\n", err)
		} else {
			fmt.Println("[disconnect] clean")
		}
	})
	client.OnError(func(err error) {
		fmt.Printf("[error] %v\n", err)
	})

	if err := client.Connect(); err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			fmt.Println("[bot] server not reachable")
			os.Exit(0)
		}
		log.Fatalf("connect failed: %v", err)
	}
	defer client.Disconnect()

	fmt.Println("[bot] connected – waiting for ready...")
	waitCtx, waitCancel := context.WithTimeout(ctx, 15*time.Second)
	if err := client.WaitUntilReady(waitCtx); err != nil {
		waitCancel()
		fmt.Printf("[bot] ready timeout: %v\n", err)
	} else {
		waitCancel()
		pos := client.Position()
		fmt.Printf("[bot] ready at (%.2f, %.2f, %.2f)\n", pos.X, pos.Y, pos.Z)
	}

	// Read chat from stdin.
	go func() {
		s := bufio.NewScanner(os.Stdin)
		for s.Scan() {
			line := strings.TrimSpace(s.Text())
			if line == "" {
				continue
			}
			if line == "!quit" {
				cancel()
				return
			}
			if err := client.Chat(line); err != nil {
				fmt.Printf("[chat-send] error: %v\n", err)
			}
		}
	}()

	// Wait for OS signal or context cancellation.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sigCh:
	case <-ctx.Done():
	}
	fmt.Println("[bot] shutting down")
}
