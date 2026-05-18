# FeastGo

FeastGo is a private Go foundation for Minecraft Java Edition 1.20.4, protocol 765. It is a protocol/runtime library for building bots, not a full Mineflayer-style automation stack yet.

## Current Architecture

- `pkg/protocol`: Minecraft primitive readers/writers, packet framing, typed Login/Configuration packets, and selected Play packets used by the runtime.
- `pkg/protocol/consts`: protocol 765 packet ID registries.
- `pkg/conn`: framed packet IO, compression threshold handling, and AES/CFB8 stream support.
- `pkg/state`: connection FSM, synchronous event bus, and packet dispatcher.
- `pkg/feast`: internal client orchestrator for login, configuration, play read loop, heartbeat, chat, and player state snapshots.
- `cmd/bot`: small example bot using `pkg/feast`.
- `cmd/ping`: minimal status/ping client.
- `pkg/client_legacy`: archived raw client implementation behind the `legacy` build tag.

## Implemented Features

- Offline-mode login/configuration flow for protocol 765.
- Compression threshold support after Set Compression.
- Play-state dispatch for login play, position sync, keepalive, disconnect, system chat, raw player chat, health, selected chunk/container packets.
- Synchronous events: `login`, `play_login`, `spawn`, `position`, `chat`, `keep_alive`, `health`, `kick`, `disconnect`, and raw `packet`.
- Basic client APIs: `Connect`, `Disconnect`, `Close`, `On`, `Events`, `CurrentState`, `PlayerState`, and `SendChat`.
- Minimal tracked player state: entity ID, position, yaw/pitch, health, food, and saturation.
- Runtime stats via `Stats()`: connected time, packet counters, last keepalive, last position sync, and current FSM state.
- Optional debug logging through `Options.Debug`, `Options.DebugPackets`, and `Options.Logger`.

## Not Implemented Yet

- Online-mode authentication/encryption login flow.
- Pathfinding, block breaking/placing automation, inventory automation, crafting, or AI/LLM logic.
- Full chunk decoding, block palette decoding, entity tracking, and full item component/NBT interpretation.
- Complete typed Play packet coverage.

## Example

```go
client := feast.NewClient(feast.Options{
    Host:     "localhost",
    Port:     "25565",
    Username: "FeastBot",
})

client.On("chat", func(e state.Event) {
    msg := e.(state.ChatEvent)
    fmt.Println(msg.Sender, msg.Message)
})

if err := client.Connect(); err != nil {
    log.Fatal(err)
}
defer client.Close()
```

## Commands

```sh
go test ./...
go build ./cmd/bot ./cmd/ping
MC_HOST=localhost MC_PORT=25565 MC_USERNAME=FeastBot go run ./cmd/bot
FEAST_DEBUG=true go run ./cmd/bot
FEAST_DEBUG_PACKETS=true go run ./cmd/bot
go run ./cmd/ping localhost 25565
```

The default target is fixed at Minecraft Java Edition 1.20.4 / protocol 765.

`cmd/bot` expects an offline-mode local server unless you add online-mode auth support. It prints lifecycle, position, health, keepalive, chat, kick, and disconnect events.
