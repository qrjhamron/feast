# FeastGo Integration Tests

Integration tests require a live Minecraft **Paper 1.20.4** server in **offline mode**.

## Running Integration Tests

```bash
# With server running:
export MC_HOST=127.0.0.1
export MC_PORT=25565
export MC_USERNAME=FeastGoBot

go test ./test/integration -tags=integration -v -timeout 120s
```

Without a live server, tests skip automatically with a clean `t.Skip` message.

## Test Coverage

| Test | What it verifies |
|---|---|
| `TestIntegration_ConnectAndReady` | Connect + position sync within 15s |
| `TestIntegration_ChatSend` | Chat packet accepted by server |
| `TestIntegration_WorldChunks` | At least 1 chunk loaded after join |
| `TestIntegration_FindNearestBlock` | Block search in loaded world |
| `TestIntegration_CleanDisconnect` | All shutdown flags set on Disconnect |

## Normal `go test ./...`

Integration tests are **excluded** from the default test run because they are
behind the `integration` build tag. You can safely run:

```bash
go test ./...
```

without a server and all tests will pass.
