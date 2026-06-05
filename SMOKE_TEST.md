# FeastGo Integration & Smoke Testing

This document describes how to execute and verify the complete integration and smoke tests against a local Minecraft server.

## Prerequisites

1.  **Minecraft Server**: Download Paper or a vanilla Minecraft server for version 1.20.4.
2.  **Configuration**: In `server.properties`, set `online-mode=false` and ensure `server-port=25565`. Start the server.
3.  **Command Execution Location**: Run all commands from the repository root `/root/feastgo`.

## Running the Verification Commands

### 1. Pinger verification
Verify that the server status ping is working:
```sh
go run ./cmd/ping 127.0.0.1 25565
```
**Expected Output**:
```
Status: {"version":{"name":"Paper 1.20.4","protocol":765},"description":"A Minecraft Server","players":{"max":20,"online":0}}
Latency: 5ms
```

### 2. World and Chunk Tracking
Join the server, verify configuration synchronization, play transition, chunk tracking, block palettes, surface Y checks, and chat send/receive:
```sh
FEAST_DEBUG=true MC_HOST=127.0.0.1 MC_PORT=25565 MC_USERNAME=FeastGoBot go run ./cmd/bot --smoke-world
```
**Expected Output**:
```
[world] chunks_loaded=...
[world] spawn_block={...}
[world] passable_check=true
[world] solid_ground_check=true
[chat-send] sent=true message="hello from FeastGo full integration test"
[world-smoke] result=PASS
```

### 3. Block Search
Search for the nearest `grass_block` relative to the bot:
```sh
FEAST_DEBUG=true MC_HOST=127.0.0.1 MC_PORT=25565 MC_USERNAME=FeastGoBot go run ./cmd/bot --find-block grass_block
```
**Expected Output**:
```
[find-block] target=grass_block found=true x=... y=... z=... distance=...
```

### 4. HPA Pathfinding Test
Verify Hierarchical Pathfinding A* (HPA*) abstract graph construction, cluster manager loading, entrance node creation, optimistic/refined local planning, and abstract path verification:
```sh
FEAST_DEBUG=true MC_HOST=127.0.0.1 MC_PORT=25565 MC_USERNAME=FeastGoBot go run ./cmd/bot --hpa-test
```
**Expected Output**:
```
[hpa-test] mode=full
[hpa-test] abstract_path_found=true
[hpa-test] refined_segments=...
[hpa-test] result=PASS
```

### 5. Block Placement (Creative Smoke Mode)
Test placing a block (creative smoke mode) at a valid coordinate, ensuring hitbox overlap checks, support blocks check, packet transmission, and block update confirmation:
```sh
FEAST_DEBUG=true MC_HOST=127.0.0.1 MC_PORT=25565 MC_USERNAME=FeastGoBot go run ./cmd/bot --smoke-place-block
```
**Expected Output**:
```
[place] mode=creative_smoke
[place] selected_slot=0
[place] held_item=stone
[place] target=...
[place] support=...
[place] old_state=air
[place] packet_sent=true
[place] block_update_received=true
[place] new_state=stone
[place] rollback_detected=false
[place] result=PASS
```

### 6. Invalid Block Placement
Verify that invalid placement (e.g. overlapping player hitbox) is correctly caught and rejected with a clean error instead of panicking:
```sh
FEAST_DEBUG=true MC_HOST=127.0.0.1 MC_PORT=25565 MC_USERNAME=FeastGoBot go run ./cmd/bot --smoke-place-block-invalid
```
**Expected Output**:
```
[place-invalid] result=PASS reason=clean_error
```

### 7. Block Breaking
Test breaking a block (start/finish destroy action), waiting for block update confirmation, and checking if the block state successfully updates to `air`:
```sh
FEAST_DEBUG=true MC_HOST=127.0.0.1 MC_PORT=25565 MC_USERNAME=FeastGoBot go run ./cmd/bot --smoke-break-block
```
**Expected Output**:
```
[break] target=...
[break] old_state=...
[break] start_sent=true
[break] finish_sent=true
[break] block_update_received=true
[break] new_state=air
[break] hpa_invalidated=true
[break] result=PASS
```

