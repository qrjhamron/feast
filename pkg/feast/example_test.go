package feast_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/qrjhamron/feast/pkg/feast"
	"github.com/qrjhamron/feast/pkg/nav/goal"
	"github.com/qrjhamron/feast/pkg/state"
)

func ExampleConnect() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	bot, err := feast.Connect(ctx, feast.Options{
		Host:     "127.0.0.1",
		Port:     "25565",
		Username: "FeastGoBot",
	})
	if err != nil {
		log.Fatal(err)
	}
	defer bot.Disconnect()
}

func ExampleClient_WaitUntilReady() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	bot, err := feast.Connect(ctx, feast.Options{
		Host:     "127.0.0.1",
		Port:     "25565",
		Username: "FeastGoBot",
	})
	if err != nil {
		log.Fatal(err)
	}
	defer bot.Disconnect()

	readyCtx, readyCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer readyCancel()
	if err := bot.WaitUntilReady(readyCtx); err != nil {
		log.Fatal(err)
	}
}

func ExampleClient_Chat() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	bot, err := feast.Connect(ctx, feast.Options{
		Host:     "127.0.0.1",
		Port:     "25565",
		Username: "FeastGoBot",
	})
	if err != nil {
		log.Fatal(err)
	}
	defer bot.Disconnect()

	if err := bot.Chat("hello from FeastGo"); err != nil {
		log.Fatal(err)
	}
}

func ExampleClient_FindNearestBlock() {
	bot := feast.NewClient(feast.Options{})

	hit, ok := bot.FindNearestBlock("oak_log", 32)
	if !ok {
		return
	}
	fmt.Println(hit.X, hit.Y, hit.Z)
}

func ExampleClient_NavigateTo() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	bot := feast.NewClient(feast.Options{})
	if err := bot.NavigateTo(ctx, goal.NewGoalBlock(10, 64, 10)); err != nil {
		log.Fatal(err)
	}
}

func ExampleClient_BreakBlockWithResult() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	bot := feast.NewClient(feast.Options{})
	result, err := bot.BreakBlockWithResult(ctx, feast.BlockPos{X: 10, Y: 64, Z: 10}, feast.BreakOptions{
		AutoTool: true,
	})
	if err != nil {
		if errors.Is(err, feast.ErrChunkNotLoaded) {
			return
		}
		log.Fatal(err)
	}
	fmt.Printf("broke %s with %s\n", result.OldBlock, result.ToolUsed)
}

func ExampleClient_PlaceBlockSurvival() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	bot := feast.NewClient(feast.Options{})
	target, face, ok := feast.FindPlaceTargetNear(bot.World(), bot.Position(), "stone", 4)
	if !ok {
		return
	}
	if err := bot.PlaceBlockSurvival(ctx, target, face); err != nil {
		if errors.Is(err, feast.ErrNoPlaceableBlock) {
			return
		}
		log.Fatal(err)
	}
}

func ExampleClient_State() {
	bot := feast.NewClient(feast.Options{Username: "FeastGoBot"})
	snap := bot.State()
	fmt.Println(snap.Username, snap.Ready)
}

func ExampleClient_WaitForPositionSync() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bot := feast.NewClient(feast.Options{})
	_ = bot.WaitForPositionSync(ctx)
}

func ExampleClient_WaitForBlockUpdate() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bot := feast.NewClient(feast.Options{})
	_, _ = bot.WaitForBlockUpdate(ctx, 0, 64, 0)
}

func ExampleClient_WaitForBlockState() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bot := feast.NewClient(feast.Options{})
	_ = bot.WaitForBlockState(ctx, 0, 64, 0, "stone")
}

func ExampleClient_OnRespawn() {
	bot := feast.NewClient(feast.Options{})
	unsub := bot.OnRespawn(func(ev state.RespawnEvent) {
		fmt.Println(ev.DimensionName)
	})
	defer unsub()
}

func ExampleClient_OnBlockUpdate() {
	bot := feast.NewClient(feast.Options{})
	unsub := bot.OnBlockUpdate(func(ev feast.BlockUpdateEvent) {
		fmt.Println(ev.X, ev.Y, ev.Z)
	})
	defer unsub()
}

func ExampleClient_PlaceBlockWithResult() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bot := feast.NewClient(feast.Options{})
	_, _ = bot.PlaceBlockWithResult(ctx, feast.BlockPos{X: 0, Y: 64, Z: 0}, feast.FaceUp)
}
