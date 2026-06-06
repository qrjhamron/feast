# FeastGo

**FeastGo** is an offline-mode Minecraft Java Edition bot framework written in Go.
It is designed as a clean, embeddable library – similar in spirit to Mineflayer or Azalea –
exposing a high-level API on top of a tested, Protocol-765-compliant transport stack.

> ⚠️ **Offline-mode only.** FeastGo does **not** support Microsoft/Mojang authentication,
> Yggdrasil, online-mode encryption, or the Minecraft encryption handshake.
> It is for use with servers that have `online-mode=false`.

---

## Supported Version

| Property | Value |
|---|---|
| Minecraft | Java Edition **1.20.4** |
| Protocol | **765** |
| Tested server | **Paper 1.20.4** |
| Vanilla | Not tested |
| Multi-version | Not supported |

Latest real-world validation in this checkout used a local/private Paper
1.20.4 server reporting protocol 765 with `online-mode=false`.

---

## Install

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
        Username: "FeastGoBot",
    })
    if err != nil {
        log.Fatal(err)
    }
    defer bot.Disconnect()

    // Event hook registration.
    bot.OnChat(func(e feast.ChatEvent) {
        fmt.Printf("[chat] %s: %s\n", e.Sender, e.Message)
    })

    // Wait until position-synced and ready.
    if err := bot.WaitUntilReady(ctx); err != nil {
        log.Fatal(err)
    }

    // Find nearest grass block and navigate to it.
    hit, ok := bot.FindNearestBlock("grass_block", 64)
    if ok {
        g := goal.NewGoalBlock(hit.X, hit.Y+1, hit.Z)
        _ = bot.NavigateTo(ctx, g)
    }
}
```

---

## Feature Matrix

| Feature | Status |
|---|---|
| Offline login | ✅ |
| Configuration + Play handshake | ✅ |
| KeepAlive | ✅ |
| Chunk / world tracking | ✅ |
| Chat send / receive | ✅ |
| Entity tracking (spawn, move, remove) | ✅ |
| Entity metadata + hitboxes | ✅ |
| Local A\* pathfinding | ✅ |
| HPA\* pathfinding | ✅ |
| Navigation (NavigateTo) | ✅ |
| Block break | ✅ |
| Creative block place | ✅ |
| Survival block place | ✅ |
| Survival inventory tracking | ✅ |
| Entity-aware pathfinding | ✅ |
| Clean disconnect | ✅ |
| Online-mode auth | ❌ intentionally excluded |
| Encryption | ❌ intentionally excluded |
| Command Graph | ❌ not yet |
| Multi-version | ❌ not planned |

---

## Public API Overview

```go
// Connect creates and returns a connected Client.
func Connect(ctx context.Context, opts Options) (*Client, error)

// NewClient creates a Client without connecting.
func NewClient(opts Options) *Client

// Connection
func (c *Client) Connect() error
func (c *Client) Disconnect() error
func (c *Client) WaitUntilReady(ctx context.Context) error

// Chat
func (c *Client) Chat(message string) error
func (c *Client) SendChat(message string) error

// State
func (c *Client) Position() world.Vec3
func (c *Client) Health() float32
func (c *Client) Food() int32

// World & entities
func (c *Client) World() *world.World
func (c *Client) Entities() *world.EntityStore
func (c *Client) Inventory() *InventoryState

// Block search
func (c *Client) FindNearestBlock(name string, radius int) (world.BlockHit, bool)

// Navigation
func (c *Client) NavigateTo(ctx context.Context, g goal.Goal) error
func (c *Client) StopNavigation()

// Block interaction
func (c *Client) BreakBlock(ctx context.Context, pos BlockPos) error
func (c *Client) PlaceBlockCreative(ctx context.Context, target BlockPos, face Direction, blockName string) error
func (c *Client) PlaceBlockSurvival(ctx context.Context, target BlockPos, face Direction) error

// Inventory
func (c *Client) SelectHotbarSlot(ctx context.Context, slot int) error
func (c *Client) HeldItem() (ItemStack, bool)
func (c *Client) FindHotbarItem(name string) (slot int, stack ItemStack, ok bool)

