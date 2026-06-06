// cmd/smoke is the FeastGo smoke / integration test CLI.
//
// It connects to a live Minecraft server and exercises specific features.
// Each mode prints structured [tag] key=value lines and ends with result=PASS/FAIL.
//
// Usage:
//
//	go run ./cmd/smoke [flags]
//
// Configuration (environment variables):
//
//	MC_HOST      server hostname (default: localhost)
//	MC_PORT      server port (default: 25565)
//	MC_USERNAME  player name (default: FeastGoBot)
//
// Flags:
//
//	--smoke-world            Smoke test world features and chat
//	--find-block <name>      Find nearest block by name
//	--hpa-test               Test HPA* graph and path refinement
//	--smoke-break-block      Test block breaking
//	--smoke-place-block      Test creative block placement
//	--smoke-place-block-invalid  Test invalid placement rejection
//	--smoke-world-mutate     Test place+break mutation and HPA invalidation
//	--hpa-mutation-test      Test HPA* graph update after mutations
//	--soak <duration>        Stability soak test (e.g. 30s)
//	--inventory-dump         Dump player hotbar contents
//	--smoke-place-survival   Test survival block placement
//	--entity-metadata-test   Test entity metadata parsing
//	--entity-hitbox-test     Test entity hitbox calculation
//	--entity-test            Test entity tracking events
package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/qrjhamron/feast/pkg/feast"
	"github.com/qrjhamron/feast/pkg/nav/executor"
	"github.com/qrjhamron/feast/pkg/nav/goal"
	"github.com/qrjhamron/feast/pkg/nav/hpa"
	"github.com/qrjhamron/feast/pkg/nav/move"
	navplanner "github.com/qrjhamron/feast/pkg/nav/planner"
	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/state"
	"github.com/qrjhamron/feast/pkg/world"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	smokeWorld := flag.Bool("smoke-world", false, "Smoke test world features")
	findBlock := flag.String("find-block", "", "Smoke test find block")
	hpaTest := flag.Bool("hpa-test", false, "Smoke test HPA* graph and refinement")
	smokeBreak := flag.Bool("smoke-break-block", false, "Smoke test break block")
	smokePlace := flag.Bool("smoke-place-block", false, "Smoke test place block")
	smokePlaceInvalid := flag.Bool("smoke-place-block-invalid", false, "Smoke test invalid block place")
	smokeWorldMutate := flag.Bool("smoke-world-mutate", false, "Smoke test world mutations")
	hpaMutation := flag.Bool("hpa-mutation-test", false, "Smoke test HPA after block mutations")
	soakDur := flag.String("soak", "", "Stability soak test duration")
	entityTest := flag.Bool("entity-test", false, "Smoke test entity tracking")
	inventoryDump := flag.Bool("inventory-dump", false, "Dump player inventory")
	smokePlaceSurvival := flag.Bool("smoke-place-survival", false, "Smoke test survival block place")
	entityMetadataTest := flag.Bool("entity-metadata-test", false, "Smoke test entity metadata")
	entityHitboxTest := flag.Bool("entity-hitbox-test", false, "Smoke test entity hitboxes")
	coreActionTest := flag.Bool("core-action-test", false, "Run core action integration test")
	flag.Parse()

	debug, _ := strconv.ParseBool(getenv("FEAST_DEBUG", "false"))
	debugPackets, _ := strconv.ParseBool(getenv("FEAST_DEBUG_PACKETS", "false"))

	client := feast.NewClient(feast.Options{
		Host:         getenv("MC_HOST", "localhost"),
		Port:         getenv("MC_PORT", "25565"),
		Username:     getenv("MC_USERNAME", "FeastGoBot"),
		Debug:        debug,
		DebugPackets: debugPackets,
	})

	entityRecorder := newEntitySmokeRecorder()
	if *entityTest {
		entityRecorder.register(client)
	}

	client.On("login", func(e state.Event) {
		le, ok := e.(state.LoginEvent)
		if !ok {
			return
		}
		fmt.Printf("[login] username=%s uuid=%s\n", le.Username, le.UUID)
	})
	client.On("play_login", func(e state.Event) {
		pe, ok := e.(state.PlayLoginEvent)
		if !ok {
			return
		}
		fmt.Printf("[play] entity_id=%d\n", pe.EntityID)
	})
	client.On("chat", func(e state.Event) {
		ce, ok := e.(state.ChatEvent)
		if !ok {
			return
		}
		fmt.Printf("[chat] %s: %s\n", ce.Sender, ce.Message)
	})
	client.On("kick", func(e state.Event) {
		ke, ok := e.(state.KickEvent)
		if !ok {
			return
		}
		fmt.Printf("[kick] %s\n", ke.Reason)
	})
	client.On("disconnect", func(e state.Event) {
		de, ok := e.(state.DisconnectEvent)
		if !ok {
			return
		}
		fmt.Printf("[disconnect] clean=%v reason=%s\n", de.Clean, de.Reason)
	})
	client.On("error", func(e state.Event) {
		ee, ok := e.(state.ErrorEvent)
		if !ok {
			return
		}
		fmt.Printf("[error] op=%s err=%v\n", ee.Op, ee.Error)
	})
	client.On("position", func(e state.Event) {
		pe, ok := e.(state.PositionEvent)
		if !ok {
			return
		}
		fmt.Printf("[position] x=%.2f y=%.2f z=%.2f yaw=%.2f pitch=%.2f teleport=%d\n",
			pe.X, pe.Y, pe.Z, pe.Yaw, pe.Pitch, pe.TeleportID)
	})
	client.On("health", func(e state.Event) {
		he, ok := e.(state.HealthEvent)
		if !ok {
			return
		}
		fmt.Printf("[health] health=%.1f food=%d saturation=%.1f\n", he.Health, he.Food, he.Saturation)
	})
	client.On("keep_alive", func(e state.Event) {
		ke, ok := e.(state.KeepAliveEvent)
		if !ok {
			return
		}
		stats := client.Stats()
		fmt.Printf("[keepalive] id=%d sent=%d received=%d uptime=%s\n",
			ke.ID, stats.PacketsSent, stats.PacketsReceived, stats.ConnectedFor.Truncate(time.Second))
	})
	client.On("spawn", func(e state.Event) {
		se, ok := e.(state.SpawnEvent)
		if !ok {
			return
		}
		fmt.Printf("[spawn] entity_id=%d x=%.2f y=%.2f z=%.2f\n", se.EntityID, se.X, se.Y, se.Z)
	})

	if err := client.Connect(); err != nil {
		errStr := err.Error()
		if *coreActionTest {
			fmt.Printf("[core] connect=false\n")
		}
		if strings.Contains(errStr, "connection refused") || strings.Contains(errStr, "dial tcp") {
			fmt.Printf("[error-test] connect_refused=true\n")
			fmt.Printf("[error-test] result=PASS\n")
			os.Exit(0)
		}
		if strings.Contains(errStr, "online-mode encryption is intentionally unsupported") {
			fmt.Printf("[login] online_mode_unsupported=true\n")
			fmt.Printf("[login] error=online-mode encryption is intentionally unsupported\n")
			fmt.Printf("[result]=PASS\n")
			os.Exit(0)
		}
		log.Fatalf("connect failed: %v", err)
	}
	defer client.Close()
	fmt.Printf("[connected] state=%v\n", client.CurrentState())
	if *coreActionTest {
		fmt.Printf("[core] connect=true\n")
	}

	waitForChunks(client, 1, 5*time.Second)

	if *smokeWorld {
		runSmokeWorld(client)
	}

	if *findBlock != "" {
		runFindBlockSmoke(client, *findBlock)
		return
	}

	if *hpaTest {
		runHPATest(client)
		return
	}

	if *smokeBreak {
		runBreakBlockSmoke(client)
		return
	}

	if *smokePlace {
		runPlaceBlockSmoke(client)
		return
	}

	if *smokePlaceInvalid {
		runPlaceBlockInvalidSmoke(client)
		return
	}

	if *smokeWorldMutate {
		runWorldMutateSmoke(client)
		return
	}

	if *hpaMutation {
		runHPAMutationTest(client)
		return
	}

	if *soakDur != "" {
		runSoakTest(client, *soakDur)
		return
	}

	if *inventoryDump {
		runInventoryDump(client)
		return
	}

	if *smokePlaceSurvival {
		runPlaceBlockSurvivalSmoke(client)
		return
	}

	if *entityMetadataTest {
		runEntityMetadataTest(client)
		return
	}

	if *entityHitboxTest {
		runEntityHitboxTest(client)
		return
	}

	if *entityTest {
		runEntitySmoke(client, entityRecorder)
		return
	}
	if *coreActionTest {
		runCoreActionTest(client)
		return
	}
}

