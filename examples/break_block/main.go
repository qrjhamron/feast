// break_block finds the nearest breakable block and breaks it.
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

	time.Sleep(2 * time.Second)

	// Find a nearby dirt or grass block.
	hit, ok := bot.FindNearestBlock("dirt", 8)
	if !ok {
		hit, ok = bot.FindNearestBlock("grass_block", 8)
	}
	if !ok {
		fmt.Println("[break] no breakable block found nearby")
		os.Exit(1)
	}

	fmt.Printf("[break] breaking %s at (%d, %d, %d)\n", hit.Block.Name, hit.X, hit.Y, hit.Z)

	pos := feast.BlockPos{X: int32(hit.X), Y: int32(hit.Y), Z: int32(hit.Z)}
	if err := bot.BreakBlock(ctx, pos); err != nil {
		fmt.Printf("[break] error: %v\n", err)
		os.Exit(1)
	}

	// Wait for block update confirmation.
	done := make(chan struct{}, 1)
	bot.OnBlockUpdate(func(e feast.BlockUpdateEvent) {
		if e.X == int32(hit.X) && e.Y == int32(hit.Y) && e.Z == int32(hit.Z) {
			select {
			case done <- struct{}{}:
			default:
			}
		}
	})

	select {
	case <-done:
		fmt.Println("[break] block update received – done")
	case <-time.After(5 * time.Second):
		fmt.Println("[break] timeout waiting for block update")
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
