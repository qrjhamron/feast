// place_block_survival places a block from the hotbar in survival mode.
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
	"math"
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

	// Look for an air block to place into.
	pos := bot.Position()
	originX := int(math.Floor(pos.X))
	originZ := int(math.Floor(pos.Z))

	var target *feast.BlockPos
	for r := 2; r <= 6 && target == nil; r++ {
		for dx := -r; dx <= r && target == nil; dx++ {
			for dz := -r; dz <= r && target == nil; dz++ {
				wx := originX + dx
				wz := originZ + dz
				surfY := bot.World().GetSurfaceY(wx, wz)
				if surfY < 0 {
					continue
				}
				air, err := bot.World().GetBlock(wx, surfY+1, wz)
				if err != nil || air.Name != "air" {
					continue
				}
				sup, err := bot.World().GetBlock(wx, surfY, wz)
				if err != nil || !sup.Solid {
					continue
				}
				bp := feast.BlockPos{X: int32(wx), Y: int32(surfY + 1), Z: int32(wz)}
				target = &bp
			}
		}
	}

	if target == nil {
		fmt.Println("[place_survival] no suitable air target found nearby")
		os.Exit(1)
	}

	fmt.Printf("[place_survival] placing at (%d, %d, %d)\n", target.X, target.Y, target.Z)

	if err := bot.PlaceBlockSurvival(ctx, *target, feast.FaceUp); err != nil {
		fmt.Printf("[place_survival] error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("[place_survival] done")
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
