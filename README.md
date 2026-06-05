# FeastGo

FeastGo is a private Go bot framework and protocol implementation for Minecraft Java Edition 1.20.4, targeting Protocol 765. It features offline-mode login, FSM state management, chunk/world tracking, block lookup, local A*, and Hierarchical Pathfinding A* (HPA*) navigation.

## Crucial Protocol Constraints

*   **Offline-Mode Only:** FeastGo is strictly offline-mode.
*   **Target version:** Minecraft Java Edition 1.20.4, Protocol 765.
*   **Tested on:** Paper 1.20.4.
*   **Vanilla 1.20.4 Status:** NOT_RUN (No vanilla jar was stood up, but paper is fully verified).
*   **No Online-Mode Support:** Microsoft and Mojang/Yggdrasil authentication are not supported.
*   **No Encryption Support:** AES/CFB8 packet encryption is intentionally unsupported. Connection attempts to online-mode servers are refused and disconnected cleanly with an explicit error.
*   **No Access Tokens / Session Tokens:** Token-based authentication or session handling is not implemented.

## Feature Matrix

| Feature | Status | Notes |
|---|---|---|
| **Ping** | PASS | Handshake, Status request, and Ping request are fully implemented. |
| **Offline login** | PASS | Custom name login start is supported; encryption request is parsed and rejected cleanly. |
| **Config state** | PASS | Feature flags, registry data, tags, and finish configuration are synced. |
| **Play state** | PASS | Chunk tracking, player state sync, position updates, entity spawns, and time updates are supported. |
| **KeepAlive** | PASS | Client replies to server KeepAlives to stay connected. |
| **Chat send** | PASS | Serverbound chat message packets are fully verified. |
| **Chat receive** | PASS | System and player chat packets are parsed and displayed. |
| **Chunk tracking** | PASS | 3D section palette decoding, light updates, and chunks unloading are supported. |
| **Block lookup** | PASS | Fast block querying by coordinates from world state. |
| **Find block** | PASS | BFS/nearest block lookup for target block types. |
| **Block break** | PASS | Player action (digging start/finish) packets sent; waits for block update confirmation. |
| **Block place creative smoke** | PASS | Placement of creative smoke items is fully functional, verified by block update. |
| **Full inventory tracking** | PASS | Tracks all slots, hotbar, selection, empty slots, and counts. |
| **Hotbar selection** | PASS | Supports SelectedHotbarSlot, HeldItem, SelectHotbarSlot, and FindHotbarItem APIs. |
| **Survival place block** | Paper PASS | Places blocks from hotbar without creative injection; verifies block update and decrement. (Vanilla: NOT_RUN) |
| **Entity tracking** | PASS | Tracks position changes, spawns, and removals of neighboring entities. |
| **Entity metadata** | PASS | Decodes 1.20.4 entity metadata index values (including Pose) and updates store. |
| **Entity hitbox** | PASS | Calculates AABB based on entity type and pose; queries collisions thread-safely. |
| **Entity-aware pathfinding** | opt-in | A* pathfinder avoids blocking entities when `AvoidEntities=true`. |
| **Local A\*** | PASS | 3D local A* pathfinder for short-range segment paths. |
| **HPA\*** | PASS | 3D Hierarchical Pathfinding A* (HPA*) graph construction, cluster management, and path planning. |
| **Navigation** | PASS | Movement execution with stuck detection, climbing, jumping, and falling. |
| **Stuck detection** | PASS | Stuck position detection triggers replanning or clean failures. |
| **Clean disconnect** | PASS | Sends disconnect packet and tears down all loops cleanly. |
| **Command Graph** | NOT_IMPLEMENTED | Autocomplete commands list is skipped. |
| **Online-mode auth** | NOT_SUPPORTED | Intentionally unsupported. |

## Running Tests

To run the full suite of unit tests, run:
```sh
go test ./...
```

To run package tests with the Go race detector enabled:
```sh
go test -race ./pkg/world ./pkg/state ./pkg/conn ./pkg/feast ./pkg/nav/...
```

To run a linter check:
```sh
go vet ./...
```

## Running Server Smoke Tests

First, start a local Minecraft Java Edition 1.20.4 server (e.g. Paper) with `online-mode=false` in `server.properties` on port 25565.

Then, execute any of the following integration commands:

```sh
# 1. Ping the server
go run ./cmd/ping 127.0.0.1 25565

# 2. Join and track world chunks
FEAST_DEBUG=true MC_HOST=127.0.0.1 MC_PORT=25565 MC_USERNAME=FeastGoBot go run ./cmd/bot --smoke-world

# 3. Join and find nearest grass_block
FEAST_DEBUG=true MC_HOST=127.0.0.1 MC_PORT=25565 MC_USERNAME=FeastGoBot go run ./cmd/bot --find-block grass_block

# 4. Run full HPA pathfinding tests
FEAST_DEBUG=true MC_HOST=127.0.0.1 MC_PORT=25565 MC_USERNAME=FeastGoBot go run ./cmd/bot --hpa-test

# 5. Place a block (creative smoke mode)
FEAST_DEBUG=true MC_HOST=127.0.0.1 MC_PORT=25565 MC_USERNAME=FeastGoBot go run ./cmd/bot --smoke-place-block

# 6. Break a block
FEAST_DEBUG=true MC_HOST=127.0.0.1 MC_PORT=25565 MC_USERNAME=FeastGoBot go run ./cmd/bot --smoke-break-block

# 7. Place and break block mutation sequence
FEAST_DEBUG=true MC_HOST=127.0.0.1 MC_PORT=25565 MC_USERNAME=FeastGoBot go run ./cmd/bot --smoke-world-mutate

# 8. Run HPA rebuilds and planner queries after block changes
FEAST_DEBUG=true MC_HOST=127.0.0.1 MC_PORT=25565 MC_USERNAME=FeastGoBot go run ./cmd/bot --hpa-mutation-test

# 9. Run a stability soak test (e.g. 60 seconds)
FEAST_DEBUG=true MC_HOST=127.0.0.1 MC_PORT=25565 MC_USERNAME=FeastGoBot go run ./cmd/bot --soak 60s