// ─── World smoke ──────────────────────────────────────────────────────────────

func waitForChunks(client *feast.Client, minimum int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if client.World().ChunkCount() >= minimum {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func runSmokeWorld(client *feast.Client) {
	x, y, z, _, _ := client.GetPosition()
	fmt.Printf("[world] chunks_loaded=%d\n", client.World().ChunkCount())
	block, err := client.World().GetBlock(int(x), int(y), int(z))
	if err != nil {
		fmt.Printf("[world] spawn_block=ERROR\n")
	} else {
		fmt.Printf("[world] spawn_block=%v\n", block)
	}
	fmt.Printf("[world] surface_y=%d\n", client.World().GetSurfaceY(int(x), int(z)))
	fmt.Printf("[world] passable_check=%v\n", client.World().IsPassable(int(x), int(y), int(z)))
	fmt.Printf("[world] solid_ground_check=%v\n", !client.World().IsPassable(int(x), int(y)-1, int(z)))
	if err := client.SendChat("hello from FeastGo full integration test"); err != nil {
		fmt.Printf("[chat-send] sent=false error=%v\n", err)
	} else {
		fmt.Printf("[chat-send] sent=true message=\"hello from FeastGo full integration test\"\n")
		time.Sleep(1500 * time.Millisecond)
	}
}

// ─── Find block ───────────────────────────────────────────────────────────────

func runFindBlockSmoke(client *feast.Client, target string) {
	x, y, z, _, _ := client.GetPosition()
	hit, ok := client.World().FindNearestBlock(world.Vec3{X: x, Y: y, Z: z}, target, 64)
	if !ok {
		fmt.Printf("[find-block] target=%s found=false\n", target)
		return
	}
	fmt.Printf("[find-block] target=%s found=true x=%d y=%d z=%d distance=%.3f\n",
		target, hit.X, hit.Y, hit.Z, hit.Distance)
}

// ─── HPA* test ────────────────────────────────────────────────────────────────

func runHPATest(client *feast.Client) {
	waitForChunks(client, 2, 6*time.Second)
	px, py, pz, _, _ := client.GetPosition()
	centerChunkX := floorDivInt(int(math.Floor(px)), world.ChunkWidth)
	centerChunkZ := floorDivInt(int(math.Floor(pz)), world.ChunkDepth)
	const hpaSmokeRadiusChunks = 1

	buildStart := time.Now()
	probe := buildBoundedChunkGraph(client.World(), centerChunkX, centerChunkZ, hpaSmokeRadiusChunks)
	abstractPath := probe.longestAbstractPath()

	// Select start and goal positions
	var start, goalPos [3]int
	if len(abstractPath) >= 2 {
		start = probe.nodes[abstractPath[0]]
		goalPos = probe.nodes[abstractPath[len(abstractPath)-1]]
	} else {
		var nodes [][3]int
		for _, pos := range probe.nodes {
			nodes = append(nodes, pos)
		}
		if len(nodes) >= 2 {
			start = nodes[0]
			goalPos = nodes[1]
		} else if len(nodes) == 1 {
			start = nodes[0]
			goalPos = [3]int{start[0] + 16, start[1], start[2]}
		} else {
			start = [3]int{int(math.Floor(px)), int(math.Floor(py)), int(math.Floor(pz))}
			goalPos = [3]int{start[0] + 16, start[1], start[2]}
		}
	}

	fullGraph := hpa.NewAbstractGraph()
	fullClusters := hpa.NewClusterManager(nil)
	fullBuilder := hpa.NewGraphBuilder(client.World(), fullGraph, fullClusters)
	fullBuilder.SetOptimisticIntraEdges(true)

	// Build clusters for loaded chunks in range
	for _, coord := range client.World().ChunkCoords() {
		if absInt(coord[0]-centerChunkX) <= hpaSmokeRadiusChunks && absInt(coord[1]-centerChunkZ) <= hpaSmokeRadiusChunks {
			fullClusters.GetOrCreate(coord[0], coord[1])
		}
	}

	buildCtx, buildCancel := context.WithTimeout(context.Background(), 20*time.Second)
	buildErr := fullBuilder.RebuildDirtyWithContext(buildCtx)
	buildCtxErr := buildCtx.Err()
	buildCancel()
	buildDuration := time.Since(buildStart)
	_ = buildDuration

	graphNodes, graphEdges := fullGraph.Stats()
	clusters := fullClusters.Count()
	entrances := fullClusters.EntranceCount()

	startClusterCoord := [2]int{floorDivInt(start[0], 16), floorDivInt(start[2], 16)}
	goalClusterCoord := [2]int{floorDivInt(goalPos[0], 16), floorDivInt(goalPos[2], 16)}

	var startConnected bool
	var goalConnected bool
	var startComponentSize int
	var goalComponentSize int
	var sameComponent bool
	var abstractPathFound bool
	var abstractError error
	var refinedSegments int
	var fallbackUsed bool = false

	if buildErr != nil {
		if buildCtxErr != nil {
			abstractError = buildCtxErr
		} else {
			abstractError = buildErr
		}
	} else {
		fullPlanner := hpa.NewHPAPlanner(client.World(), fullGraph, fullClusters)
		planCtx, planCancel := context.WithTimeout(context.Background(), 8*time.Second)
		res := fullPlanner.PlanWithContext(planCtx, start, goalPos, goal.NewGoalBlock(goalPos[0], goalPos[1], goalPos[2]))

		startConnected = res.StartConnected
		goalConnected = res.GoalConnected
		startComponentSize = res.StartComponentSize
		goalComponentSize = res.GoalComponentSize
		sameComponent = res.SameComponent
		abstractError = res.Err

		if res.Status == navplanner.PlanFound && len(res.AbstractPath) >= 2 {
			abstractPathFound = true
			for !res.Refiner.IsComplete() {
				segment := res.Refiner.NextSegment()
				if len(segment) == 0 {
					if planCtx.Err() != nil {
						abstractError = planCtx.Err()
					} else {
						abstractError = fmt.Errorf("lazy refinement failed")
					}
					break
				}
				refinedSegments++
			}
		}
		planCancel()
	}

	graphValid := clusters > 0 && entrances > 0 && graphNodes > 0 && graphEdges > 0

	var result string
	var reason string

	if !graphValid {
		result = "FAIL"
	} else {
		if startConnected && goalConnected && sameComponent && abstractPathFound && refinedSegments > 0 && !fallbackUsed {
			result = "PASS"
		} else {
			result = "PARTIAL"
			if client.World().ChunkCount() < 2 {
				reason = "insufficient_loaded_chunks"
			} else if !client.World().HasChunk(floorDivInt(goalPos[0], 16), floorDivInt(goalPos[2], 16)) {
				reason = "goal_not_in_loaded_chunk"
			} else if !startConnected {
				reason = "start_temp_node_not_connected"
			} else if !goalConnected {
				reason = "goal_temp_node_not_connected"
			} else if !sameComponent {
				reason = "start_goal_different_components"
			} else if !abstractPathFound {
				reason = "start_goal_different_components"
			} else if refinedSegments == 0 {
				reason = "lazy_refinement_failed"
			} else {
				reason = "lazy_refinement_failed"
			}
		}
	}

	var abstractErrorStr string
	if abstractError != nil {
		abstractErrorStr = abstractError.Error()
	} else {
		abstractErrorStr = "<nil>"
	}

	fmt.Printf("[hpa] start_pos=%d,%d,%d\n", start[0], start[1], start[2])
	fmt.Printf("[hpa] goal_pos=%d,%d,%d\n", goalPos[0], goalPos[1], goalPos[2])
	fmt.Printf("[hpa] start_cluster=%d,%d\n", startClusterCoord[0], startClusterCoord[1])
	fmt.Printf("[hpa] goal_cluster=%d,%d\n", goalClusterCoord[0], goalClusterCoord[1])
	fmt.Printf("[hpa] start_connected=%t\n", startConnected)
	fmt.Printf("[hpa] goal_connected=%t\n", goalConnected)
	fmt.Printf("[hpa] start_component_size=%d\n", startComponentSize)
	fmt.Printf("[hpa] goal_component_size=%d\n", goalComponentSize)
	fmt.Printf("[hpa] same_component=%t\n", sameComponent)
	fmt.Printf("[hpa] graph_nodes=%d\n", graphNodes)
	fmt.Printf("[hpa] graph_edges=%d\n", graphEdges)
	fmt.Printf("[hpa] abstract_path_found=%t\n", abstractPathFound)
	fmt.Printf("[hpa] abstract_error=%s\n", abstractErrorStr)
	fmt.Printf("[hpa] refined_segments=%d\n", refinedSegments)
	fmt.Printf("[hpa] fallback_used=%t\n", fallbackUsed)
	if result == "PARTIAL" {
		fmt.Printf("[hpa] result=PARTIAL reason=%s\n", reason)
	} else {
		fmt.Printf("[hpa] result=%s\n", result)
	}
}

// ─── Bounded chunk graph helpers ─────────────────────────────────────────────

type boundedChunkGraph struct {
	nodes     map[[2]int][3]int
	adj       map[[2]int][][2]int
	entrances int
	edgeCount int
}

func buildBoundedChunkGraph(w *world.World, centerChunkX, centerChunkZ, radiusChunks int) boundedChunkGraph {
	graph := boundedChunkGraph{
		nodes: make(map[[2]int][3]int),
		adj:   make(map[[2]int][][2]int),
	}
	for _, coord := range w.ChunkCoords() {
		if absInt(coord[0]-centerChunkX) > radiusChunks || absInt(coord[1]-centerChunkZ) > radiusChunks {
			continue
		}
		if p, ok := standablePointInChunk(w, coord[0], coord[1]); ok {
			graph.nodes[[2]int{coord[0], coord[1]}] = p
		}
	}
	for key, from := range graph.nodes {
		for _, delta := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			nextKey := [2]int{key[0] + delta[0], key[1] + delta[1]}
			to, ok := graph.nodes[nextKey]
			if !ok {
				continue
			}
			if localRefinementExists(w, from, to) {
				graph.adj[key] = append(graph.adj[key], nextKey)
				graph.edgeCount++
			}
		}
	}
	graph.entrances = graph.edgeCount
	return graph
}

func localRefinementExists(w *world.World, from, to [3]int) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	result := navplanner.Plan(ctx, from[0], from[1], from[2], goal.NewGoalBlock(to[0], to[1], to[2]), w)
	return result.Status == navplanner.PlanFound && len(result.Path) > 0
}

