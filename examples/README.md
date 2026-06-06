# FeastGo Examples

Each subdirectory is a self-contained `main` package demonstrating a single feature.

## Running an Example

All examples read server coordinates from environment variables:

```bash
export MC_HOST=127.0.0.1
export MC_PORT=25565
export MC_USERNAME=FeastGoBot

go run ./examples/basic_join
```

## Examples

| Example | What it shows |
|---|---|
| [basic_join](./basic_join/) | Connect, WaitUntilReady, status display |
| [chat_echo](./chat_echo/) | OnChat, Chat send, echo bot |
| [find_block](./find_block/) | FindNearestBlock |
| [navigate_to_block](./navigate_to_block/) | FindNearestBlock + NavigateTo |
| [break_block](./break_block/) | BreakBlock, OnBlockUpdate |
| [place_block_creative](./place_block_creative/) | PlaceBlockCreative |
| [place_block_survival](./place_block_survival/) | PlaceBlockSurvival |
| [entity_events](./entity_events/) | OnEntitySpawn, OnEntityMove, OnEntityRemove |
| [avoid_entities](./avoid_entities/) | Entities().Nearby() for proximity check |
| [public_api_validation](./public_api_validation/) | End-to-end public API validation against a real server |

## Prerequisites

- A running Minecraft Java Edition **1.20.4** server in **offline mode**.
- Go 1.22+.

## Compile Check (no server needed)

```bash
go test ./examples/...
```

This compiles all examples without running them (main packages without `_test.go` are
compiled but not executed by `go test`).
