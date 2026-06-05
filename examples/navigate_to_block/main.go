// navigate_to_block finds the nearest grass_block and navigates to it.
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
	"github.com/qrjhamron/feast/pkg/nav/goal"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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

	hit, ok := bot.FindNearestBlock("grass_block", 64)
	if !ok {
		fmt.Println("[nav] no grass_block found within 64 blocks")
		os.Exit(1)
	}

	fmt.Printf("[nav] navigating to grass_block at (%d, %d, %d)\n", hit.X, hit.Y, hit.Z)

	// Navigate to the block above the grass_block (the standing position).
	g := goal.NewGoalBlock(hit.X, hit.Y+1, hit.Z)
	if err := bot.NavigateTo(ctx, g); err != nil {
		fmt.Printf("[nav] failed: %v\n", err)
		os.Exit(1)
	}

	pos := bot.Position()
	fmt.Printf("[nav] arrived at (%.2f, %.2f, %.2f)\n", pos.X, pos.Y, pos.Z)
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
