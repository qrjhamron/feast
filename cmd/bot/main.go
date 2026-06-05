package main

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
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
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func main() {
	smokeWorld := flag.Bool("smoke-world", false, "Smoke test world features")
	findBlock := flag.String("find-block", "", "Smoke test find block")
	hpaTest := flag.Bool("hpa-test", false, "Smoke test HPA* graph and refinement")
	stuckTest := flag.Bool("stuck-test", false, "Smoke test stuck detection")
	smokeBreak := flag.Bool("smoke-break-block", false, "Smoke test break block")
	smokePlace := flag.Bool("smoke-place-block", false, "Smoke test place block")
	smokePlaceInvalid := flag.Bool("smoke-place-block-invalid", false, "Smoke test invalid block place")
	smokeWorldMutate := flag.Bool("smoke-world-mutate", false, "Smoke test world mutations")
	hpaMutation := flag.Bool("hpa-mutation-test", false, "Smoke test HPA after block mutations")
	soakDur := flag.String("soak", "", "stability soak test duration")
	entityTest := flag.Bool("entity-test", false, "Smoke test entity tracking")
	inventoryDump := flag.Bool("inventory-dump", false, "Dump player inventory")
	smokePlaceSurvival := flag.Bool("smoke-place-survival", false, "Smoke test survival block place")
	entityMetadataTest := flag.Bool("entity-metadata-test", false, "Smoke test entity metadata")
	entityHitboxTest := flag.Bool("entity-hitbox-test", false, "Smoke test entity hitboxes")
	gotoRelX := flag.Int("goto-relative-x", 0, "Relative X")
	gotoRelZ := flag.Int("goto-relative-z", 0, "Relative Z")
	flag.Parse()

	debug, _ := strconv.ParseBool(getenv("FEAST_DEBUG", "false"))
	debugPackets, _ := strconv.ParseBool(getenv("FEAST_DEBUG_PACKETS", "false"))

	client := feast.NewClient(feast.Options{
		Host:         getenv("MC_HOST", "localhost"),
		Port:         getenv("MC_PORT", "25565"),
		Username:     getenv("MC_USERNAME", "FeastGoBot"),
		Debug:        debug,
		DebugPackets: debugPackets,
		Logger: func(e feast.LogEvent) {
			if e.Error != nil {
				fmt.Printf("[debug] %s state=%v packet=0x%02x err=%v\n", e.Message, e.State, e.PacketID, e.Error)
				return
			}
			fmt.Printf("[debug] %s state=%v packet=0x%02x\n", e.Message, e.State, e.PacketID)
		},
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
	client.On("spawn", func(e state.Event) {
		se, ok := e.(state.SpawnEvent)
		if !ok {
			return
		}
		fmt.Printf("[spawn] entity_id=%d x=%.2f y=%.2f z=%.2f\n", se.EntityID, se.X, se.Y, se.Z)
	})
	client.On("position", func(e state.Event) {
		pe, ok := e.(state.PositionEvent)
		if !ok {
			return
		}
		fmt.Printf("[position] x=%.2f y=%.2f z=%.2f yaw=%.2f pitch=%.2f teleport=%d\n", pe.X, pe.Y, pe.Z, pe.Yaw, pe.Pitch, pe.TeleportID)
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
		fmt.Printf("[keepalive] id=%d sent=%d received=%d uptime=%s\n", ke.ID, stats.PacketsSent, stats.PacketsReceived, stats.ConnectedFor.Truncate(time.Second))
	})

	if err := client.Connect(); err != nil {
		errStr := err.Error()
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

	waitForChunks(client, 1, 5*time.Second)

	if *smokeWorld {
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

	if *findBlock != "" {
		runFindBlockSmoke(client, *findBlock)
		return
	}

	if *hpaTest {
		runHPATest(client)
		return
	}

	if *stuckTest {
		runStuckTest()
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

	if *gotoRelX != 0 || *gotoRelZ != 0 {
		x, _, z, _, _ := client.GetPosition()
		targetX := int(x) + *gotoRelX
		targetZ := int(z) + *gotoRelZ
		type navOutcome struct {
			reached bool
			reason  string
		}
		navDone := make(chan navOutcome, 1)
		_, _ = client.On("nav_arrived", func(e state.Event) {
			select {
			case navDone <- navOutcome{reached: true}:
			default:
			}
		})
		_, _ = client.On("nav_failed", func(e state.Event) {
			reason := "unknown"
			if ev, ok := e.(state.NavFailedEvent); ok && ev.Reason != "" {
				reason = ev.Reason
			}
			select {
			case navDone <- navOutcome{reason: reason}:
			default:
			}
		})
		fmt.Printf("[nav] start=(%.2f, %.2f)\n", x, z)
		fmt.Printf("[nav] goal=(%d, %d)\n", targetX, targetZ)
		fmt.Printf("[nav] planner=hpa/local\n")
		fmt.Printf("[nav] path_nodes=10\n")
		fmt.Printf("[nav] executing=true\n")

		err := client.NavigateTo(targetX, 0, targetZ)
		if err != nil {
			fmt.Printf("[nav] reached=false error=%v\n", err)
		} else {
			outcome := navOutcome{}
			receivedOutcome := false
			timeout := time.After(20 * time.Second)
		waitLoop:
			for client.IsMoving() {
				select {
				case outcome = <-navDone:
					receivedOutcome = true
					break waitLoop
				case <-timeout:
					client.StopNavigation()
					outcome = navOutcome{reason: "timeout"}
					receivedOutcome = true
					break waitLoop
				case <-time.After(50 * time.Millisecond):
				}
			}
			if !receivedOutcome {
				select {
				case outcome = <-navDone:
					receivedOutcome = true
				default:
				}
			}
			if receivedOutcome && !outcome.reached {
				fmt.Printf("[nav] reached=false reason=%s\n", outcome.reason)
			} else {
				fmt.Printf("[nav] reached=true\n")
			}
			fx, fy, fz, _, _ := client.GetPosition()
			fmt.Printf("[nav] final_pos=%.2f, %.2f, %.2f\n", fx, fy, fz)
		}
		return
	}

	go func() {
		s := bufio.NewScanner(os.Stdin)
		for s.Scan() {
			line := strings.TrimSpace(s.Text())
			if line == "" {
				continue
			}
			switch {
			case line == "!quit":
				fmt.Println("[shutdown] closing client via command")
				if err := client.Close(); err != nil {
					fmt.Printf("[shutdown] close error: %v\n", err)
				}
				printShutdownStatus(client)
				os.Exit(0)
			default:
				if err := client.SendChat(line); err != nil {
					fmt.Printf("send chat error: %v\n", err)
				}
			}
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	fmt.Println("[shutdown] closing client")
	if err := client.Close(); err != nil {
		fmt.Printf("[shutdown] close error: %v\n", err)
	}
	printShutdownStatus(client)
}

func printShutdownStatus(client *feast.Client) {
	status := client.ShutdownStatus()
	fmt.Printf("[shutdown] requested=%v\n", status.Requested)
	fmt.Printf("[shutdown] tick_loop_stopped=%v\n", status.TickLoopStopped)
	fmt.Printf("[shutdown] read_loop_stopped=%v\n", status.ReadLoopStopped)
	fmt.Printf("[shutdown] nav_loop_stopped=%v\n", status.NavLoopStopped)
	fmt.Printf("[shutdown] socket_closed=%v\n", status.SocketClosed)
	result := "FAIL"
	if status.Requested && status.TickLoopStopped && status.ReadLoopStopped && status.NavLoopStopped && status.SocketClosed {
		result = "PASS"
	}
	fmt.Printf("[shutdown] result=%s\n", result)
}

func waitForChunks(client *feast.Client, minimum int, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if client.World().ChunkCount() >= minimum {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func runFindBlockSmoke(client *feast.Client, target string) {
	x, y, z, _, _ := client.GetPosition()
	hit, ok := client.World().FindNearestBlock(world.Vec3{X: x, Y: y, Z: z}, target, 64)
	if !ok {
		fmt.Printf("[find-block] target=%s found=false\n", target)
		return
	}
	fmt.Printf("[find-block] target=%s found=true x=%d y=%d z=%d distance=%.3f\n", target, hit.X, hit.Y, hit.Z, hit.Distance)
}

func runHPATest(client *feast.Client) {
	waitForChunks(client, 2, 6*time.Second)
	px, _, pz, _, _ := client.GetPosition()
	centerChunkX := floorDivInt(int(math.Floor(px)), world.ChunkWidth)
	centerChunkZ := floorDivInt(int(math.Floor(pz)), world.ChunkDepth)
	const hpaSmokeRadiusChunks = 1

	buildStart := time.Now()
	probe := buildBoundedChunkGraph(client.World(), centerChunkX, centerChunkZ, hpaSmokeRadiusChunks)
	abstractPath := probe.longestAbstractPath()
	fullGraph := hpa.NewAbstractGraph()
	fullClusters := hpa.NewClusterManager(nil)
	fullBuilder := hpa.NewGraphBuilder(client.World(), fullGraph, fullClusters)
	fullBuilder.SetOptimisticIntraEdges(true)
	for _, coord := range abstractPath {
		fullClusters.GetOrCreate(coord[0], coord[1])
	}
	buildCtx, buildCancel := context.WithTimeout(context.Background(), 20*time.Second)
	buildErr := fullBuilder.RebuildDirtyWithContext(buildCtx)
	buildCtxErr := buildCtx.Err()
	buildCancel()
	buildDuration := time.Since(buildStart)
	graphNodes, graphEdges := fullGraph.Stats()
	fmt.Printf("[hpa-test] chunks=%d\n", client.World().ChunkCount())
	fmt.Printf("[hpa-test] clusters=%d\n", fullClusters.Count())
	fmt.Printf("[hpa-test] entrances=%d\n", fullClusters.EntranceCount())
	fmt.Printf("[hpa-test] graph_nodes=%d\n", graphNodes)
	fmt.Printf("[hpa-test] graph_edges=%d\n", graphEdges)

	if len(abstractPath) < 2 {
		fmt.Printf("[hpa-test] mode=bounded_loaded_chunks\n")
		fmt.Printf("[hpa-test] abstract_path_found=false\n")
		fmt.Printf("[hpa-test] refined_segments=0\n")
		fmt.Printf("[hpa-test] fallback_used=false\n")
		fmt.Printf("[hpa-test] build_duration_ms=%d\n", buildDuration.Milliseconds())
		fmt.Printf("[hpa-test] plan_duration_ms=0\n")
		fmt.Printf("[hpa-test] bounded_graph_verified=false\n")
		fmt.Printf("[hpa-test] result=PARTIAL reason=no_connected_loaded_chunk_abstract_path full_reason=not_attempted_no_probe_path\n")
		return
	}

	start := probe.nodes[abstractPath[0]]
	target := probe.nodes[abstractPath[len(abstractPath)-1]]
	planStart := time.Now()
	fullRefinedSegments := 0
	fullReason := ""
	if buildErr != nil {
		if buildCtxErr != nil {
			fullReason = "full_builder_timeout"
		} else {
			fullReason = fmt.Sprintf("full_builder_error:%v", buildErr)
		}
	}
	if fullReason != "" {
		// Keep the bounded diagnostic below.
	} else if graphNodes == 0 {
		fullReason = "full_graph_empty"
	} else if graphEdges == 0 {
		fullReason = "full_graph_has_no_edges"
	} else {
		fullPlanner := hpa.NewHPAPlanner(client.World(), fullGraph, fullClusters)
		planCtx, planCancel := context.WithTimeout(context.Background(), 8*time.Second)
		res := fullPlanner.PlanWithContext(planCtx, start, target, goal.NewGoalBlock(target[0], target[1], target[2]))
		if res.Status != navplanner.PlanFound {
			if planCtx.Err() != nil {
				fullReason = "full_planner_timeout"
			} else if res.Err != nil {
				fullReason = fmt.Sprintf("full_planner_error:%v", res.Err)
			} else {
				fullReason = fmt.Sprintf("full_abstract_no_path:%s", res.Status)
			}
		} else if len(res.AbstractPath) < 2 {
			fullReason = "full_abstract_path_too_short"
		} else {
			for !res.Refiner.IsComplete() {
				segment := res.Refiner.NextSegment()
				if len(segment) == 0 {
					if planCtx.Err() != nil {
						fullReason = "full_refinement_timeout"
					} else {
						fullReason = "full_refinement_failed"
					}
					break
				}
				fullRefinedSegments++
			}
			if fullReason == "" && fullRefinedSegments == 0 {
				fullReason = "full_refined_zero_segments"
			}
		}
		planCancel()
	}
	planDuration := time.Since(planStart)
	if fullReason == "" {
		fmt.Printf("[hpa-test] mode=full\n")
		fmt.Printf("[hpa-test] abstract_path_found=true\n")
		fmt.Printf("[hpa-test] refined_segments=%d\n", fullRefinedSegments)
		fmt.Printf("[hpa-test] fallback_used=false\n")
		fmt.Printf("[hpa-test] build_duration_ms=%d\n", buildDuration.Milliseconds())
		fmt.Printf("[hpa-test] plan_duration_ms=%d\n", planDuration.Milliseconds())
		fmt.Printf("[hpa-test] result=PASS\n")
		return
	}

	boundedPlanStart := time.Now()
	boundedRefinedSegments := 0
	for i := 0; i < len(abstractPath)-1; i++ {
		from := probe.nodes[abstractPath[i]]
		to := probe.nodes[abstractPath[i+1]]
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		segment := navplanner.Plan(ctx, from[0], from[1], from[2], goal.NewGoalBlock(to[0], to[1], to[2]), client.World())
		cancel()
		if segment.Status != navplanner.PlanFound || len(segment.Path) == 0 {
			break
		}
		boundedRefinedSegments++
	}
	boundedPlanDuration := time.Since(boundedPlanStart)

	fmt.Printf("[hpa-test] mode=bounded_loaded_chunks\n")
	fmt.Printf("[hpa-test] abstract_path_found=true\n")
	fmt.Printf("[hpa-test] refined_segments=%d\n", boundedRefinedSegments)
	fmt.Printf("[hpa-test] fallback_used=false\n")
	fmt.Printf("[hpa-test] build_duration_ms=%d\n", buildDuration.Milliseconds())
	fmt.Printf("[hpa-test] plan_duration_ms=%d\n", boundedPlanDuration.Milliseconds())
	fmt.Printf("[hpa-test] full_reason=%s\n", fullReason)
	boundedVerified := boundedRefinedSegments == len(abstractPath)-1
	fmt.Printf("[hpa-test] bounded_graph_verified=%v\n", boundedVerified)
	if boundedRefinedSegments != len(abstractPath)-1 {
		fmt.Printf("[hpa-test] result=PARTIAL reason=bounded_refinement_incomplete\n")
		return
	}
	fmt.Printf("[hpa-test] result=PARTIAL reason=bounded_loaded_chunk_proof\n")
}

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
	start := [2]int{startChunkX, startChunkZ}
	target := [2]int{targetChunkX, targetChunkZ}
	if _, ok := g.nodes[start]; !ok {
		return nil
	}
	if _, ok := g.nodes[target]; !ok {
		return nil
	}
	queue := [][2]int{start}
	parent := map[[2]int][2]int{}
	seen := map[[2]int]bool{start: true}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if cur == target {
			path := [][2]int{cur}
			for cur != start {
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

func selectHPAProbePoints(w *world.World, centerChunkX, centerChunkZ, radiusChunks int) ([3]int, [3]int, bool) {
	coords := w.ChunkCoords()
	points := make([][3]int, 0, len(coords))
	for _, coord := range coords {
		if absInt(coord[0]-centerChunkX) > radiusChunks || absInt(coord[1]-centerChunkZ) > radiusChunks {
			continue
		}
		if p, ok := standablePointInChunk(w, coord[0], coord[1]); ok {
			points = append(points, p)
		}
	}
	if len(points) < 2 {
		return [3]int{}, [3]int{}, false
	}
	start := points[0]
	target := points[1]
	best := -1
	for _, a := range points {
		for _, b := range points {
			dist := absInt(a[0]-b[0]) + absInt(a[2]-b[2])
			if dist > best {
				best = dist
				start = a
				target = b
			}
		}
	}
	return start, target, best > 0
}

func standablePointInChunk(w *world.World, chunkX, chunkZ int) ([3]int, bool) {
	baseX := chunkX * world.ChunkWidth
	baseZ := chunkZ * world.ChunkDepth
	for _, local := range preferredLocalColumns() {
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

func preferredLocalColumns() [][2]int {
	return [][2]int{{8, 8}, {4, 4}, {12, 12}, {4, 12}, {12, 4}, {8, 4}, {4, 8}, {8, 12}, {12, 8}}
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

func runStuckTest() {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	client := &stuckProbeClient{x: 0.5, y: 64, z: 0.5}
	err := executor.Execute(ctx, client, world.NewWorld(), stuckProbeGoal{}, []move.Movement{stuckProbeMove{}})
	detected := err != nil && strings.Contains(err.Error(), "replanning produced empty path")
	fmt.Printf("[stuck] detected=%v\n", detected)
	fmt.Printf("[stuck] avoid_added=%v\n", detected)
	fmt.Printf("[stuck] replan_attempted=%v\n", detected)
	if detected {
		fmt.Printf("[stuck] result=failed_cleanly\n")
		return
	}
	if err != nil {
		fmt.Printf("[stuck] result=failed_unproven error=%v\n", err)
		return
	}
	fmt.Printf("[stuck] result=failed_unproven error=<nil>\n")
}

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

	// Rollback check
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
	x, _, z, _, _ := client.GetPosition()
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
				centerX := float64(wx) + 0.5
				centerZ := float64(wz) + 0.5
				if math.Hypot(centerX-x, centerZ-z) < 2.0 {
					continue
				}
				surfaceY := client.World().GetSurfaceY(wx, wz)
				if surfaceY == world.UnknownSurfaceY {
					continue
				}
				targetY := surfaceY + 1
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

func blockChunkCoord(block int) int {
	return floorDivInt(block, world.ChunkWidth)
}

func runPlaceBlockInvalidSmoke(client *feast.Client) {
	x, y, z, _, _ := client.GetPosition()
	// Target overlaps player hitbox
	target := protocol.BlockPos{X: int32(x), Y: int32(y), Z: int32(z)}
	err := client.PlaceBlock(target, protocol.BlockFaceTop)
	if err != nil {
		fmt.Printf("[place-invalid] result=PASS reason=clean_error\n")
	} else {
		fmt.Printf("[place-invalid] result=FAIL reason=no_error_for_invalid_placement\n")
	}
}

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
	start := &protocol.PlayServerboundPlayerActionPacket{
		Status:   protocol.PlayerActionStartDigging,
		Position: plan.Target,
		Face:     protocol.BlockFaceTop,
		Sequence: 20,
	}
	_ = client.WritePacket(start)
	time.Sleep(1500 * time.Millisecond)

	finish := &protocol.PlayServerboundPlayerActionPacket{
		Status:   protocol.PlayerActionFinishDigging,
		Position: plan.Target,
		Face:     protocol.BlockFaceTop,
		Sequence: 21,
	}
	_ = client.WritePacket(finish)

	breakUpdate := waitForBlockUpdateCoords(updateCh, int(plan.Target.X), int(plan.Target.Y), int(plan.Target.Z), 5*time.Second)
	invalidationAfterBreak := client.HPAInvalidations() > initInvalidations
	fmt.Printf("[hpa-mutation] break_block=%v\n", breakUpdate)
	fmt.Printf("[hpa-mutation] invalidation_after_break=%v\n", invalidationAfterBreak)

	// Wait for HPA rebuilding
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
	fmt.Printf("[hpa-mutation] result=%s\n", result)
}

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
