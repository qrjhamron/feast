# Repository Guidelines

## Project Structure & Module Organization
- `cmd/ping/main.go`: CLI entrypoint for server status ping.
- `cmd/bot/main.go`: CLI entrypoint for login/chat bot behavior.
- `pkg/feast/`: internal client orchestrator for login, configuration, play loop, events, and chat.
- `pkg/state/`: FSM, event bus, and packet dispatcher.
- `pkg/conn/`: framed transport, compression, and encryption streams.
- `pkg/protocol/`: packet constants, primitive codecs, framing helpers, and typed packets.
- `pkg/client_legacy/`: archived raw implementation behind the `legacy` build tag.
- `pkg/protocol/*_test.go`: unit tests for protocol primitives.
- `.env`: local runtime configuration (do not commit secrets).

Keep new executable entrypoints under `cmd/<name>/main.go` and reusable logic under `pkg/<domain>/`.

## Build, Test, and Development Commands
- `go test ./...`: run all unit tests across packages.
- `go test ./pkg/protocol -v`: run protocol tests with verbose output.
- `go run ./cmd/ping <host> [port]`: ping a Minecraft server.
- `go run ./cmd/bot [host] [username] [port]`: run the bot client.
- `go build ./cmd/ping && go build ./cmd/bot`: build both CLIs.
- `go fmt ./...`: format all Go source files.

Run commands from repository root (`/root/feastgo`).

## Coding Style & Naming Conventions
- Follow standard Go style (`gofmt` formatting, tabs, grouped imports).
- Use short, package-scoped names in `pkg/protocol`; use descriptive names in `pkg/feast` and `pkg/state` for runtime flow.
- Exported identifiers use `PascalCase`; internal helpers use `camelCase`.
- Keep packet IDs and protocol-state comments explicit near handlers.

## Testing Guidelines
- Framework: Go `testing` package.
- Place tests adjacent to implementation using `*_test.go`.
- Prefer table-driven tests for protocol encode/decode edge cases.
- Add/extend tests whenever packet parsing, varint logic, or compression behavior changes.
- Before PRs: run `go test ./...` and ensure no regressions.

## Commit & Pull Request Guidelines
- Commit style: imperative, scoped subject lines (e.g., `protocol: fix varint overflow handling`).
- Keep commits focused; separate refactors from behavior changes.
- PRs should include:
  - clear summary of functional changes,
  - test evidence (exact commands run),
  - protocol-impact notes if packet/state behavior changed,
  - terminal output snippets for CLI behavior changes.

## Security & Configuration Tips
- Prefer `MC_HOST`, `MC_PORT`, and `MC_USERNAME` env vars for local runs.
- Never hardcode credentials or tokens.
- Treat `.env` as local-only; provide sanitized examples in docs when needed.
