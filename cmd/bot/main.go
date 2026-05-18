package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/user/feastgo/pkg/feast"
	"github.com/user/feastgo/pkg/state"
)

func getenv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func main() {
	debug, _ := strconv.ParseBool(getenv("FEAST_DEBUG", "false"))
	debugPackets, _ := strconv.ParseBool(getenv("FEAST_DEBUG_PACKETS", "false"))

	client := feast.NewClient(feast.Options{
		Host:         getenv("MC_HOST", "localhost"),
		Port:         getenv("MC_PORT", "25565"),
		Username:     getenv("MC_USERNAME", "FeastBot"),
		Debug:        debug,
		DebugPackets: debugPackets,
		Logger: func(e feast.LogEvent) {
			if e.Error != nil {
				fmt.Printf("[debug] %s state=%v packet=0x%02x err=%v\n", e.Message, e.State, e.PacketID, e.Error)
				return
			}
			fmt.Printf("[debug] %s state=%v packet=0x%02x\n", e.Message, e.State, e.PacketID)
		},
	})

	client.On("login", func(e state.Event) {
		le, ok := e.(state.LoginEvent)
		if !ok {
			return
		}
		fmt.Printf("[login] username=%s uuid=%s\n", le.Username, le.UUID)
	})
	client.On("play_login", func(e state.Event) {
		pe, ok := e.(state.PlayLoginEvent)
		if !ok {
			return
		}
		fmt.Printf("[play] entity_id=%d\n", pe.EntityID)
	})
	client.On("chat", func(e state.Event) {
		ce, ok := e.(state.ChatEvent)
		if !ok {
			return
		}
		fmt.Printf("[chat] %s: %s\n", ce.Sender, ce.Message)
	})
	client.On("kick", func(e state.Event) {
		ke, ok := e.(state.KickEvent)
		if !ok {
			return
		}
		fmt.Printf("[kick] %s\n", ke.Reason)
	})
	client.On("disconnect", func(e state.Event) {
		de, ok := e.(state.DisconnectEvent)
		if !ok {
			return
		}
		fmt.Printf("[disconnect] clean=%v reason=%s\n", de.Clean, de.Reason)
	})
	client.On("error", func(e state.Event) {
		ee, ok := e.(state.ErrorEvent)
		if !ok {
			return
		}
		fmt.Printf("[error] op=%s err=%v\n", ee.Op, ee.Error)
	})
	client.On("spawn", func(e state.Event) {
		se, ok := e.(state.SpawnEvent)
		if !ok {
			return
		}
		fmt.Printf("[spawn] entity_id=%d x=%.2f y=%.2f z=%.2f\n", se.EntityID, se.X, se.Y, se.Z)
	})
	client.On("position", func(e state.Event) {
		pe, ok := e.(state.PositionEvent)
		if !ok {
			return
		}
		fmt.Printf("[position] x=%.2f y=%.2f z=%.2f yaw=%.2f pitch=%.2f teleport=%d\n", pe.X, pe.Y, pe.Z, pe.Yaw, pe.Pitch, pe.TeleportID)
	})
	client.On("health", func(e state.Event) {
		he, ok := e.(state.HealthEvent)
		if !ok {
			return
		}
		fmt.Printf("[health] health=%.1f food=%d saturation=%.1f\n", he.Health, he.Food, he.Saturation)
	})
	client.On("keep_alive", func(e state.Event) {
		ke, ok := e.(state.KeepAliveEvent)
		if !ok {
			return
		}
		stats := client.Stats()
		fmt.Printf("[keepalive] id=%d sent=%d received=%d uptime=%s\n", ke.ID, stats.PacketsSent, stats.PacketsReceived, stats.ConnectedFor.Truncate(time.Second))
	})

	if err := client.Connect(); err != nil {
		log.Fatalf("connect failed: %v", err)
	}
	defer client.Close()
	fmt.Printf("[connected] state=%v\n", client.CurrentState())

	go func() {
		s := bufio.NewScanner(os.Stdin)
		for s.Scan() {
			msg := s.Text()
			if msg == "" {
				continue
			}
			if err := client.SendChat(msg); err != nil {
				fmt.Printf("send chat error: %v\n", err)
			}
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	fmt.Println("[shutdown] closing client")
	if err := client.Close(); err != nil {
		fmt.Printf("[shutdown] close error: %v\n", err)
	}
}