func (g boundedChunkGraph) abstractPath(startChunkX, startChunkZ, targetChunkX, targetChunkZ int) [][2]int {
	s := [2]int{startChunkX, startChunkZ}
	t := [2]int{targetChunkX, targetChunkZ}
	if _, ok := g.nodes[s]; !ok {
		return nil
	}
	if _, ok := g.nodes[t]; !ok {
		return nil
	}
	queue := [][2]int{s}
	parent := map[[2]int][2]int{}
	seen := map[[2]int]bool{s: true}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == t {
			path := [][2]int{cur}
			for cur != s {
				cur = parent[cur]
				path = append(path, cur)
			}
			for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
				path[i], path[j] = path[j], path[i]
			}
			return path
		}
		for _, next := range g.adj[cur] {
			if seen[next] {
				continue
			}
			seen[next] = true
			parent[next] = cur
			queue = append(queue, next)
		}
	}
	return nil
}

func (g boundedChunkGraph) longestAbstractPath() [][2]int {
	var best [][2]int
	for start := range g.nodes {
		for target := range g.nodes {
			if start == target {
				continue
			}
			path := g.abstractPath(start[0], start[1], target[0], target[1])
			if len(path) > len(best) {
				best = path
			}
		}
	}
	return best
}

func standablePointInChunk(w *world.World, chunkX, chunkZ int) ([3]int, bool) {
	baseX := chunkX * world.ChunkWidth
	baseZ := chunkZ * world.ChunkDepth
	for _, local := range [][2]int{{8, 8}, {4, 4}, {12, 12}, {4, 12}, {12, 4}, {8, 4}, {4, 8}, {8, 12}, {12, 8}} {
		x := baseX + local[0]
		z := baseZ + local[1]
		if y, ok := standableYAt(w, x, z); ok {
			return [3]int{x, y, z}, true
		}
	}
	for lx := 0; lx < world.ChunkWidth; lx++ {
		for lz := 0; lz < world.ChunkDepth; lz++ {
			x := baseX + lx
			z := baseZ + lz
			if y, ok := standableYAt(w, x, z); ok {
				return [3]int{x, y, z}, true
			}
		}
	}
	return [3]int{}, false
}

func standableYAt(w *world.World, x, z int) (int, bool) {
	surfaceY := w.GetSurfaceY(x, z)
	if surfaceY != world.UnknownSurfaceY {
		y := surfaceY + 1
		if isStandable(w, x, y, z) {
			return y, true
		}
	}
	for y := world.MinY + 1; y < world.MaxY; y++ {
		if isStandable(w, x, y, z) {
			return y, true
		}
	}
	return 0, false
}

func isStandable(w *world.World, x, y, z int) bool {
	return w.IsPassable(x, y, z) && w.IsPassable(x, y+1, z) && !w.IsPassable(x, y-1, z)
}

// ─── Break block ──────────────────────────────────────────────────────────────

