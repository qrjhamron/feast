# FeastGo

A Go-native Minecraft Java Edition bot framework. Protocol 765 / Minecraft 1.20.4.

FeastGo gives you a fully typed, embeddable bot runtime in pure Go — no Node.js, no browser runtime, no Electron. Connect to an offline-mode 1.20.4 server, track world state, navigate terrain, break and place blocks, manage inventory, and react to events, all from clean Go code.

---

## Why FeastGo?

- **Go-native** — one `go get`, no FFI, no subprocess
- **Typed protocol** — every packet is a proper struct; nothing is stringly typed
- **World state** — full chunk/block/entity tracking with an A\* + HPA\* nav stack
- **Strong test culture** — unit tests, race tests, protocol codec tests, no fake-pass
- **Honest scope** — offline-mode 1.20.4 only; limitations are documented, not hidden

---

## Status

**Alpha / experimental.** The core protocol stack is working and tested against a local Paper 1.20.4 server. APIs are stable enough to build real bots with, but may change as the library matures.

| | |
|---|---|
| Minecraft | Java Edition **1.20.4** |
| Protocol | **765** |
| Auth | **Offline-mode only** (`online-mode=false`) |
| Tested server | Paper 1.20.4 |

---

## What works

| Feature | Status |
|---|---|
| Offline login + play handshake | ✅ |
| KeepAlive | ✅ |
| Chunk / world tracking | ✅ |
| Block update / section blocks | ✅ |
| Entity tracking (spawn, move, remove, metadata, hitboxes) | ✅ |
| Chat send / receive | ✅ |
| Local A\* pathfinding | ✅ |
| HPA\* pathfinding | ✅ |
| Navigation (`NavigateTo`) | ✅ |
| Block break | ✅ |
| Block place (creative + survival) | ✅ |
| Survival inventory tracking | ✅ |
| Container/chest operations | ✅ |
| Protocol 765 Bundle Delimiter | ✅ |
| Protocol 765 Acknowledge Block Change | ✅ |
| Protocol 765 Respawn / dimension change | ✅ |
| Clean disconnect | ✅ |
| Online-mode auth / encryption | ❌ not supported |
| Multi-version | ❌ not planned |
| Command Graph | ❌ not yet |

---

## Install

Requires Go 1.22+.

```bash
go get github.com/qrjhamron/feast/pkg/feast
```

---

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/qrjhamron/feast/pkg/feast"
    "github.com/qrjhamron/feast/pkg/nav/goal"
)

func main() {
    ctx := context.Background()

    bot, err := feast.Connect(ctx, feast.Options{
        Host:     "127.0.0.1",
        Port:     "25565",
        Username: "FeastBot",
    })
    if err != nil {
        log.Fatal(err)
    }
    defer bot.Disconnect()

    // Block until the server sends the first position sync.
    if err := bot.WaitUntilReady(ctx); err != nil {
        log.Fatal(err)
    }

    pos := bot.Position()
    fmt.Printf("spawned at %.1f %.1f %.1f\n", pos.X, pos.Y, pos.Z)

    // Listen for chat.
    bot.OnChat(func(e feast.ChatEvent) {
        fmt.Printf("<%s> %s\n", e.Sender, e.Message)
    })

    // Find and walk to nearest grass block.
    if hit, ok := bot.FindNearestBlock("grass_block", 64); ok {
        _ = bot.NavigateTo(ctx, goal.NewGoalBlock(hit.X, hit.Y+1, hit.Z))
    }

    // Stay alive until Ctrl+C.
    select {}
}
```

---

## Architecture

```
pkg/
  feast/      public client API ← start here
  protocol/   packet codecs, constants, framing
  state/      FSM, event bus, packet dispatcher
  world/      chunk/block/entity models
  nav/        A*, HPA*, executor, goals, movement

cmd/
  bot/        minimal interactive demo bot
  ping/       server status / MOTD query
  smoke/      offline integration CLI

tests/
  advanced_controls/   dev harness (not public API)
```

The public surface lives entirely in `pkg/feast`. Everything else is internal infrastructure.

---

## Public API

```go
// Connection
feast.Connect(ctx, opts)        → *Client, error   // dial + login + play handshake
feast.NewClient(opts)           → *Client           // create without connecting
client.Connect()                → error
client.Disconnect()             → error
client.WaitUntilReady(ctx)      → error             // blocks until first position sync

// World state
client.Position()               → world.Vec3
client.Health()                 → float32
client.Food()                   → int32
client.World()                  → *world.World
client.Entities()               → *world.EntityStore
client.Inventory()              → *InventoryState
client.FindNearestBlock(name, radius) → (BlockHit, bool)

