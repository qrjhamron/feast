// avoid_entities prints a warning whenever a nearby entity is detected
// and lists all entities within 10 blocks of the bot.
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

	if err := bot.WaitUntilReady(ctx); err != nil {
		log.Fatal("ready:", err)
	}

	// Poll nearby entities every second.
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				pos := bot.Position()
				nearby := bot.Entities().Nearby(pos.X, pos.Y, pos.Z, 10)
				if len(nearby) > 0 {
					fmt.Printf("[avoid] %d entities within 10 blocks:\n", len(nearby))
					for _, e := range nearby {
						fmt.Printf("  id=%d type=%-12s pos=(%.1f,%.1f,%.1f)\n",
							e.ID, e.Type, e.X, e.Y, e.Z)
					}
				}
			}
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sig:
	case <-ctx.Done():
	}
	fmt.Println("[avoid_entities] shutting down")
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