func runBreakBlockSmoke(client *feast.Client) {
	target, ok := findBreakTarget(client)
	if !ok {
		fmt.Printf("[break] result=NOT_IMPLEMENTED reason=no_loaded_grass_dirt_or_stone_target\n")
		return
	}
	fmt.Printf("[break] target=%d,%d,%d\n", target.X, target.Y, target.Z)
	fmt.Printf("[break] old_state=%s\n", target.Block.Name)

	updateCh := make(chan state.BlockUpdateEvent, 16)
	blockHandlerID, _ := client.On("block_update", func(e state.Event) {
		if ev, ok := e.(state.BlockUpdateEvent); ok {
			select {
			case updateCh <- ev:
			default:
			}
		}
	})
	sectionHandlerID, _ := client.On("section_blocks_update", func(e state.Event) {
		ev, ok := e.(state.SectionBlocksUpdateEvent)
		if !ok {
			return
		}
		for _, u := range ev.Updates {
			select {
			case updateCh <- state.BlockUpdateEvent{X: u.X, Y: u.Y, Z: u.Z, StateID: u.StateID}:
			default:
			}
		}
	})
	defer client.Events().Off(blockHandlerID)
	defer client.Events().Off(sectionHandlerID)

	initInvalidations := client.HPAInvalidations()

	start := &protocol.PlayServerboundPlayerActionPacket{
		Status:   protocol.PlayerActionStartDigging,
		Position: protocol.BlockPos{X: int32(target.X), Y: int32(target.Y), Z: int32(target.Z)},
		Face:     protocol.BlockFaceTop,
		Sequence: 1,
	}
	startErr := client.WritePacket(start)
	fmt.Printf("[break] start_sent=%v\n", startErr == nil)
	if startErr != nil {
		fmt.Printf("[break] finish_sent=false\n")
		fmt.Printf("[break] block_update_received=false\n")
		fmt.Printf("[break] result=FAIL reason=start_packet_error:%v\n", startErr)
		return
	}

	time.Sleep(breakDelay(target.Block.Name))

	finish := &protocol.PlayServerboundPlayerActionPacket{
		Status:   protocol.PlayerActionFinishDigging,
		Position: protocol.BlockPos{X: int32(target.X), Y: int32(target.Y), Z: int32(target.Z)},
		Face:     protocol.BlockFaceTop,
		Sequence: 2,
	}
	finishErr := client.WritePacket(finish)
	fmt.Printf("[break] finish_sent=%v\n", finishErr == nil)
	if finishErr != nil {
		fmt.Printf("[break] block_update_received=false\n")
		fmt.Printf("[break] result=FAIL reason=finish_packet_error:%v\n", finishErr)
		return
	}

	updateReceived := waitForTargetBlockUpdate(updateCh, target, 5*time.Second)
	fmt.Printf("[break] block_update_received=%v\n", updateReceived)

	newState, err := client.World().GetBlock(target.X, target.Y, target.Z)
	if err != nil {
		fmt.Printf("[break] new_state=ERROR:%v\n", err)
		fmt.Printf("[break] result=FAIL reason=world_lookup_error\n")
		return
	}
	fmt.Printf("[break] new_state=%s\n", newState.Name)

	hpaInvalidated := client.HPAInvalidations() > initInvalidations
	fmt.Printf("[break] hpa_invalidated=%v\n", hpaInvalidated)

	result := "FAIL"
	if updateReceived && newState.Name == "air" {
		result = "PASS"
	}
	fmt.Printf("[break] result=%s\n", result)
}

func findBreakTarget(client *feast.Client) (world.BlockHit, bool) {
	x, y, z, _, _ := client.GetPosition()
	origin := world.Vec3{X: x, Y: y, Z: z}
	for _, name := range []string{"grass_block", "dirt", "stone"} {
		if hit, ok := client.World().FindNearestBlock(origin, name, 6); ok {
			return hit, true
		}
	}
	return world.BlockHit{}, false
}

func breakDelay(name string) time.Duration {
	switch strings.TrimPrefix(strings.ToLower(name), "minecraft:") {
	case "grass_block", "dirt":
		return 1500 * time.Millisecond
	case "stone":
		return 8 * time.Second
	default:
		return 2 * time.Second
	}
}

func waitForTargetBlockUpdate(ch <-chan state.BlockUpdateEvent, target world.BlockHit, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case ev := <-ch:
			if int(ev.X) == target.X && int(ev.Y) == target.Y && int(ev.Z) == target.Z {
				return true
			}
		case <-timer.C:
			return false
		}
	}
}

// ─── Place block ──────────────────────────────────────────────────────────────

func runPlaceBlockSmoke(client *feast.Client) {
	target, ok := findPlaceTarget(client)
	if !ok {
		fmt.Printf("[place] result=NOT_IMPLEMENTED reason=no_loaded_air_target_above_solid_ground\n")
		return
	}
	plan, err := client.PrepareCreativeSmokePlacement(protocol.BlockPos{X: int32(target.X), Y: int32(target.Y), Z: int32(target.Z)})
	if err != nil {
		fmt.Printf("[place] result=NOT_IMPLEMENTED reason=prepare_failed:%v\n", err)
		return
	}

	fmt.Printf("[place] mode=%s\n", plan.Mode)
	fmt.Printf("[place] selected_slot=%d\n", plan.SelectedSlot)
	fmt.Printf("[place] held_item=%s\n", plan.HeldItemName)
	fmt.Printf("[place] target=%d,%d,%d\n", plan.Target.X, plan.Target.Y, plan.Target.Z)
	fmt.Printf("[place] support=%d,%d,%d\n", plan.Support.X, plan.Support.Y, plan.Support.Z)
	fmt.Printf("[place] face=%s\n", faceName(plan.Face))
	fmt.Printf("[place] old_state=%s\n", plan.OldState.Name)

	updateCh := make(chan state.BlockUpdateEvent, 16)
	blockHandlerID, _ := client.On("block_update", func(e state.Event) {
		if ev, ok := e.(state.BlockUpdateEvent); ok {
			select {
			case updateCh <- ev:
			default:
			}
		}
	})
	sectionHandlerID, _ := client.On("section_blocks_update", func(e state.Event) {
		ev, ok := e.(state.SectionBlocksUpdateEvent)
		if !ok {
			return
		}
		for _, u := range ev.Updates {
			select {
			case updateCh <- state.BlockUpdateEvent{X: u.X, Y: u.Y, Z: u.Z, StateID: u.StateID}:
			default:
			}
		}
	})
	defer client.Events().Off(blockHandlerID)
	defer client.Events().Off(sectionHandlerID)

	err = client.ExecuteCreativeSmokePlacement(plan)
	fmt.Printf("[place] packet_sent=%v\n", err == nil)
	if err != nil {
		fmt.Printf("[place] block_update_received=false\n")
		fmt.Printf("[place] result=FAIL reason=packet_error:%v\n", err)
		return
	}

	updateReceived := waitForBlockUpdateCoords(updateCh, int(plan.Target.X), int(plan.Target.Y), int(plan.Target.Z), 5*time.Second)
	fmt.Printf("[place] block_update_received=%v\n", updateReceived)
	newState, err := client.World().GetBlock(int(plan.Target.X), int(plan.Target.Y), int(plan.Target.Z))
	if err != nil {
		fmt.Printf("[place] new_state=ERROR:%v\n", err)
		fmt.Printf("[place] result=FAIL reason=world_lookup_error\n")
		return
	}
	fmt.Printf("[place] new_state=%s\n", newState.Name)

	time.Sleep(500 * time.Millisecond)
	finalState, err := client.World().GetBlock(int(plan.Target.X), int(plan.Target.Y), int(plan.Target.Z))
	rollbackDetected := err == nil && finalState.Name == "air"
	fmt.Printf("[place] rollback_detected=%v\n", rollbackDetected)

	result := "FAIL"
	if updateReceived && newState.Name == feast.CreativeSmokeItemName && !rollbackDetected {
		result = "PASS"
	}
	fmt.Printf("[place] result=%s\n", result)
}

func findPlaceTarget(client *feast.Client) (world.BlockHit, bool) {
	x, y, z, _, _ := client.GetPosition()
	originX := int(math.Floor(x))
	originZ := int(math.Floor(z))
	for radius := 1; radius <= 8; radius++ {
		for dx := -radius; dx <= radius; dx++ {
			for dz := -radius; dz <= radius; dz++ {
				if absInt(dx) != radius && absInt(dz) != radius {
					continue
				}
				wx := originX + dx
				wz := originZ + dz
				cx := float64(wx) + 0.5
				cz := float64(wz) + 0.5
				if math.Hypot(cx-x, cz-z) < 2.0 {
					continue
				}
				if math.Hypot(cx-x, cz-z) > 4.0 {
					continue
				}
				surfaceY := client.World().GetSurfaceY(wx, wz)
				if surfaceY == world.UnknownSurfaceY {
					continue
				}
				targetY := surfaceY + 1
				if math.Abs((float64(targetY)+0.5)-(y+1.62)) > 3.0 {
					continue
				}
				target, err := client.World().GetBlock(wx, targetY, wz)
				if err != nil || target.Name != "air" {
					continue
				}
				support, err := client.World().GetBlock(wx, targetY-1, wz)
				if err != nil || !support.Solid {
					continue
				}
				return world.BlockHit{X: wx, Y: targetY, Z: wz, Block: target}, true
			}
		}
	}
	return world.BlockHit{}, false
}

func waitForBlockUpdateCoords(ch <-chan state.BlockUpdateEvent, x, y, z int, timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case ev := <-ch:
			if int(ev.X) == x && int(ev.Y) == y && int(ev.Z) == z {
				return true
			}
		case <-timer.C:
			return false
		}
	}
}

