// basic_join connects to a Minecraft server and prints ready state.
//
// Environment variables:
//
//	MC_HOST      server hostname (default: 127.0.0.1)
//	MC_PORT      server port (default: 25565)
//	MC_USERNAME  player name (default: FeastGoBot)
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/qrjhamron/feast/pkg/feast"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bot, err := feast.Connect(ctx, feast.Options{
		Host:     getenv("MC_HOST", "127.0.0.1"),
		Port:     getenv("MC_PORT", "25565"),
		Username: getenv("MC_USERNAME", "FeastGoBot"),
	})
	if err != nil {
		log.Fatal("connect:", err)
	}
	defer bot.Disconnect()

	// Wait up to 15 seconds for position sync.
	waitCtx, waitCancel := context.WithTimeout(ctx, 15*time.Second)
	defer waitCancel()
	if err := bot.WaitUntilReady(waitCtx); err != nil {
		log.Fatal("ready:", err)
	}

	pos := bot.Position()
	fmt.Printf("[basic_join] ready at (%.2f, %.2f, %.2f)\n", pos.X, pos.Y, pos.Z)
	fmt.Printf("[basic_join] health=%.1f food=%d\n", bot.Health(), bot.Food())
	fmt.Printf("[basic_join] chunks_loaded=%d\n", bot.World().ChunkCount())

	// Stay connected until signal.
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	fmt.Println("[basic_join] shutting down")
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
