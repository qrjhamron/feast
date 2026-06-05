// chat_echo listens for chat messages and echoes them back with a prefix.
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

	bot.OnChat(func(e feast.ChatEvent) {
		fmt.Printf("[chat] %s: %s\n", e.Sender, e.Message)
		// Echo messages that start with "!echo ".
		if len(e.Message) > 6 && e.Message[:6] == "!echo " {
			reply := e.Message[6:]
			if err := bot.Chat(reply); err != nil {
				fmt.Printf("[chat-echo] send error: %v\n", err)
			}
		}
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
	fmt.Println("[chat_echo] shutting down")
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