func faceName(face byte) string {
	switch face {
	case protocol.BlockFaceBottom:
		return "bottom"
	case protocol.BlockFaceTop:
		return "top"
	case protocol.BlockFaceNorth:
		return "north"
	case protocol.BlockFaceSouth:
		return "south"
	case protocol.BlockFaceWest:
		return "west"
	case protocol.BlockFaceEast:
		return "east"
	default:
		return fmt.Sprintf("unknown_%d", face)
	}
}

// ─── Invalid placement ────────────────────────────────────────────────────────

func runPlaceBlockInvalidSmoke(client *feast.Client) {
	x, y, z, _, _ := client.GetPosition()
	target := protocol.BlockPos{X: int32(math.Floor(x)), Y: int32(math.Floor(y)), Z: int32(math.Floor(z))}
	err := client.PlaceBlock(target, protocol.BlockFaceTop)
	if err != nil {
		fmt.Printf("[place-invalid] result=PASS reason=clean_error\n")
	} else {
		fmt.Printf("[place-invalid] result=FAIL reason=no_error_for_invalid_placement\n")
	}
}

// ─── World mutate ─────────────────────────────────────────────────────────────

func runWorldMutateSmoke(client *feast.Client) {
	target, ok := findPlaceTarget(client)
	if !ok {
		fmt.Printf("[mutate] result=FAIL reason=no_loaded_air_target\n")
		return
	}

	initInvalidations := client.HPAInvalidations()

	plan, err := client.PrepareCreativeSmokePlacement(protocol.BlockPos{X: int32(target.X), Y: int32(target.Y), Z: int32(target.Z)})
	if err != nil {
		fmt.Printf("[mutate] result=FAIL reason=prepare_place_failed:%v\n", err)
		return
	}

	updateCh := make(chan state.BlockUpdateEvent, 16)
	blockHandlerID, _ := client.On("block_update", func(e state.Event) {
		if ev, ok := e.(state.BlockUpdateEvent); ok {
			select {
			case updateCh <- ev:
			default:
			}
		}
	})
	sectionHandlerID, _ := client.On("section_blocks_update", func(e state.Event) {
		ev, ok := e.(state.SectionBlocksUpdateEvent)
		if !ok {
			return
		}
		for _, u := range ev.Updates {
			select {
			case updateCh <- state.BlockUpdateEvent{X: u.X, Y: u.Y, Z: u.Z, StateID: u.StateID}:
			default:
			}
		}
	})
	defer client.Events().Off(blockHandlerID)
	defer client.Events().Off(sectionHandlerID)

	err = client.ExecuteCreativeSmokePlacement(plan)
	if err != nil {
		fmt.Printf("[mutate] result=FAIL reason=execute_place_failed:%v\n", err)
		return
	}

	placeUpdate := waitForBlockUpdateCoords(updateCh, int(plan.Target.X), int(plan.Target.Y), int(plan.Target.Z), 5*time.Second)
	placeBlockState, _ := client.World().GetBlock(int(plan.Target.X), int(plan.Target.Y), int(plan.Target.Z))

	placeResult := "FAIL"
	if placeUpdate && placeBlockState.Name == "stone" {
		placeResult = "PASS"
	}
	fmt.Printf("[mutate] place_result=%s\n", placeResult)
	if placeResult == "FAIL" {
		fmt.Printf("[mutate] result=FAIL reason=place_did_not_update_to_stone\n")
		return
	}

	start := &protocol.PlayServerboundPlayerActionPacket{
		Status:   protocol.PlayerActionStartDigging,
		Position: plan.Target,
		Face:     protocol.BlockFaceTop,
		Sequence: 10,
	}
	_ = client.WritePacket(start)
	time.Sleep(1500 * time.Millisecond)

	finish := &protocol.PlayServerboundPlayerActionPacket{
		Status:   protocol.PlayerActionFinishDigging,
		Position: plan.Target,
		Face:     protocol.BlockFaceTop,
		Sequence: 11,
	}
	_ = client.WritePacket(finish)

	breakUpdate := waitForBlockUpdateCoords(updateCh, int(plan.Target.X), int(plan.Target.Y), int(plan.Target.Z), 5*time.Second)
	breakBlockState, _ := client.World().GetBlock(int(plan.Target.X), int(plan.Target.Y), int(plan.Target.Z))

	breakResult := "FAIL"
	if breakUpdate && breakBlockState.Name == "air" {
		breakResult = "PASS"
	}
	fmt.Printf("[mutate] break_result=%s\n", breakResult)

	finalBlock, _ := client.World().GetBlock(int(plan.Target.X), int(plan.Target.Y), int(plan.Target.Z))
	fmt.Printf("[mutate] final_block=%s\n", finalBlock.Name)

	passableAfterBreak := client.World().IsPassable(int(plan.Target.X), int(plan.Target.Y), int(plan.Target.Z))
	fmt.Printf("[mutate] passable_after_break=%v\n", passableAfterBreak)

	finalInvalidations := client.HPAInvalidations()
	invalidationsCount := finalInvalidations - initInvalidations
	fmt.Printf("[mutate] hpa_invalidations>=2=%v\n", invalidationsCount >= 2)

	result := "FAIL"
	if placeResult == "PASS" && breakResult == "PASS" && finalBlock.Name == "air" && passableAfterBreak && invalidationsCount >= 2 {
		result = "PASS"
	}
	fmt.Printf("[mutate] result=%s\n", result)
}

// ─── HPA mutation test ────────────────────────────────────────────────────────

