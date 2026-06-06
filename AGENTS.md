# Agent Instructions for FeastGo

## Core identity

FeastGo is an **offline-mode Minecraft Java Edition 1.20.4 / Protocol 765** bot framework written in Go. It is alpha/experimental. The public surface is `pkg/feast`. Everything else is internal infrastructure.

---

## Non-negotiable constraints

- Do **not** add online-mode auth or encryption unless the task explicitly requests it.
- Do **not** add multi-version support unless explicitly requested.
- Do **not** touch `pkg/nav/hpa` algorithm, state, or path logic unless the task explicitly says so. Log-only hygiene changes (gating unconditional prints) are acceptable with an explicit note.
- Do **not** add gameplay features (combat, mining layer, PvP) during protocol/reliability passes.
- Do **not** commit or push unless explicitly instructed.
- Do **not** run real server tests unless explicitly instructed.
- Do **not** fake PASS. If something fails, report it.
- Do **not** run `git add .`. Stage only the files you intend.

---

## Dirty working tree rule

Always run `git status --short` first. Classify every dirty file before touching any code:

- **A. Intended core changes** — files you plan to edit.
- **B. Unrelated local experiments** — mining files, server scripts, advanced harness expansions.
- **C. Unexpected** — anything that should not be dirty; investigate before proceeding.

Known experiment paths that must **never** be staged by a core pass:

```
tests/advanced_controls/mining.go
tests/advanced_controls/mining_test.go
tests/mining_validation/
tests/mininglib/
test/server/
```

---

## Testing expectations

Before any change, run baseline:

```bash
go test ./pkg/protocol ./pkg/state ./pkg/feast ./pkg/world
go vet ./pkg/protocol ./pkg/state ./pkg/feast ./pkg/world
go test -race ./pkg/world ./pkg/state ./pkg/conn ./pkg/feast
```

After changes, run the same set plus:

```bash
go test ./pkg/nav/executor ./pkg/nav/planner ./pkg/nav/move
go vet ./pkg/nav/executor ./pkg/nav/planner ./pkg/nav/move
go build ./...
```

Use `go test ./...` as a sanity check. If it fails on out-of-scope packages, report them rather than fixing them.

---

## Logging rule

No unconditional `fmt.Printf` / `log.Printf` on hot paths in `pkg/feast`, `pkg/state`, `pkg/protocol`, or `pkg/world`.

Use the existing gated helper in `pkg/feast/debug.go`:

```go
c.debugf("message %s", value)          // gated by c.opts.Debug
c.debugActionf("action", "k=v %d", n)  // gated by c.opts.Debug
```

For `pkg/nav` sub-packages, use their existing `DebugLogs` flags.

Verify with:

```bash
grep -Rn "fmt\.Print\|log\.Print\|println" pkg/feast pkg/protocol pkg/state pkg/world pkg/nav \
  | grep -v "_test.go"
```

Expected output: only gated helpers, panic-recovery in `bus.go`, world-error log in `chunk.go`, and the feast `c.log` helper (which checks `c.opts.Debug`).

---

## Protocol rule

Before editing any packet codec or adding a new packet ID:

1. Look up the exact packet ID in `pkg/protocol/consts/play.go`.
2. Cross-check the field layout against PrismarineJS `minecraft-data` for `pc/1.20.3` (the dataset shared by 1.20.3 and 1.20.4; protocol 765).
3. Write a test that decodes a hand-built wire image (not just a round-trip) to lock field order independently.

Do **not** guess packet IDs. Do **not** invent serverbound packets that do not exist in 765.

---

## Documentation rule

README must be honest:

- State that it is alpha/experimental.
- List what works and what does not.
- Do not claim production readiness.
- Do not invent API names — verify against actual code before documenting.
- Do not hide limitations.

---

## Package map

| Package | Purpose |
|---|---|
| `pkg/protocol` | Packet encoding/decoding, constants, framing |
| `pkg/state` | FSM, event bus (`EventBus`), packet dispatcher |
| `pkg/world` | Chunk, block, entity models |
| `pkg/nav` | A\*, HPA\*, executor, goals, movement |
| `pkg/feast` | Public client orchestration — start here |
| `pkg/conn` | Framed transport, compression |
| `tests/advanced_controls` | Dev harness (not public API) |

---

## Validation checklist (minimum before declaring PASS)

```
gofmt -w .
go test ./pkg/protocol ./pkg/state ./pkg/feast ./pkg/world
go vet ./pkg/protocol ./pkg/state ./pkg/feast ./pkg/world
go test -race ./pkg/world ./pkg/state ./pkg/conn ./pkg/feast
go test ./pkg/nav/executor ./pkg/nav/planner ./pkg/nav/move
go vet ./pkg/nav/executor ./pkg/nav/planner ./pkg/nav/move
go build ./...
```

All must pass cleanly before declaring `Final result: PASS`.

---

## Final report expectations

Every task must end with:

1. Scope compliance
2. Working tree classification
3. What was changed and why
4. Raw validation output (actual command output, not paraphrased)
5. Bugs found (if any)
6. Files changed list
7. Safe staging command (`git add` listing individual files)
8. Do-not-stage list
9. Final result: PASS / PARTIAL / FAIL

`Final result: PARTIAL` is not a failure. Use it honestly when constraints prevented full completion, and explain what remains.
