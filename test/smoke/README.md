# FeastGo Smoke Scripts

Smoke scripts connect to a **live Minecraft Paper 1.20.4** server and execute
structured smoke tests via `cmd/smoke`. Current validation targets protocol 765
in offline mode only.

## Quick Start

```bash
export MC_HOST=127.0.0.1
export MC_PORT=25565
export MC_USERNAME=FeastGoBot

# Full paper smoke suite:
bash ./test/smoke/scripts/run_paper_smoke.sh

# Local validation only (no server):
bash ./test/smoke/scripts/run_local_validation.sh
```

Required `server.properties` baseline:

```properties
online-mode=false
spawn-protection=0
difficulty=peaceful
gamemode=survival
enable-command-block=false
view-distance=10
simulation-distance=10
```

## Individual Modes

Run any single smoke mode:

```bash
go run ./cmd/smoke --smoke-world
go run ./cmd/smoke --smoke-break-block
go run ./cmd/smoke --smoke-place-block
go run ./cmd/smoke --hpa-test
go run ./cmd/smoke --soak 30s
```

See `go run ./cmd/smoke --help` for the full list.