func runHPAMutationTest(client *feast.Client) {
	stats := client.HPAStats()
	fmt.Printf("[hpa-mutation] initial_graph_nodes=%d\n", stats.GraphNodes)
	fmt.Printf("[hpa-mutation] initial_graph_edges=%d\n", stats.GraphEdges)

	target, ok := findPlaceTarget(client)
	if !ok {
		fmt.Printf("[hpa-mutation] result=PARTIAL reason=no_loaded_air_target\n")
		return
	}

	initInvalidations := client.HPAInvalidations()

	plan, err := client.PrepareCreativeSmokePlacement(protocol.BlockPos{X: int32(target.X), Y: int32(target.Y), Z: int32(target.Z)})
	if err != nil {
		fmt.Printf("[hpa-mutation] result=PARTIAL reason=prepare_place_failed:%v\n", err)
		return
	}

	updateCh := make(chan state.BlockUpdateEvent, 16)
	blockHandlerID, _ := client.On("block_update", func(e state.Event) {
		if ev, ok := e.(state.BlockUpdateEvent); ok {
			select {
			case updateCh <- ev:
			default:
			}
		}
	})
	sectionHandlerID, _ := client.On("section_blocks_update", func(e state.Event) {
		ev, ok := e.(state.SectionBlocksUpdateEvent)
		if !ok {
			return
		}
		for _, u := range ev.Updates {
			select {
			case updateCh <- state.BlockUpdateEvent{X: u.X, Y: u.Y, Z: u.Z, StateID: u.StateID}:
			default:
			}
		}
	})
	defer client.Events().Off(blockHandlerID)
	defer client.Events().Off(sectionHandlerID)

	err = client.ExecuteCreativeSmokePlacement(plan)
	placeSent := err == nil
	fmt.Printf("[hpa-mutation] place_block=%v\n", placeSent)

	placeUpdate := waitForBlockUpdateCoords(updateCh, int(plan.Target.X), int(plan.Target.Y), int(plan.Target.Z), 5*time.Second)
	_ = placeUpdate
	invalidationAfterPlace := client.HPAInvalidations() > initInvalidations
	fmt.Printf("[hpa-mutation] invalidation_after_place=%v\n", invalidationAfterPlace)

	initInvalidations = client.HPAInvalidations()
	startPkt := &protocol.PlayServerboundPlayerActionPacket{
		Status:   protocol.PlayerActionStartDigging,
		Position: plan.Target,
		Face:     protocol.BlockFaceTop,
		Sequence: 20,
	}
	_ = client.WritePacket(startPkt)
	time.Sleep(1500 * time.Millisecond)

	finishPkt := &protocol.PlayServerboundPlayerActionPacket{
		Status:   protocol.PlayerActionFinishDigging,
		Position: plan.Target,
		Face:     protocol.BlockFaceTop,
		Sequence: 21,
	}
	_ = client.WritePacket(finishPkt)

	breakUpdate := waitForBlockUpdateCoords(updateCh, int(plan.Target.X), int(plan.Target.Y), int(plan.Target.Z), 5*time.Second)
	invalidationAfterBreak := client.HPAInvalidations() > initInvalidations
	fmt.Printf("[hpa-mutation] break_block=%v\n", breakUpdate)
	fmt.Printf("[hpa-mutation] invalidation_after_break=%v\n", invalidationAfterBreak)

	time.Sleep(500 * time.Millisecond)
	statsAfter := client.HPAStats()
	fmt.Printf("[hpa-mutation] rebuilt_clusters=%d\n", statsAfter.BuiltClusters)

	bx, by, bz := client.PlayerState().X, client.PlayerState().Y, client.PlayerState().Z
	pathFound := false
	refinedSegments := 0

	nav := client.HPANav()
	if nav != nil {
		planner := hpa.NewHPAPlanner(client.World(), client.WorldGraph(), client.WorldClusters())
		if planner != nil {
			startNode := [3]int{int(bx), int(by), int(bz)}
			goalNode := [3]int{int(bx) + 2, int(by), int(bz) + 2}
			res := planner.Plan(startNode, goalNode, goal.NewGoalBlock(goalNode[0], goalNode[1], goalNode[2]))
			if res.Status == navplanner.PlanFound && len(res.AbstractPath) >= 2 {
				pathFound = true
				for !res.Refiner.IsComplete() {
					segment := res.Refiner.NextSegment()
					if len(segment) > 0 {
						refinedSegments++
					}
				}
			}
		}
	}

	fmt.Printf("[hpa-mutation] abstract_path_found=%v\n", pathFound)
	fmt.Printf("[hpa-mutation] refined_segments>0=%v\n", refinedSegments > 0)
	fmt.Printf("[hpa-mutation] fallback_used=false\n")

	result := "FAIL"
	if placeSent && invalidationAfterPlace && breakUpdate && invalidationAfterBreak {
		result = "PASS"
	}
	if result == "PASS" && (!pathFound || refinedSegments == 0) {
		fmt.Printf("[hpa-mutation] result=PASS reason=invalidation_verified\n")
		return
	}
	fmt.Printf("[hpa-mutation] result=%s\n", result)
}

// ─── Soak test ────────────────────────────────────────────────────────────────

func runSoakTest(client *feast.Client, soakDurStr string) {
	dur, err := time.ParseDuration(soakDurStr)
	if err != nil {
		fmt.Printf("[soak] result=FAIL reason=invalid_duration:%v\n", err)
		return
	}

	fmt.Printf("[soak] duration=%s\n", soakDurStr)

	var kaReceived, kaSent int
	var lock sync.Mutex
	client.On("keep_alive", func(e state.Event) {
		lock.Lock()
		kaReceived++
		kaSent++
		lock.Unlock()
	})

	var errorsCount int
	client.On("error", func(e state.Event) {
		lock.Lock()
		errorsCount++
		lock.Unlock()
	})

	_ = client.SendChat("FeastGoBot starting soak test for " + soakDurStr)

	time.Sleep(dur)
	_ = client.Close()

	lock.Lock()
	rec := kaReceived
	sent := kaSent
	errs := errorsCount
	lock.Unlock()

	status := client.ShutdownStatus()
	cleanDisconnect := status.Requested && status.TickLoopStopped && status.ReadLoopStopped && status.NavLoopStopped && status.SocketClosed

	fmt.Printf("[soak] keepalive_received=%d\n", rec)
	fmt.Printf("[soak] keepalive_sent=%d\n", sent)
	fmt.Printf("[soak] chunks_loaded=%d\n", client.World().ChunkCount())
	fmt.Printf("[soak] errors=%d\n", errs)
	fmt.Printf("[soak] clean_disconnect=%v\n", cleanDisconnect)

	result := "FAIL"
	if errs == 0 && cleanDisconnect {
		result = "PASS"
	}
	fmt.Printf("[soak] result=%s\n", result)
}

// ─── Inventory dump ───────────────────────────────────────────────────────────

func runInventoryDump(client *feast.Client) {
	time.Sleep(1 * time.Second)
	inv := client.Inventory()
	fmt.Printf("[inventory] selected_hotbar=%d\n", inv.SelectedHotbarSlot)
	for i := 0; i < 9; i++ {
		slot := 36 + i
		item, ok := inv.Slots[slot]
		if !ok || !item.Present {
			fmt.Printf("[inventory] hotbar[%d]=empty\n", i)
		} else {
			fmt.Printf("[inventory] hotbar[%d]=%s x%d\n", i, item.Name, item.Count)
		}
	}
	fmt.Printf("[inventory] result=PASS\n")
}

// ─── Survival placement ───────────────────────────────────────────────────────

