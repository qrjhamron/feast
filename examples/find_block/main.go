// find_block searches for the nearest grass_block within 64 blocks.
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
	"time"

	"github.com/qrjhamron/feast/pkg/feast"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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

	if err := bot.WaitUntilReady(ctx); err != nil {
		log.Fatal("ready:", err)
	}

	// Give the world a moment to load more chunks.
	time.Sleep(2 * time.Second)

	blockName := "grass_block"
	hit, ok := bot.FindNearestBlock(blockName, 64)
	if !ok {
		fmt.Printf("[find_block] %s not found within 64 blocks\n", blockName)
		os.Exit(1)
	}

	fmt.Printf("[find_block] found %s at (%d, %d, %d) distance=%.2f\n",
		blockName, hit.X, hit.Y, hit.Z, hit.Distance)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
