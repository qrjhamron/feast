# Advanced Controls Runner

This directory contains a manual real-world API control harness for local/private offline-mode Minecraft Java 1.20.4 (Protocol 765) servers. It is controlled using Minecraft chat commands starting with `!`.

## Run Command

Start the runner with:

```bash
FEAST_DEBUG=true MC_HOST=127.0.0.1 MC_PORT=25565 MC_USERNAME=FeastGoBot go run ./tests/advanced_controls
```

### Logging verbosity

Output is compact by default (one short summary per command). Two optional
environment flags increase detail:

- `ADV_VERBOSE=true` — per-step diagnostic chat lines (e.g. each scaffold step,
  every `!breakn` block instead of an every-5 progress line).
- `ADV_TRACE_PACKETS=true` — reserved for raw movement-packet tracing.

```bash
ADV_VERBOSE=false ADV_TRACE_PACKETS=false MC_HOST=127.0.0.1 go run ./tests/advanced_controls
```

## Command List

- **`!help`**: Shows all available commands.
- **`!pos`**: Displays current position and sync status.
- **`!inv`**: Summarizes active hotbar items.
- **`!find <block> [count] [radius]`**: Locates and lists nearest blocks matching the name or alias.
- **`!nav <block|x y z> [radius]`**: Navigates near a block or to a coordinate. For
  coordinates it prefers a nearby safe standing spot at a similar Y and reports a
  clear reason (`chunk_unloaded`, `target_unsafe`) when it cannot.
- **`!navbuild <x y z>`**: Navigates to a coordinate, first normally and then with a
  limited survival scaffold-assisted route (max 64 placements) if a gap/height
  blocks the direct path. Reports honest `PASS`/`PARTIAL`/`FAIL` with a reason.
- **`!hpa <x y z>`**: Run HPA navigation diagnostics to coordinates.
- **`!move <distance>`**: Moves forward along current heading, targeting a safe spot
  near the bot's current feet level (does not snap to the surface).
- **`!swim [radius]`**: Finds water, enters it, and swims.
- **`!scaffold <length>`**: Bridges/steps forward placing survival blocks only, moving
  onto each placed block; supports a one-block climb and refuses two-block jumps.
- **`!breakn <block> <count> [radius]`**: Breaks up to `count` blocks safely.
- **`!build10 <block>`**: Builds a 10x10 platform of `block` at the level below player feet.
- **`!stop`**: Disconnects and stops the runner.