// Navigation
client.NavigateTo(ctx, goal)    → error
client.StopNavigation()

// Interaction
client.BreakBlock(ctx, pos)     → error
client.PlaceBlockCreative(ctx, target, face, blockName) → error
client.PlaceBlockSurvival(ctx, target, face) → error
client.Chat(message)            → error

// Inventory
client.SelectHotbarSlot(ctx, slot) → error
client.HeldItem()               → (ItemStack, bool)
client.FindHotbarItem(name)     → (slot, ItemStack, bool)

// Events (return unsubscribe func)
client.OnReady(func())
client.OnChat(func(ChatEvent))
client.OnHealth(func(HealthEvent))
client.OnPosition(func(PositionEvent))
client.OnBlockUpdate(func(BlockUpdateEvent))
client.OnEntitySpawn(func(EntityEvent))
client.OnEntityMove(func(EntityEvent))
client.OnEntityRemove(func(EntityEvent))
client.OnError(func(error))
client.OnDisconnect(func(error))
```

---

## Protocol Compatibility (765)

Three reliability pieces were added for Paper 1.20.4:

**Bundle Delimiter (`0x00`)** — Paper groups packets that must land in the same tick between two delimiters. FeastGo recognises them and keeps dispatch in order without desyncing the stream.

**Acknowledge Block Change (`0x05`)** — The server sends a sequence acknowledgement for every break/place action. FeastGo decodes it and routes it through the event bus. The authoritative result of a break or place is still the `Block Update` that follows; the ack alone does not mark anything as succeeded.

**Respawn (`0x45`)** — On death or dimension change the server sends Respawn. FeastGo decodes the full Protocol 765 layout, clears stale world chunks, block entities, and tracked entities, cancels active navigation, and marks position unsynced. The bot becomes ready again after the next server position sync.

---

## Examples

| Example | What it shows |
|---|---|
| `examples/basic_join` | Connect, WaitUntilReady, read position |
| `examples/chat_echo` | OnChat, Chat — echo bot |
| `examples/find_block` | FindNearestBlock |
| `examples/navigate_to_block` | FindNearestBlock + NavigateTo |
| `examples/break_block` | BreakBlock, OnBlockUpdate |
| `examples/place_block_creative` | PlaceBlockCreative |
| `examples/place_block_survival` | PlaceBlockSurvival |
| `examples/entity_events` | OnEntitySpawn, OnEntityMove, OnEntityRemove |
| `examples/avoid_entities` | Entities().Nearby() proximity scan |
| `examples/public_api_validation` | End-to-end public API validation against a real server |

```bash
MC_HOST=127.0.0.1 go run ./examples/basic_join
```

---

## Commands

```bash
# Query server status (no login)
go run ./cmd/ping 127.0.0.1

# Interactive demo bot
MC_HOST=127.0.0.1 MC_USERNAME=FeastBot go run ./cmd/bot

# Smoke/integration CLI (one feature per run, exits with result=PASS/FAIL)
go run ./cmd/smoke --smoke-world
go run ./cmd/smoke --smoke-break-block
go run ./cmd/smoke --hpa-test
go run ./cmd/smoke --soak 30s
```

---

## Development

### Unit tests (no server required)

```bash
go test ./...
```

### Race tests

```bash
go test -race ./pkg/world ./pkg/state ./pkg/conn ./pkg/feast
```

### Integration tests (require a live offline-mode server)

```bash
export MC_HOST=127.0.0.1 MC_PORT=25565 MC_USERNAME=FeastBot
go test ./test/integration -tags=integration -v
```

Without a server, integration tests skip cleanly.

### Build

```bash
go build ./...
```

---

## Safety and Limitations

- **Offline-mode only.** FeastGo does not implement Microsoft/Mojang/Yggdrasil authentication, the online-mode encryption handshake, or the shared-secret flow. It connects only to servers with `online-mode=false`.
- **Single protocol version.** Protocol 765 / Minecraft 1.20.4 only.
- **Vanilla untested.** Validated exclusively against Paper 1.20.4.
- **No anti-cheat bypass.** FeastGo is a clean bot framework.
- **Alpha API.** Public API shapes are stable in practice but may change before a 1.0 release.

---

## Project Philosophy

FeastGo prioritises protocol correctness and runtime reliability over feature count. A bot that silently desyncs after a death respawn or a bundle of packets is worse than a bot with fewer features. Every public behaviour is tested; every known limitation is documented. There is no faking of PASS.