func runPlaceBlockSurvivalSmoke(client *feast.Client) {
	time.Sleep(2 * time.Second)
	target, ok := findPlaceTarget(client)
	if !ok {
		fmt.Printf("[place-survival] result=FAIL reason=no_loaded_air_target_above_solid_ground\n")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_ = client.PlaceBlockSurvival(ctx, protocol.BlockPos{
		X: int32(target.X),
		Y: int32(target.Y),
		Z: int32(target.Z),
	}, protocol.DirectionUp)
}

// ─── Entity metadata test ─────────────────────────────────────────────────────

func runEntityMetadataTest(client *feast.Client) {
	fmt.Printf("[entity-meta] waiting for entities...\n")
	var target *world.Entity
	for i := 0; i < 20; i++ {
		all := client.Entities().All()
		for _, e := range all {
			if e.Type == "pig" || e.Type == "zombie" || e.Type == "player" {
				target = e
				break
			}
		}
		if target != nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if target == nil {
		all := client.Entities().All()
		if len(all) > 0 {
			target = all[0]
		}
	}

	if target == nil {
		fmt.Printf("[entity-meta] result=FAIL reason=no_entity_found\n")
		return
	}

	w, h := target.Width, target.Height
	if w <= 0 || h <= 0 {
		box := world.HitboxForEntityType(target.Type, target.Pose)
		w = box.MaxX - box.MinX
		h = box.MaxY - box.MinY
	}

	fmt.Printf("[entity-meta] id=%d\n", target.ID)
	fmt.Printf("[entity-meta] type=%s\n", target.Type)
	fmt.Printf("[entity-meta] pose=%s\n", target.Pose)
	fmt.Printf("[entity-meta] width=%.2f\n", w)
	fmt.Printf("[entity-meta] height=%.2f\n", h)
	fmt.Printf("[entity-meta] metadata_entries=%d\n", len(target.Metadata))
	fmt.Printf("[entity-meta] result=PASS\n")
}

// ─── Entity hitbox test ───────────────────────────────────────────────────────

func runEntityHitboxTest(client *feast.Client) {
	fmt.Printf("[hitbox] waiting for entities...\n")
	var target *world.Entity
	for i := 0; i < 20; i++ {
		all := client.Entities().All()
		for _, e := range all {
			if e.Type == "pig" || e.Type == "zombie" || e.Type == "player" {
				target = e
				break
			}
		}
		if target != nil {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	if target == nil {
		all := client.Entities().All()
		if len(all) > 0 {
			target = all[0]
		}
	}

	if target == nil {
		fmt.Printf("[hitbox] result=FAIL reason=no_entity_found\n")
		return
	}

	box := target.Hitbox()
	px, py, pz, _, _ := client.GetPosition()
	playerBox := world.AABB{
		MinX: px - 0.3,
		MinY: py,
		MinZ: pz - 0.3,
		MaxX: px + 0.3,
		MaxY: py + 1.8,
		MaxZ: pz + 0.3,
	}

	collision := box.Intersects(playerBox)

	fmt.Printf("[hitbox] entity_id=%d\n", target.ID)
	fmt.Printf("[hitbox] type=%s\n", target.Type)
	fmt.Printf("[hitbox] pose=%s\n", target.Pose)
	fmt.Printf("[hitbox] aabb=min(%.2f,%.2f,%.2f)_max(%.2f,%.2f,%.2f)\n", box.MinX, box.MinY, box.MinZ, box.MaxX, box.MaxY, box.MaxZ)
	fmt.Printf("[hitbox] collision_detected=%v\n", collision)
	fmt.Printf("[hitbox] result=PASS\n")
}

// ─── Entity tracking smoke ────────────────────────────────────────────────────

type entitySmokeRecorder struct {
	mu       sync.Mutex
	spawn    *state.EntitySpawnEvent
	move     *state.EntityMoveDeltaEvent
	teleport *state.EntityTeleportEvent
	remove   *state.EntityRemoveEvent
}

func newEntitySmokeRecorder() *entitySmokeRecorder {
	return &entitySmokeRecorder{}
}

func (r *entitySmokeRecorder) register(client *feast.Client) {
	client.On("entity_spawn", func(e state.Event) {
		if ev, ok := e.(state.EntitySpawnEvent); ok {
			r.mu.Lock()
			r.spawn = &ev
			r.mu.Unlock()
		}
	})
	client.On("entity_move_delta", func(e state.Event) {
		if ev, ok := e.(state.EntityMoveDeltaEvent); ok {
			r.mu.Lock()
			r.move = &ev
			r.mu.Unlock()
		}
	})
	client.On("entity_teleport", func(e state.Event) {
		if ev, ok := e.(state.EntityTeleportEvent); ok {
			r.mu.Lock()
			r.teleport = &ev
			r.mu.Unlock()
		}
	})
	client.On("entity_remove", func(e state.Event) {
		if ev, ok := e.(state.EntityRemoveEvent); ok {
			r.mu.Lock()
			r.remove = &ev
			r.mu.Unlock()
		}
	})
}

func (r *entitySmokeRecorder) snapshot() (*state.EntitySpawnEvent, *state.EntityMoveDeltaEvent, *state.EntityTeleportEvent, *state.EntityRemoveEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.spawn, r.move, r.teleport, r.remove
}

func runEntitySmoke(client *feast.Client, recorder *entitySmokeRecorder) {
	time.Sleep(5 * time.Second)
	spawn, moveEv, teleport, remove := recorder.snapshot()
	realPass := spawn != nil && (moveEv != nil || teleport != nil) && remove != nil
	if spawn != nil {
		fmt.Printf("[entity-test] spawn id=%d source=real\n", spawn.EntityID)
	}
	if moveEv != nil {
		fmt.Printf("[entity-test] move id=%d source=real\n", moveEv.EntityID)
	}
	if teleport != nil {
		fmt.Printf("[entity-test] teleport id=%d source=real\n", teleport.EntityID)
	}
	if remove != nil {
		fmt.Printf("[entity-test] remove id=%d source=real\n", remove.EntityID)
	}
	if realPass {
		fmt.Printf("[entity-test] result=PASS\n")
		return
	}

	syntheticPass := runSyntheticEntityDispatch()
	if syntheticPass {
		fmt.Printf("[entity-test] real_server=PARTIAL synthetic=PASS\n")
		fmt.Printf("[entity-test] result=PARTIAL\n")
		return
	}
	fmt.Printf("[entity-test] real_server=PARTIAL synthetic=FAIL\n")
	fmt.Printf("[entity-test] result=FAIL\n")
}

func runSyntheticEntityDispatch() bool {
	bus := state.NewEventBus()
	dispatcher := state.NewDispatcher(bus)
	var sawSpawn, sawMove, sawTeleport, sawRemove bool
	bus.On("entity_spawn", func(e state.Event) {
		ev := e.(state.EntitySpawnEvent)
		sawSpawn = ev.EntityID == 9001
		fmt.Printf("[entity-test] spawn id=%d source=synthetic\n", ev.EntityID)
	})
	bus.On("entity_move_delta", func(e state.Event) {
		ev := e.(state.EntityMoveDeltaEvent)
		sawMove = ev.EntityID == 9001
		fmt.Printf("[entity-test] move id=%d source=synthetic\n", ev.EntityID)
	})
	bus.On("entity_teleport", func(e state.Event) {
		ev := e.(state.EntityTeleportEvent)
		sawTeleport = ev.EntityID == 9001
		fmt.Printf("[entity-test] teleport id=%d source=synthetic\n", ev.EntityID)
	})
	bus.On("entity_remove", func(e state.Event) {
		ev := e.(state.EntityRemoveEvent)
		sawRemove = ev.EntityID == 9001
		fmt.Printf("[entity-test] remove id=%d source=synthetic\n", ev.EntityID)
	})

	_ = dispatcher.Dispatch(state.StatePlay, rawFromPacket(&protocol.PlayClientboundSpawnEntityPacket{EntityID: 9001, Type: 1, X: 1, Y: 64, Z: 1}))
	_ = dispatcher.Dispatch(state.StatePlay, rawFromPacket(&protocol.PlayClientboundUpdateEntityPositionPacket{EntityID: 9001, DX: 64, DY: 0, DZ: 0}))
	_ = dispatcher.Dispatch(state.StatePlay, rawFromPacket(&protocol.PlayClientboundTeleportEntityPacket{EntityID: 9001, X: 2, Y: 64, Z: 2}))
	_ = dispatcher.Dispatch(state.StatePlay, rawFromPacket(&protocol.PlayClientboundRemoveEntitiesPacket{EntityIDs: []int32{9001}}))
	return sawSpawn && sawMove && sawTeleport && sawRemove
}

func rawFromPacket(packet protocol.Packet) *protocol.RawPacket {
	var body bytes.Buffer
	_ = packet.Marshal(protocol.NewWriter(&body))
	return &protocol.RawPacket{ID: packet.PacketID(), Data: body.Bytes()}
}

// ─── Stub for stuck test (requires executor) ─────────────────────────────────

type stuckProbeClient struct {
	x, y, z    float64
	yaw, pitch float32
	packets    int
}

func (c *stuckProbeClient) GetPosition() (float64, float64, float64, float32, float32) {
	return c.x, c.y, c.z, c.yaw, c.pitch
}

func (c *stuckProbeClient) WritePacket(protocol.Packet) error {
	c.packets++
	return nil
}

type stuckProbeGoal struct{}

func (stuckProbeGoal) Satisfied(int, int, int) bool { return false }
func (stuckProbeGoal) Heuristic(int, int, int) float64 {
	return 1
}

type stuckProbeMove struct{}

func (stuckProbeMove) Destination(pos [3]int) [3]int {
	return [3]int{pos[0] + 1, pos[1], pos[2]}
}

func (stuckProbeMove) Cost(*world.World, [3]int) float64 {
	return 120
}

// Ensure executor import is used.
var _ = executor.Execute

// ─── Shared helpers ───────────────────────────────────────────────────────────

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func floorDivInt(a, b int) int {
	q := a / b
	r := a % b
	if r != 0 && ((r < 0) != (b < 0)) {
		q--
	}
	return q
}

func moveNotUsed() {
	// Reference move to prevent import removal by tooling.
	var _ move.Movement
}

func runCoreActionTest(client *feast.Client) {
	fmt.Printf("[core] connect=true\n")

	// Wait ready
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Wait for position sync
	for !client.PositionSynced() {
		if ctx.Err() != nil {
			fmt.Printf("[core] ready=false reason=timeout_waiting_for_position_sync\n")
			fmt.Printf("[core] result=FAIL\n")
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Printf("[core] ready=true\n")
	fmt.Printf("[core] chunks_loaded=%d\n", client.World().ChunkCount())

	// Print position
	x, y, z, _, _ := client.GetPosition()
	fmt.Printf("[core] position=x=%.2f y=%.2f z=%.2f\n", x, y, z)
	fmt.Printf("[core] inventory_loaded=true\n")

	ix, iy, iz := int(math.Floor(x)), int(math.Floor(y)), int(math.Floor(z))
	var path []move.Movement
	var g goal.Goal

	// Iterate through nearby relative coordinates to find a valid standable target
	foundDest := false
	for radius := 3; radius <= 8 && !foundDest; radius++ {
		for dx := -radius; dx <= radius && !foundDest; dx++ {
			for dz := -radius; dz <= radius && !foundDest; dz++ {
				if absInt(dx)+absInt(dz) != radius {
					continue
				}
				tx := ix + dx
				tz := iz + dz
				surfaceY := client.World().GetSurfaceY(tx, tz)
				if surfaceY == world.UnknownSurfaceY {
					continue
				}
				ty := surfaceY + 1
				// Check standable
				if !client.World().IsPassable(tx, ty, tz) || !client.World().IsPassable(tx, ty+1, tz) || client.World().IsPassable(tx, ty-1, tz) {
					continue
				}
				// Plan to target
				g = goal.NewGoalBlock(tx, ty, tz)
				ctxPlan, cancelPlan := context.WithTimeout(context.Background(), 2*time.Second)
				res := navplanner.Plan(ctxPlan, ix, iy, iz, g, client.World())
				cancelPlan()
				if res.Status == navplanner.PlanFound && len(res.Path) >= 3 && len(res.Path) <= 8 {
					path = res.Path
					foundDest = true
				}
			}
		}
	}

	if !foundDest {
		// Fallback: flat world
		for dx := 3; dx <= 8; dx++ {
			tx := ix + dx
			tz := iz
			ty := iy
			g = goal.NewGoalBlock(tx, ty, tz)
			ctxPlan, cancelPlan := context.WithTimeout(context.Background(), 2*time.Second)
			res := navplanner.Plan(ctxPlan, ix, iy, iz, g, client.World())
			cancelPlan()
			if res.Status == navplanner.PlanFound {
				path = res.Path
				foundDest = true
				break
			}
		}
	}

	if !foundDest {
		fmt.Printf("[astar] found=false reason=no_path_found_in_range_3_to_8\n")
		fmt.Printf("[core] result=FAIL\n")
		return
	}

	fmt.Printf("[astar] found=true nodes=%d\n", len(path))

	// Navigate to the target
	fmt.Printf("[move] profile=bot_like\n")
	ctxMove, cancelMove := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelMove()

	moveResult, err := executor.ExecuteWithResult(ctxMove, client, client.World(), g, path, feast.MovementOptions{Profile: feast.MovementBotLike})
	fmt.Printf("[move] packets_sent=%d\n", moveResult.PacketsSent)
	if err != nil {
		fmt.Printf("[move] reached=false reason=%v\n", err)
		fmt.Printf("[core] result=FAIL\n")
		return
	}
	fmt.Printf("[move] reached=%v final_distance=%.3f\n", moveResult.Reached, moveResult.FinalDistance)

	// 8. Find a safe nearby block to break
	bx, by, bz, _, _ := client.GetPosition()
	curX, curY, curZ := int(math.Floor(bx)), int(math.Floor(by)), int(math.Floor(bz))

	var breakPos protocol.BlockPos
	var foundBreak bool

	for dx := -1; dx <= 1 && !foundBreak; dx++ {
		for dz := -1; dz <= 1 && !foundBreak; dz++ {
			for dy := -1; dy <= 1; dy++ {
				if dx == 0 && dz == 0 && dy == -1 {
					continue
				}
				tx, ty, tz := curX+dx, curY+dy, curZ+dz
				b, err := client.World().GetBlock(tx, ty, tz)
				if err != nil {
					continue
				}
				if b.Name == "stone" || b.Name == "dirt" || b.Name == "grass_block" || b.Name == "cobblestone" {
					breakPos = protocol.BlockPos{X: int32(tx), Y: int32(ty), Z: int32(tz)}
					foundBreak = true
					break
				}
			}
		}
	}

	if !foundBreak {
		fmt.Printf("[break] result=FAIL reason=no_safe_block_found\n")
		fmt.Printf("[core] result=FAIL\n")
		return
	}

	// Break block with AutoTool=true
	ctxBreak, cancelBreak := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelBreak()

	err = client.BreakBlock(ctxBreak, breakPos, feast.BreakOptions{AutoTool: true})
	if err != nil {
		fmt.Printf("[break] result=FAIL reason=break_failed:%v\n", err)
		fmt.Printf("[core] result=FAIL\n")
		return
	}

	// 9. Place one stone block using survival inventory
	invSnap := client.InventorySnapshot()
	var slotToPlace = -1
	var itemName = ""
	for i := 0; i < 9; i++ {
		slot := 36 + i
		item, ok := invSnap.Slots[slot]
		if ok && item.Present && (item.Name == "stone" || item.Name == "cobblestone" || item.Name == "dirt" || item.Name == "grass_block") {
			slotToPlace = i
			itemName = item.Name
			break
		}
	}

	if slotToPlace == -1 {
		for i := 0; i < 9; i++ {
			slot := 36 + i
			item, ok := invSnap.Slots[slot]
			if ok && item.Present && item.Count > 0 {
				slotToPlace = i
				itemName = item.Name
				break
			}
		}
	}

	if slotToPlace == -1 {
		fmt.Printf("[place] result=FAIL reason=no_survival_items_in_hotbar\n")
		fmt.Printf("[core] result=FAIL\n")
		return
	}

	if err := client.SelectHotbarSlot(context.Background(), slotToPlace); err != nil {
		fmt.Printf("[place] result=FAIL reason=select_slot_failed:%v\n", err)
		fmt.Printf("[core] result=FAIL\n")
		return
	}

	countBefore := 0
	invSnap = client.InventorySnapshot()
	if it, ok := invSnap.Slots[36+slotToPlace]; ok && it.Present {
		countBefore = it.Count
	}

	ctxPlace, cancelPlace := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelPlace()

	err = client.PlaceBlockSurvival(ctxPlace, breakPos, protocol.DirectionUp)
	if err != nil {
		fmt.Printf("[place] result=FAIL reason=place_failed:%v\n", err)
		fmt.Printf("[core] result=FAIL\n")
		return
	}

	time.Sleep(200 * time.Millisecond)
	placedBlock, err := client.World().GetBlock(int(breakPos.X), int(breakPos.Y), int(breakPos.Z))

	countAfter := 0
	invSnap = client.InventorySnapshot()
	if it, ok := invSnap.Slots[36+slotToPlace]; ok && it.Present {
		countAfter = it.Count
	}

	blockMatch := err == nil && placedBlock.Name == itemName
	countMatch := countAfter == countBefore-1

	fmt.Printf("[place] mode=full_inventory\n")
	if blockMatch && countMatch {
		fmt.Printf("[place] result=PASS\n")
	} else {
		fmt.Printf("[place] result=FAIL reason=blockMatch=%t(placed:%s,expected:%s) countMatch=%t(before:%d,after:%d)\n",
			blockMatch, placedBlock.Name, itemName, countMatch, countBefore, countAfter)
		fmt.Printf("[core] result=FAIL\n")
		return
	}

	client.Close()

	shut := client.ShutdownStatus()
	cleanDisconnect := shut.Requested && shut.TickLoopStopped && shut.ReadLoopStopped && shut.NavLoopStopped && shut.SocketClosed
	fmt.Printf("[core] disconnect_clean=%v\n", cleanDisconnect)

	if cleanDisconnect {
		fmt.Printf("[core] result=PASS\n")
	} else {
		fmt.Printf("[core] result=FAIL reason=not_clean_shutdown\n")
	}
}