// Event hooks
func (c *Client) OnReady(fn func())
func (c *Client) OnChat(fn func(ChatEvent))
func (c *Client) OnHealth(fn func(HealthEvent))
func (c *Client) OnPosition(fn func(PositionEvent))
func (c *Client) OnBlockUpdate(fn func(BlockUpdateEvent))
func (c *Client) OnEntitySpawn(fn func(EntityEvent))
func (c *Client) OnEntityMove(fn func(EntityEvent))
func (c *Client) OnEntityRemove(fn func(EntityEvent))
func (c *Client) OnError(fn func(error))
func (c *Client) OnDisconnect(fn func(error))

// Low-level (escape hatch)
func (c *Client) On(eventType string, handler func(state.Event)) (int, error)
func (c *Client) Events() *state.EventBus
func (c *Client) WritePacket(p protocol.Packet) error
```

---

## Commands

### `cmd/ping` – Server Status

```bash
go run ./cmd/ping <host> [port]
```

Queries the server's status packet (ping + MOTD) without logging in.

### `cmd/bot` – Demo Bot

```bash
MC_HOST=127.0.0.1 MC_USERNAME=FeastGoBot go run ./cmd/bot
```

A minimal interactive bot that connects, prints events, reads chat from stdin,
and shuts down cleanly on `Ctrl+C` or `!quit`.

### `cmd/smoke` – Smoke/Integration Test CLI

```bash
go run ./cmd/smoke --smoke-world
go run ./cmd/smoke --smoke-break-block
go run ./cmd/smoke --hpa-test
go run ./cmd/smoke --soak 30s
```

Connects to a live server and exercises one feature per run. Each mode prints
structured `[tag] key=value` lines and ends with `result=PASS` / `result=FAIL`.

Run all smoke modes at once:

```bash
bash ./test/smoke/scripts/run_paper_smoke.sh
```

Full flag list: `go run ./cmd/smoke --help`

Public API validation runner:

```bash
MC_HOST=127.0.0.1 MC_PORT=25565 MC_USERNAME=FeastGoBot go run ./examples/public_api_validation
```

---

## Examples

| Example | What it shows |
|---|---|
| `examples/basic_join` | Connect, WaitUntilReady, status |
| `examples/chat_echo` | OnChat, Chat, echo bot |
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

## Testing

### Unit tests (no server required)

```bash
go test ./...
```

### Race tests

```bash
go test -race ./pkg/world ./pkg/state ./pkg/conn ./pkg/feast
```

### Integration tests (require a live server)

```bash
export MC_HOST=127.0.0.1 MC_PORT=25565 MC_USERNAME=FeastGoBot
go test ./test/integration -tags=integration -v
```

Without a server, integration tests skip cleanly.

### Smoke scripts

```bash
bash ./test/smoke/scripts/run_paper_smoke.sh
bash ./test/smoke/scripts/run_local_validation.sh  # no server needed
```

---

## Package Structure

```
cmd/
  bot/      simple demo bot
  ping/     server status ping
  smoke/    smoke / integration test CLI

pkg/
  feast/    public client API  ← start here
  world/    chunk, block, entity models
  nav/      pathfinding (A*, HPA*, executor, goals, moves)
  state/    FSM, event bus, packet dispatcher
  conn/     framed transport, compression
  protocol/ packet codecs, constants

internal/
  smoke/    shared helpers for cmd/smoke

test/
  integration/   live-server integration tests (build tag: integration)
  smoke/         smoke scripts and README

examples/   one example per feature
```

---

## Remaining Limitations

- **Offline-mode only** – online-mode auth, encryption, and Microsoft/Mojang/Yggdrasil login are intentionally not supported.
- **No Command Graph** – server command tab-completion is not implemented.
- **No multi-version** – only Protocol 765 / Minecraft 1.20.4.
- **Vanilla untested** – validated against Paper 1.20.4 only; vanilla may differ.
- **No exploit/bypass logic** – FeastGo is a clean bot framework, not an exploit tool.
