// place_block_creative places a stone block in creative mode above the bot.
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

	pos := bot.Position()
	// Place above and slightly away from the bot.
	target := feast.BlockPos{
		X: int32(pos.X) + 2,
		Y: int32(pos.Y),
		Z: int32(pos.Z),
	}

	fmt.Printf("[place_creative] placing stone at (%d, %d, %d)\n", target.X, target.Y, target.Z)

	if err := bot.PlaceBlockCreative(ctx, target, feast.FaceUp, "stone"); err != nil {
		fmt.Printf("[place_creative] error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[place_creative] packet sent")

	// Wait for block update.
	done := make(chan struct{}, 1)
	bot.OnBlockUpdate(func(e feast.BlockUpdateEvent) {
		if e.X == target.X && e.Y == target.Y && e.Z == target.Z {
			select {
			case done <- struct{}{}:
			default:
			}
		}
	})

	select {
	case <-done:
		state, err := bot.World().GetBlock(int(target.X), int(target.Y), int(target.Z))
		if err == nil {
			fmt.Printf("[place_creative] block is now: %s\n", state.Name)
		}
	case <-time.After(5 * time.Second):
		fmt.Println("[place_creative] timeout waiting for block update")
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
