package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/qrjhamron/feast/pkg/feast"
	"github.com/qrjhamron/feast/pkg/nav/goal"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	debug, _ := strconv.ParseBool(getenv("FEAST_DEBUG", "false"))
	bot, err := feast.Connect(ctx, feast.Options{
		Host:     getenv("MC_HOST", "127.0.0.1"),
		Port:     getenv("MC_PORT", "25565"),
		Username: getenv("MC_USERNAME", "FeastGoBot"),
		Debug:    debug,
	})
	if err != nil {
		fmt.Printf("[api-test] connect=false error=%q\n", err)
		fmt.Printf("[api-test] result=FAIL\n")
		return
	}
	fmt.Printf("[api-test] connect=true\n")

	var readySeen atomic.Bool
	var chatSeen atomic.Bool
	var healthSeen atomic.Bool
	var positionSeen atomic.Bool
	var blockUpdateSeen atomic.Bool
	var entitySpawnSeen atomic.Bool
	var entityMoveSeen atomic.Bool
	var entityRemoveSeen atomic.Bool
	var errorSeen atomic.Bool
	var disconnectClean atomic.Bool

	bot.OnReady(func() { readySeen.Store(true) })
	bot.OnChat(func(feast.ChatEvent) { chatSeen.Store(true) })
	bot.OnHealth(func(feast.HealthEvent) { healthSeen.Store(true) })
	bot.OnPosition(func(feast.PositionEvent) { positionSeen.Store(true) })
	bot.OnBlockUpdate(func(feast.BlockUpdateEvent) { blockUpdateSeen.Store(true) })
	bot.OnEntitySpawn(func(feast.EntityEvent) { entitySpawnSeen.Store(true) })
	bot.OnEntityMove(func(feast.EntityEvent) { entityMoveSeen.Store(true) })
	bot.OnEntityRemove(func(feast.EntityEvent) { entityRemoveSeen.Store(true) })
	bot.OnError(func(error) { errorSeen.Store(true) })
	bot.OnDisconnect(func(err error) { disconnectClean.Store(err == nil) })

	readyCtx, readyCancel := context.WithTimeout(context.Background(), 15*time.Second)
	err = bot.WaitUntilReady(readyCtx)
	readyCancel()
	ready := err == nil
	if ready {
		readySeen.Store(true)
	}
	fmt.Printf("[api-test] ready=%v\n", ready)

	if ready {
		err = bot.Chat("api validation hello")
		fmt.Printf("[api-test] chat_sent=%v\n", err == nil)
	} else {
		fmt.Printf("[api-test] chat_sent=false\n")
	}

	time.Sleep(2 * time.Second)
	pos := bot.Position()
	fmt.Printf("[api-test] position=x=%.2f,y=%.2f,z=%.2f\n", pos.X, pos.Y, pos.Z)

	worldChunks := bot.World().ChunkCount()
	fmt.Printf("[api-test] world_chunks=%d\n", worldChunks)

	_, blockQueryErr := bot.World().GetBlock(int(math.Floor(pos.X)), int(math.Floor(pos.Y-1)), int(math.Floor(pos.Z)))
	fmt.Printf("[api-test] world_query=%v\n", blockQueryErr == nil)

	var found feast.BlockPos
	findBlock := false
	for _, name := range []string{"grass_block", "dirt", "stone"} {
		if hit, ok := bot.FindNearestBlock(name, 64); ok {
			found = feast.BlockPos{X: int32(hit.X), Y: int32(hit.Y), Z: int32(hit.Z)}
			findBlock = true
			break
		}
	}
	fmt.Printf("[api-test] find_block=%v\n", findBlock)

	navOK := false
	if target, ok := nearbyStandable(bot); ok {
		navCtx, navCancel := context.WithTimeout(context.Background(), 12*time.Second)
		err = bot.NavigateTo(navCtx, goal.NewGoalXZ(target.X, target.Z))
		navOK = err == nil
		navCancel()
	}
	fmt.Printf("[api-test] navigate=%v\n", navOK)

	breakOK := false
	if findBlock {
		breakCtx, breakCancel := context.WithTimeout(context.Background(), 8*time.Second)
		err = bot.BreakBlock(breakCtx, found)
		breakCancel()
		breakOK = err == nil
	}
	fmt.Printf("[api-test] break_block=%v\n", breakOK)

	survivalPlaceOK := false
	if target, ok := nearbyPlaceTarget(bot); ok {
		placeCtx, placeCancel := context.WithTimeout(context.Background(), 10*time.Second)
		err = bot.PlaceBlockSurvival(placeCtx, feast.BlockPos{X: int32(target.X), Y: int32(target.Y), Z: int32(target.Z)}, feast.FaceUp)
		placeCancel()
		survivalPlaceOK = err == nil
	}
	fmt.Printf("[api-test] survival_place=%v\n", survivalPlaceOK)

	entities := bot.Entities().All()
	entitySeen := len(entities) > 0 || entitySpawnSeen.Load() || entityMoveSeen.Load() || entityRemoveSeen.Load()
	fmt.Printf("[api-test] entity_seen=%v\n", entitySeen)
	if len(entities) > 0 {
		e := entities[0]
		box := e.Hitbox()
		fmt.Printf("[api-test] entity_metadata_entries=%d\n", len(e.Metadata))
		fmt.Printf("[api-test] entity_hitbox=min=%.2f,%.2f,%.2f,max=%.2f,%.2f,%.2f\n", box.MinX, box.MinY, box.MinZ, box.MaxX, box.MaxY, box.MaxZ)
	}

	discErr := bot.Disconnect()
	time.Sleep(200 * time.Millisecond)
	if discErr == nil && !disconnectClean.Load() {
		status := bot.ShutdownStatus()
		disconnectClean.Store(status.Requested && status.SocketClosed)
	}
	fmt.Printf("[api-test] disconnect_clean=%v\n", discErr == nil && disconnectClean.Load())
	fmt.Printf("[api-test] hooks=ready:%v,chat:%v,health:%v,position:%v,block_update:%v,entity_spawn:%v,entity_move:%v,entity_remove:%v,error:%v,disconnect:%v\n",
		readySeen.Load(), chatSeen.Load(), healthSeen.Load(), positionSeen.Load(), blockUpdateSeen.Load(),
		entitySpawnSeen.Load(), entityMoveSeen.Load(), entityRemoveSeen.Load(), errorSeen.Load(), disconnectClean.Load())

	result := "PASS"
	reasons := make([]string, 0)
	if !ready || worldChunks == 0 || !findBlock || !navOK || !breakOK || discErr != nil || !disconnectClean.Load() {
		result = "FAIL"
	}
	if result == "PASS" && (!survivalPlaceOK || !entitySeen) {
		result = "PARTIAL"
		if !survivalPlaceOK {
			reasons = append(reasons, "survival_place_not_confirmed")
		}
		if !entitySeen {
			reasons = append(reasons, "no_real_entity_observed")
		}
	}
	if len(reasons) > 0 {
		fmt.Printf("[api-test] result=%s reason=%s\n", result, strings.Join(reasons, ","))
	} else {
		fmt.Printf("[api-test] result=%s\n", result)
	}
}