### 8. World Mutations and Consistency
Execute a combined mutation sequence (join -> place stone -> verify place -> break stone -> verify air -> verify passable -> verify HPA invalidations):
```sh
FEAST_DEBUG=true MC_HOST=127.0.0.1 MC_PORT=25565 MC_USERNAME=FeastGoBot go run ./cmd/bot --smoke-world-mutate
```
**Expected Output**:
```
[mutate] place_result=PASS
[mutate] break_result=PASS
[mutate] final_block=air
[mutate] passable_after_break=true
[mutate] hpa_invalidations>=2=true
[mutate] result=PASS
```

### 9. HPA Under Mutation
Verify HPA stats, place block, verify HPA invalidation, break block, verify HPA invalidation, and ensure paths can still be planned/refined:
```sh
FEAST_DEBUG=true MC_HOST=127.0.0.1 MC_PORT=25565 MC_USERNAME=FeastGoBot go run ./cmd/bot --hpa-mutation-test
```
**Expected Output**:
```
[hpa-mutation] place_block=true
[hpa-mutation] invalidation_after_place=true
[hpa-mutation] break_block=true
[hpa-mutation] invalidation_after_break=true
[hpa-mutation] result=PASS
```

### 10. Stability Soak Test
Run a stability soak test (e.g. 60 seconds) to ensure KeepAlives are handled, no loop crashes occur, and the client disconnects cleanly:
```sh
FEAST_DEBUG=true MC_HOST=127.0.0.1 MC_PORT=25565 MC_USERNAME=FeastGoBot go run ./cmd/bot --soak 60s
```
**Expected Output**:
```
[soak] duration=60s
[soak] keepalive_received=...
[soak] keepalive_sent=...
[soak] errors=0
[soak] clean_disconnect=true
[soak] result=PASS
```

### 11. Connection Refusal (Server Unavailable)
Verify that attempting to connect to a offline/dead port terminates cleanly with a refusal error:
```sh
MC_HOST=127.0.0.1 MC_PORT=25566 go run ./cmd/bot --smoke-world
```
**Expected Output**:
```
[error-test] connect_refused=true
[error-test] result=PASS
```

### 12. Inventory Dump
Dump the player's inventory to verify hotbar, selection, and container slots:
```sh
FEAST_DEBUG=true MC_HOST=127.0.0.1 MC_PORT=25565 MC_USERNAME=FeastGoBot go run ./cmd/bot --inventory-dump
```
**Expected Output**:
```
[inventory] selected_hotbar=...
[inventory] hotbar[0]=...
...
[inventory] result=PASS
```

### 13. Survival Block Placement
Test placing a block in survival mode, verifying real inventory decrement and server block update confirmation:
```sh
FEAST_DEBUG=true MC_HOST=127.0.0.1 MC_PORT=25565 MC_USERNAME=FeastGoBot go run ./cmd/bot --smoke-place-survival
```
**Expected Output**:
```
[place-survival] mode=full_inventory
[place-survival] selected_slot=...
[place-survival] held_item=...
[place-survival] target=...
[place-survival] support=...
[place-survival] face=...
[place-survival] old_state=air
[place-survival] packet_sent=true
[place-survival] block_update_received=true
[place-survival] new_state=stone
[place-survival] rollback_detected=false
[place-survival] inventory_count_before=16
[place-survival] inventory_count_after=15
[place-survival] result=PASS
```

### 14. Entity Metadata Decoding
Test decoding 1.20.4 entity metadata fields:
```sh
FEAST_DEBUG=true MC_HOST=127.0.0.1 MC_PORT=25565 MC_USERNAME=FeastGoBot go run ./cmd/bot --entity-metadata-test
```
**Expected Output**:
```
[entity-meta] id=...
[entity-meta] type=pig
[entity-meta] pose=...
[entity-meta] width=0.90
[entity-meta] height=0.90
[entity-meta] metadata_entries=...
[entity-meta] result=PASS
```

### 15. Entity Hitbox system
Verify the calculation of hitboxes and collision detection querying:
```sh
FEAST_DEBUG=true MC_HOST=127.0.0.1 MC_PORT=25565 MC_USERNAME=FeastGoBot go run ./cmd/bot --entity-hitbox-test
```
**Expected Output**:
```
[hitbox] entity_id=...
[hitbox] type=pig
[hitbox] pose=...
[hitbox] aabb=...
[hitbox] collision_detected=...
[hitbox] result=PASS
```

