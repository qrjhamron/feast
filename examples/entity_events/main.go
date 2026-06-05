// entity_events listens for entity spawn, move, and remove events.
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

	bot.OnEntitySpawn(func(e feast.EntityEvent) {
		ent := e.Entity
		fmt.Printf("[entity] spawn id=%d type=%s pos=(%.1f,%.1f,%.1f)\n",
			ent.ID, ent.Type, ent.X, ent.Y, ent.Z)
	})

	bot.OnEntityMove(func(e feast.EntityEvent) {
		ent := e.Entity
		fmt.Printf("[entity] move  id=%d pos=(%.1f,%.1f,%.1f)\n",
			ent.ID, ent.X, ent.Y, ent.Z)
	})

	bot.OnEntityRemove(func(e feast.EntityEvent) {
		fmt.Printf("[entity] remove id=%d\n", e.Entity.ID)
	})

	bot.OnDisconnect(func(err error) {
		if err != nil {
			fmt.Printf("[disconnect] %v\n", err)
		}
		cancel()
	})

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sig:
	case <-ctx.Done():
	}
	fmt.Println("[entity_events] shutting down")
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