type blockTarget struct {
	X int
	Y int
	Z int
}

func nearbyStandable(bot *feast.Client) (blockTarget, bool) {
	pos := bot.Position()
	originX := int(math.Floor(pos.X))
	originZ := int(math.Floor(pos.Z))
	for radius := 1; radius <= 6; radius++ {
		for dx := -radius; dx <= radius; dx++ {
			for dz := -radius; dz <= radius; dz++ {
				if abs(dx) != radius && abs(dz) != radius {
					continue
				}
				x := originX + dx
				z := originZ + dz
				y := bot.World().GetSurfaceY(x, z) + 1
				if y < -64 {
					continue
				}
				if bot.World().IsPassable(x, y, z) && bot.World().IsPassable(x, y+1, z) {
					return blockTarget{X: x, Y: y, Z: z}, true
				}
			}
		}
	}
	return blockTarget{}, false
}

func nearbyPlaceTarget(bot *feast.Client) (blockTarget, bool) {
	pos := bot.Position()
	originX := int(math.Floor(pos.X))
	originZ := int(math.Floor(pos.Z))
	for radius := 2; radius <= 8; radius++ {
		for dx := -radius; dx <= radius; dx++ {
			for dz := -radius; dz <= radius; dz++ {
				if abs(dx) != radius && abs(dz) != radius {
					continue
				}
				x := originX + dx
				z := originZ + dz
				y := bot.World().GetSurfaceY(x, z) + 1
				if y < -64 {
					continue
				}
				air, err := bot.World().GetBlock(x, y, z)
				if err != nil || air.Name != "air" {
					continue
				}
				support, err := bot.World().GetBlock(x, y-1, z)
				if err != nil || !support.Solid {
					continue
				}
				return blockTarget{X: x, Y: y, Z: z}, true
			}
		}
	}
	return blockTarget{}, false
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
