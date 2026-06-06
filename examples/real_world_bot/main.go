// real_world_bot demonstrates FeastGo as a usable offline-mode Minecraft bot
// library against a local/private Java 1.20.4 server.
//
// Environment variables:
//
//	MC_HOST      server hostname (default: 127.0.0.1)
//	MC_PORT      server port (default: 25565)
//	MC_USERNAME  player name (default: FeastGoBot)
package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/qrjhamron/feast/pkg/feast"
	"github.com/qrjhamron/feast/pkg/nav/goal"
	"github.com/qrjhamron/feast/pkg/nav/move"
	navplanner "github.com/qrjhamron/feast/pkg/nav/planner"
	"github.com/qrjhamron/feast/pkg/world"
)

const (
	requestedDistance = 30
)

type scenarioStatus int

const (
	statusPASS scenarioStatus = iota
	statusPARTIAL
	statusFAIL
)

type navTarget struct {
	pos              feast.BlockPos
	path             []move.Movement
	cost             float64
	actual           float64
	yDelta           int
	selectedReason   string
	rejectedElevated int
	rejectedUnloaded int
	rejectedUnsafe   int
	rejectedNoPath   int
	fallback         bool
	reason           string
}

type placeChoice struct {
	slot   int
	stack  feast.ItemStack
	target feast.BlockPos
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	status := statusPASS
	var reasons []string
	addPartial := func(reason string) {
		if status != statusFAIL {
			status = statusPARTIAL
		}
		reasons = append(reasons, reason)
	}
	fail := func(reason string) {
		status = statusFAIL
		reasons = append(reasons, reason)
	}

	debug, _ := strconv.ParseBool(getenv("FEAST_DEBUG", "false"))
	bot, err := feast.Connect(ctx, feast.Options{
		Host:     getenv("MC_HOST", "127.0.0.1"),
		Port:     getenv("MC_PORT", "25565"),
		Username: getenv("MC_USERNAME", "FeastGoBot"),
		Debug:    debug,
	})
	if err != nil {
		fmt.Printf("[scenario] connected=false error=%v\n", err)
		fmt.Printf("[scenario] result=FAIL reason=connect_failed\n")
		return
	}
	fmt.Printf("[scenario] connected=true\n")

	disconnectClean := false
	defer func() {
		fmt.Printf("[scenario] disconnect_clean=%v\n", disconnectClean)
		fmt.Printf("[scenario] result=%s", status)
		if len(reasons) > 0 {
			fmt.Printf(" reason=%s", strings.Join(reasons, ","))
		}
		fmt.Println()
	}()

	if err := bot.WaitUntilReady(ctx); err != nil {
		fmt.Printf("[scenario] ready=false error=%v\n", err)
		fail("ready_failed")
		disconnectClean = disconnect(bot)
		return
	}
	fmt.Printf("[scenario] ready=true\n")

	time.Sleep(2 * time.Second)

	pos := bot.Position()
	fmt.Printf("[scenario] position=x=%.2f y=%.2f z=%.2f\n", pos.X, pos.Y, pos.Z)
	fmt.Printf("[scenario] health=%.1f\n", bot.Health())
	fmt.Printf("[scenario] food=%d\n", bot.Food())
	chunks := bot.World().ChunkCount()
	fmt.Printf("[scenario] chunks_loaded=%d\n", chunks)
	if chunks == 0 {
		fail("world_not_loaded")
		disconnectClean = disconnect(bot)
		return
	}

	printInventorySummary(bot)

	if hit, ok := findNearbyTargetBlock(bot); ok {
		fmt.Printf("[block] nearby_target=%s pos=%d,%d,%d distance=%.2f\n", hit.Block.Name, hit.X, hit.Y, hit.Z, hit.Distance)
	} else {
		addPartial("no_nearby_target_block")
		fmt.Printf("[block] nearby_target_found=false\n")
	}

	var nav navTarget
	var ok bool
	for attempt := 1; attempt <= 10; attempt++ {
		nav, ok = findSafeNavigationTarget(ctx, bot, requestedDistance)
		if ok {
			break
		}
		time.Sleep(1 * time.Second)
	}

	fmt.Printf("[path] mode=local_astar\n")
	fmt.Printf("[path] requested_distance=%d\n", requestedDistance)
	if !ok {
		fmt.Printf("[path] found=false\n")
		fmt.Printf("[path] local_astar_found=false reason=no_safe_loaded_target\n")
		fmt.Printf("[path] diagnosis=target_unreachable_due_y_or_no_flat_safe_loaded_target\n")
		printPathDiagnostics(nav)
		addPartial("target_unreachable_due_y")
		disconnectClean = disconnect(bot)
		return
	}
	if nav.fallback {
		addPartial(nav.reason)
		fmt.Printf("[path] fallback=true reason=%s\n", nav.reason)
	}
	fmt.Printf("[path] target=%d,%d,%d\n", nav.pos.X, nav.pos.Y, nav.pos.Z)
	fmt.Printf("[path] actual_distance=%.2f\n", nav.actual)
	fmt.Printf("[path] target_loaded=%v\n", bot.World().IsBlockLoaded(world.BlockPos(nav.pos)))
	fmt.Printf("[path] found=true\n")
	fmt.Printf("[path] local_astar_found=true\n")
	fmt.Printf("[path] nodes=%d\n", len(nav.path))
	fmt.Printf("[path] cost=%.2f\n", nav.cost)
	printPathDiagnostics(nav)

	moveErr := moveToScenarioTarget(ctx, bot, nav)
	if moveErr != nil && !nav.fallback {
		fmt.Printf("[path] fallback=true reason=far_target_move_failed:%v\n", moveErr)
		fallbackNav, fallbackOK := scanNavigationRange(ctx, bot, 3, 8, 5)
		if fallbackOK {
			fallbackNav.fallback = true
			fallbackNav.reason = "far_target_move_failed"
			fmt.Printf("[path] target=%d,%d,%d\n", fallbackNav.pos.X, fallbackNav.pos.Y, fallbackNav.pos.Z)
			fmt.Printf("[path] actual_distance=%.2f\n", fallbackNav.actual)
			fmt.Printf("[path] target_loaded=%v\n", bot.World().IsBlockLoaded(world.BlockPos(fallbackNav.pos)))
			fmt.Printf("[path] found=true\n")
			fmt.Printf("[path] local_astar_found=true\n")
			fmt.Printf("[path] nodes=%d\n", len(fallbackNav.path))
			fmt.Printf("[path] cost=%.2f\n", fallbackNav.cost)
			printPathDiagnostics(fallbackNav)
			moveErr = moveToScenarioTarget(ctx, bot, fallbackNav)
			if moveErr == nil {
				addPartial("far_movement_fallback")
			}
		}
	}
	if moveErr != nil {
		fail("movement_failed")
		disconnectClean = disconnect(bot)
		return
	}

	breakStatus, breakPos, breakReason := runBreakScenario(ctx, bot)
	if breakStatus == statusFAIL {
		fail(breakReason)
		disconnectClean = disconnect(bot)
		return
	}
	if breakStatus == statusPARTIAL {
		addPartial(breakReason)
	} else {
		placeStatus, placeReason := runPlaceScenario(ctx, bot, breakPos)
		switch placeStatus {
		case statusFAIL:
			fail(placeReason)
			disconnectClean = disconnect(bot)
			return
		case statusPARTIAL:
			addPartial(placeReason)
		}
	}

	containerStatus := tryChestScenario(ctx, bot)
	if containerStatus != statusPASS {
		addPartial("container_optional")
	}
	if !tryEntitySummary(bot) {
		addPartial("entity_optional")
	}

	disconnectClean = disconnect(bot)
	if !disconnectClean {
		fail("disconnect_failed")
	}
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func printInventorySummary(bot *feast.Client) {
	inv := bot.InventorySnapshot()
	fmt.Printf("[inventory] selected_hotbar=%d\n", inv.SelectedHotbarSlot)
	if held, ok := bot.HeldItem(); ok {
		fmt.Printf("[inventory] held_item=%s count=%d\n", held.Name, held.Count)
	} else {
		fmt.Printf("[inventory] held_item=empty\n")
	}
	for i := 0; i < 9; i++ {
		slot := 36 + i
		stack := inv.Slots[slot]
		if stack.Present {
			fmt.Printf("[inventory] hotbar_%d=%s x%d\n", i, stack.Name, stack.Count)
		} else {
			fmt.Printf("[inventory] hotbar_%d=empty\n", i)
		}
	}
}

func findNearbyTargetBlock(bot *feast.Client) (world.BlockHit, bool) {
	for _, name := range []string{"grass_block", "dirt", "stone", "cobblestone", "chest"} {
		if hit, ok := bot.FindNearestBlock(name, 32); ok {
			return hit, true
		}
	}
	return world.BlockHit{}, false
}

func findSafeNavigationTarget(ctx context.Context, bot *feast.Client, requested int) (navTarget, bool) {
	if target, ok := scanNavigationRange(ctx, bot, 15, 40, requested); ok {
		return target, true
	}
	target, ok := scanNavigationRange(ctx, bot, 3, 8, 5)
	if ok {
		target.fallback = true
		target.reason = "far_target_unavailable"
		return target, true
	}
	target, ok = scanNavigationRange(ctx, bot, 2, 8, 3)
	if ok {
		target.fallback = true
		target.reason = "flat_3_to_8_unavailable"
	}
	return target, ok
}

func moveToScenarioTarget(ctx context.Context, bot *feast.Client, nav navTarget) error {
	moveStart := bot.Position()
	fmt.Printf("[move] start=x=%.2f y=%.2f z=%.2f\n", moveStart.X, moveStart.Y, moveStart.Z)
	fmt.Printf("[move] local_astar_node=%d,%d,%d\n", nav.pos.X, nav.pos.Y, nav.pos.Z)
	fmt.Printf("[move] executor_target_center=x=%.2f y=%.2f z=%.2f\n", float64(nav.pos.X)+0.5, float64(nav.pos.Y), float64(nav.pos.Z)+0.5)
	fmt.Printf("[move] node_y_semantics=feet_position\n")
	moveCtx, moveCancel := context.WithTimeout(ctx, 45*time.Second)
	moveErr := bot.NavigateTo(moveCtx, goal.NewGoalBlock(int(nav.pos.X), int(nav.pos.Y), int(nav.pos.Z)), feast.MovementOptions{
		Profile:   feast.MovementBotLike,
		Timeout:   40 * time.Second,
		Tolerance: 1.25,
	})
	moveCancel()
	moveFinal := bot.Position()
	moveStats := bot.LastMovementStats()
	fmt.Printf("[move] first_target_node=%s\n", formatTargetNode(moveStats))
	fmt.Printf("[move] first_packet_pos=%s\n", formatPacketPos(moveStats.HasFirstPacketPos, moveStats.FirstPacketX, moveStats.FirstPacketY, moveStats.FirstPacketZ))
	fmt.Printf("[move] last_packet_pos=%s\n", formatPacketPos(moveStats.HasFirstPacketPos, moveStats.LastPacketX, moveStats.LastPacketY, moveStats.LastPacketZ))
	fmt.Printf("[move] final=x=%.2f y=%.2f z=%.2f\n", moveFinal.X, moveFinal.Y, moveFinal.Z)
	fmt.Printf("[move] stats_start_pos=x=%.2f y=%.2f z=%.2f\n", moveStats.StartX, moveStats.StartY, moveStats.StartZ)
	fmt.Printf("[move] stats_final_pos=x=%.2f y=%.2f z=%.2f\n", moveStats.FinalX, moveStats.FinalY, moveStats.FinalZ)
	fmt.Printf("[move] distance_traveled=%.2f\n", moveStats.DistanceTraveled)
	fmt.Printf("[move] packets_sent=%d\n", moveStats.PacketsSent)
	fmt.Printf("[move] server_positions_seen=%d\n", moveStats.ServerPositionsSeen)
	fmt.Printf("[move] corrections_seen=%d\n", moveStats.CorrectionsSeen)
	fmt.Printf("[move] teleports_seen=%d\n", moveStats.TeleportsSeen)
	if moveStats.CorrectionsSeen > 0 {
		fmt.Printf("[move] last_correction_pos=x=%.2f y=%.2f z=%.2f\n", moveStats.LastCorrectionX, moveStats.LastCorrectionY, moveStats.LastCorrectionZ)
	}
	fmt.Printf("[move] final_distance=%.2f\n", moveStats.FinalDistance)
	fmt.Printf("[move] reached=%v\n", moveErr == nil)
	if moveErr != nil {
		fmt.Printf("[move] reason=%v\n", moveErr)
		fmt.Printf("[move] stuck_reason=%s\n", moveStats.StuckReason)
		fmt.Printf("[move] result=FAIL\n")
	} else {
		fmt.Printf("[move] result=PASS\n")
	}
	return moveErr
}

func scanNavigationRange(ctx context.Context, bot *feast.Client, minDist, maxDist, preferred int) (navTarget, bool) {
	start := bot.Position()
	startX := int(math.Floor(start.X))
	startY := int(math.Floor(start.Y))
	startZ := int(math.Floor(start.Z))

	var radii []int
	for d := preferred; d <= maxDist; d++ {
		if d >= minDist {
			radii = append(radii, d)
		}
	}
	for d := preferred - 1; d >= minDist; d-- {
		radii = append(radii, d)
	}

	var diag navTarget
	for _, maxPathYDelta := range []int{0, 1} {
		for _, maxYDelta := range []int{0, 1} {
			for _, r := range radii {
				for dx := -r; dx <= r; dx++ {
					for dz := -r; dz <= r; dz++ {
						if absInt(dx)+absInt(dz) != r {
							continue
						}
						x := startX + dx
						z := startZ + dz
						actual := math.Hypot(float64(dx), float64(dz))
						if actual < float64(minDist) || actual > float64(maxDist) {
							continue
						}
						surfaceY := bot.World().GetSurfaceY(x, z)
						if surfaceY == world.UnknownSurfaceY {
							diag.rejectedUnloaded++
							continue
						}
						y := surfaceY + 1
						yDelta := absInt(y - startY)
						if yDelta > maxYDelta {
							diag.rejectedElevated++
							continue
						}
						pos := feast.BlockPos{X: int32(x), Y: int32(y), Z: int32(z)}
						if !isSafeStandable(bot.World(), pos) {
							diag.rejectedUnsafe++
							continue
						}
						g := goal.NewGoalBlock(x, y, z)
						planCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
						res := navplanner.Plan(planCtx, startX, startY, startZ, g, bot.World())
						cancel()
						if res.Status != navplanner.PlanFound || len(res.Path) == 0 {
							diag.rejectedNoPath++
							continue
						}
						if pathYDelta(startY, [3]int{startX, startY, startZ}, res.Path) > maxPathYDelta {
							diag.rejectedElevated++
							continue
						}
						return navTarget{
							pos:              pos,
							path:             res.Path,
							cost:             pathCost(bot.World(), [3]int{startX, startY, startZ}, res.Path),
							actual:           actual,
							yDelta:           yDelta,
							selectedReason:   fmt.Sprintf("max_y_delta_%d_max_path_y_delta_%d", maxYDelta, maxPathYDelta),
							rejectedElevated: diag.rejectedElevated,
							rejectedUnloaded: diag.rejectedUnloaded,
							rejectedUnsafe:   diag.rejectedUnsafe,
							rejectedNoPath:   diag.rejectedNoPath,
						}, true
					}
				}
			}
		}
	}
	return diag, false
}

func isSafeStandable(w *world.World, pos feast.BlockPos) bool {
	if w == nil || !w.IsBlockLoaded(world.BlockPos(pos)) {
		return false
	}
	if !w.IsPassable(int(pos.X), int(pos.Y), int(pos.Z)) || !w.IsPassable(int(pos.X), int(pos.Y)+1, int(pos.Z)) {
		return false
	}
	if !w.HasSolidGround(world.BlockPos(pos)) {
		return false
	}
	feet, err := w.GetBlock(int(pos.X), int(pos.Y), int(pos.Z))
	if err != nil || isUnsafeBlockName(feet.Name) {
		return false
	}
	head, err := w.GetBlock(int(pos.X), int(pos.Y)+1, int(pos.Z))
	return err == nil && !isUnsafeBlockName(head.Name)
}

func pathCost(w *world.World, start [3]int, path []move.Movement) float64 {
	pos := start
	var cost float64
	for _, step := range path {
		stepCost := step.Cost(w, pos)
		if math.IsInf(stepCost, 1) || math.IsNaN(stepCost) {
			return math.Inf(1)
		}
		cost += stepCost
		pos = step.Destination(pos)
	}
	return cost
}

func pathYDelta(startY int, start [3]int, path []move.Movement) int {
	pos := start
	maxDelta := 0
	for _, step := range path {
		pos = step.Destination(pos)
		if d := absInt(pos[1] - startY); d > maxDelta {
			maxDelta = d
		}
	}
	return maxDelta
}

func printPathDiagnostics(nav navTarget) {
	fmt.Printf("[path] selected_reason=%s\n", nav.selectedReason)
	fmt.Printf("[path] selected_y_delta=%d\n", nav.yDelta)
	fmt.Printf("[path] rejected_elevated=%d\n", nav.rejectedElevated)
	fmt.Printf("[path] rejected_unloaded=%d\n", nav.rejectedUnloaded)
	fmt.Printf("[path] rejected_unsafe=%d\n", nav.rejectedUnsafe)
	fmt.Printf("[path] rejected_no_path=%d\n", nav.rejectedNoPath)
}

func formatTargetNode(stats feast.MovementStats) string {
	if !stats.HasFirstTargetNode {
		return "none"
	}
	return fmt.Sprintf("%d,%d,%d", stats.FirstTargetNodeX, stats.FirstTargetNodeY, stats.FirstTargetNodeZ)
}

func formatPacketPos(ok bool, x, y, z float64) string {
	if !ok {
		return "none"
	}
	return fmt.Sprintf("x=%.3f y=%.3f z=%.3f", x, y, z)
}

func runBreakScenario(ctx context.Context, bot *feast.Client) (scenarioStatus, feast.BlockPos, string) {
	target, oldState, ok := findSafeBreakTarget(bot)
	if !ok {
		fmt.Printf("[break] result=SKIPPED reason=no_safe_block_found\n")
		return statusPARTIAL, feast.BlockPos{}, "break_unavailable"
	}
	fmt.Printf("[break] auto_tool=true\n")
	fmt.Printf("[break] target=%d,%d,%d\n", target.X, target.Y, target.Z)
	fmt.Printf("[break] old_state=%s\n", oldState.Name)
	beforeHeld, _ := bot.HeldItem()
	breakCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err := bot.BreakBlock(breakCtx, target, feast.BreakOptions{AutoTool: true})
	cancel()
	afterHeld, hasHeld := bot.HeldItem()
	selectedTool := beforeHeld.Name
	if hasHeld {
		selectedTool = afterHeld.Name
	}
	fmt.Printf("[break] selected_tool=%s\n", valueOrNone(selectedTool))
	if err != nil {
		fmt.Printf("[break] block_update_received=false\n")
		fmt.Printf("[break] result=FAIL error=%v\n", err)
		return statusFAIL, target, "break_failed"
	}
	newState, err := bot.World().GetBlock(int(target.X), int(target.Y), int(target.Z))
	if err != nil {
		fmt.Printf("[break] block_update_received=true\n")
		fmt.Printf("[break] result=FAIL error=%v\n", err)
		return statusFAIL, target, "break_verify_failed"
	}
	fmt.Printf("[break] block_update_received=true\n")
	fmt.Printf("[break] new_state=%s\n", newState.Name)
	if bot.World().IsReplaceable(world.BlockPos(target)) {
		fmt.Printf("[break] result=PASS\n")
		return statusPASS, target, ""
	}
	fmt.Printf("[break] result=FAIL reason=block_not_removed\n")
	return statusFAIL, target, "break_verify_failed"
}

func findSafeBreakTarget(bot *feast.Client) (feast.BlockPos, world.BlockState, bool) {
	pos := bot.Position()
	originX := int(math.Floor(pos.X))
	originY := int(math.Floor(pos.Y))
	originZ := int(math.Floor(pos.Z))

	// Check if we have a pickaxe in our hotbar
	inv := bot.InventorySnapshot()
	hasPickaxe := false
	for i := 0; i < 9; i++ {
		slot := 36 + i
		item := inv.Slots[slot]
		if item.Present && strings.Contains(strings.ToLower(item.Name), "pickaxe") {
			hasPickaxe = true
		}
	}

	eyeX := pos.X
	eyeY := pos.Y + 1.62
	eyeZ := pos.Z

	type candInfo struct {
		pos      feast.BlockPos
		state    world.BlockState
		distance float64
		score    int
	}
	var candidates []candInfo

	for dx := -5; dx <= 5; dx++ {
		for dy := -2; dy <= 3; dy++ {
			for dz := -5; dz <= 5; dz++ {
				cand := feast.BlockPos{X: int32(originX + dx), Y: int32(originY + dy), Z: int32(originZ + dz)}

				// Avoid block directly under our feet
				if cand.X == int32(originX) && cand.Z == int32(originZ) && cand.Y < int32(originY) {
					continue
				}
				// Avoid blocks near feet (radius 1 around feet at Y < originY)
				if cand.Y < int32(originY) && absInt(int(cand.X)-originX) <= 1 && absInt(int(cand.Z)-originZ) <= 1 {
					continue
				}

				if !bot.World().IsBlockLoaded(world.BlockPos(cand)) {
					continue
				}

				block, err := bot.World().GetBlock(int(cand.X), int(cand.Y), int(cand.Z))
				if err != nil || bot.World().IsReplaceable(world.BlockPos(cand)) || isUnsafeBlockName(block.Name) {
					continue
				}
				if block.Name == "bedrock" || block.Name == "barrier" {
					continue
				}

				tcX := float64(cand.X) + 0.5
				tcY := float64(cand.Y) + 0.5
				tcZ := float64(cand.Z) + 0.5
				dist := math.Sqrt((tcX-eyeX)*(tcX-eyeX) + (tcY-eyeY)*(tcY-eyeY) + (tcZ-eyeZ)*(tcZ-eyeZ))

				// Survival reach limit is 4.5. Let's use 4.0 to be safe and within reach!
				if dist > 4.0 {
					continue
				}

				name := normalizeName(block.Name)
				score := -1

				isSoft := name == "dirt" || name == "grass_block"
				isStone := name == "stone" || name == "cobblestone"

				// Prefer side blocks at same Y or one below eye level (originY or originY+1)
				isSideY := cand.Y == int32(originY) || cand.Y == int32(originY+1)

				if isSoft {
					if isSideY && dist <= 3.5 {
						score = 100 // highest preference: soft side block within 3-4 blocks
					} else {
						score = 80 // soft block elsewhere
					}
				} else if isStone {
					if hasPickaxe {
						if isSideY && dist <= 3.5 {
							score = 60 // stone side block with pickaxe
						} else {
							score = 40 // stone elsewhere with pickaxe
						}
					} else {
						// No pickaxe: do not prefer stone
						score = 10
					}
				} else {
					score = 20
				}

				if score > 0 {
					candidates = append(candidates, candInfo{
						pos:      cand,
						state:    block,
						distance: dist,
						score:    score,
					})
				}
			}
		}
	}

	if len(candidates) == 0 {
		return feast.BlockPos{}, world.BlockState{}, false
	}

	// Find candidate with highest score, tie-break by closest distance
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.score > best.score {
			best = c
		} else if c.score == best.score {
			if c.distance < best.distance {
				best = c
			}
		}
	}

	return best.pos, best.state, true
}

func runPlaceScenario(ctx context.Context, bot *feast.Client, preferred feast.BlockPos) (scenarioStatus, string) {
	choice, ok := findSafePlaceTarget(bot, preferred)
	if !ok {
		missingItems := !hasPlaceableHotbarItem(bot)
		fmt.Printf("[inventory] missing_required_items=%v\n", missingItems)
		fmt.Printf("[place] result=SKIPPED reason=no_safe_place_target_or_item\n")
		if missingItems {
			return statusPARTIAL, "missing_survival_items"
		}
		return statusPARTIAL, "place_unavailable"
	}
	if err := bot.SelectHotbarSlot(ctx, choice.slot); err != nil {
		fmt.Printf("[place] result=FAIL error=%v\n", err)
		return statusFAIL, "place_select_failed"
	}

	support := feast.BlockPos{X: choice.target.X, Y: choice.target.Y - 1, Z: choice.target.Z}
	oldState, _ := bot.World().GetBlock(int(choice.target.X), int(choice.target.Y), int(choice.target.Z))
	countBefore := currentSlotCount(bot, choice.slot)
	blockName, _ := feast.BlockNameFromItem(choice.stack)

	fmt.Printf("[place] mode=survival_inventory\n")
	fmt.Printf("[place] selected_slot=%d\n", choice.slot)
	fmt.Printf("[place] held_item=%s\n", blockName)
	fmt.Printf("[place] target=%d,%d,%d\n", choice.target.X, choice.target.Y, choice.target.Z)
	fmt.Printf("[place] support=%d,%d,%d\n", support.X, support.Y, support.Z)
	fmt.Printf("[place] old_state=%s\n", oldState.Name)

	placeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err := bot.PlaceBlockSurvival(placeCtx, choice.target, feast.FaceUp)
	cancel()
	countAfter := currentSlotCount(bot, choice.slot)
	newState, getErr := bot.World().GetBlock(int(choice.target.X), int(choice.target.Y), int(choice.target.Z))
	fmt.Printf("[place] block_update_received=%v\n", err == nil)
	if getErr == nil {
		fmt.Printf("[place] new_state=%s\n", newState.Name)
	} else {
		fmt.Printf("[place] new_state=unknown error=%v\n", getErr)
	}
	fmt.Printf("[place] inventory_count_before=%d\n", countBefore)
	fmt.Printf("[place] inventory_count_after=%d\n", countAfter)
	if err != nil {
		fmt.Printf("[place] result=FAIL error=%v\n", err)
		return statusFAIL, "place_failed"
	}
	if getErr == nil && !bot.World().IsReplaceable(world.BlockPos(choice.target)) {
		fmt.Printf("[place] result=PASS\n")
		return statusPASS, ""
	}
	fmt.Printf("[place] result=FAIL reason=target_not_filled\n")
	return statusFAIL, "place_verify_failed"
}

func findSafePlaceTarget(bot *feast.Client, preferred feast.BlockPos) (placeChoice, bool) {
	slot, stack, ok := findPlaceableHotbarItem(bot)
	if !ok {
		return placeChoice{}, false
	}
	if isSafePlaceTarget(bot, preferred) {
		return placeChoice{slot: slot, stack: stack, target: preferred}, true
	}

	pos := bot.Position()
	originX := int(math.Floor(pos.X))
	originY := int(math.Floor(pos.Y))
	originZ := int(math.Floor(pos.Z))
	for r := 1; r <= 5; r++ {
		for dx := -r; dx <= r; dx++ {
			for dz := -r; dz <= r; dz++ {
				for dy := -1; dy <= 1; dy++ {
					cand := feast.BlockPos{X: int32(originX + dx), Y: int32(originY + dy), Z: int32(originZ + dz)}
					if isSafePlaceTarget(bot, cand) {
						return placeChoice{slot: slot, stack: stack, target: cand}, true
					}
				}
			}
		}
	}
	return placeChoice{}, false
}

func isSafePlaceTarget(bot *feast.Client, pos feast.BlockPos) bool {
	if !bot.World().IsBlockLoaded(world.BlockPos(pos)) || !bot.World().IsReplaceable(world.BlockPos(pos)) {
		return false
	}
	if blockDistance(bot.Position(), pos) > 5.5 {
		return false
	}
	if bot.World().IsEntityBlocking(blockAABB(pos)) {
		return false
	}
	support := world.BlockPos{X: pos.X, Y: pos.Y - 1, Z: pos.Z}
	return bot.World().IsBlockLoaded(support) && bot.World().IsSolid(support)
}

func findPlaceableHotbarItem(bot *feast.Client) (int, feast.ItemStack, bool) {
	inv := bot.InventorySnapshot()
	preferred := []string{"stone", "cobblestone", "dirt", "grass_block"}
	for _, name := range preferred {
		for i := 0; i < 9; i++ {
			stack := inv.Slots[36+i]
			if stack.Present && stack.Count > 0 && stack.Name == name && feast.IsPlaceableBlockItem(stack) {
				return i, stack, true
			}
		}
	}
	for i := 0; i < 9; i++ {
		stack := inv.Slots[36+i]
		if stack.Present && stack.Count > 0 && feast.IsPlaceableBlockItem(stack) {
			return i, stack, true
		}
	}
	return -1, feast.ItemStack{}, false
}

func hasPlaceableHotbarItem(bot *feast.Client) bool {
	_, _, ok := findPlaceableHotbarItem(bot)
	return ok
}

func currentSlotCount(bot *feast.Client, hotbarSlot int) int {
	inv := bot.InventorySnapshot()
	stack := inv.Slots[36+hotbarSlot]
	if !stack.Present {
		return 0
	}
	return stack.Count
}

func tryChestScenario(ctx context.Context, bot *feast.Client) scenarioStatus {
	hit, ok := bot.FindNearestBlock("chest", 24)
	fmt.Printf("[container] chest_found=%v\n", ok)
	if !ok {
		fmt.Printf("[container] opened=false\n")
		fmt.Printf("[container] deposit_result=SKIPPED\n")
		fmt.Printf("[container] withdraw_result=SKIPPED\n")
		return statusPARTIAL
	}

	chestPos := feast.BlockPos{X: int32(hit.X), Y: int32(hit.Y), Z: int32(hit.Z)}
	if blockDistance(bot.Position(), chestPos) > 4.5 {
		near, found := findStandableNear(bot, chestPos, 6)
		if found {
			navCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			err := bot.NavigateTo(navCtx, goal.NewGoalBlock(int(near.X), int(near.Y), int(near.Z)), feast.MovementOptions{
				Profile:   feast.MovementBotLike,
				Timeout:   18 * time.Second,
				Tolerance: 1.25,
			})
			cancel()
			if err != nil {
				fmt.Printf("[container] opened=false reason=navigate_failed:%v\n", err)
				fmt.Printf("[container] deposit_result=SKIPPED\n")
				fmt.Printf("[container] withdraw_result=SKIPPED\n")
				return statusPARTIAL
			}
		}
	}

	openCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	chest, err := bot.OpenChest(openCtx, chestPos)
	cancel()
	fmt.Printf("[container] opened=%v\n", err == nil)
	if err != nil {
		fmt.Printf("[container] open_error=%v\n", err)
		fmt.Printf("[container] deposit_result=SKIPPED\n")
		fmt.Printf("[container] withdraw_result=SKIPPED\n")
		return statusPARTIAL
	}
	fmt.Printf("[container] id=%d\n", chest.ID)
	fmt.Printf("[container] type=%s\n", chest.Type)
	fmt.Printf("[container] title=%s\n", chest.Title)
	items := chest.Items()
	fmt.Printf("[container] slots=%d\n", len(items))
	printFirstContainerItems(items, 5)

	itemName, ok := findDepositCandidate(bot)
	if !ok {
		fmt.Printf("[container] deposit_result=SKIPPED reason=no_inventory_item\n")
		fmt.Printf("[container] withdraw_result=SKIPPED\n")
		_ = chest.Close(ctx)
		fmt.Printf("[container] closed=true\n")
		return statusPARTIAL
	}

	depositCtx, depositCancel := context.WithTimeout(ctx, 5*time.Second)
	depositErr := chest.Deposit(depositCtx, itemName, 1)
	depositCancel()
	if depositErr != nil {
		fmt.Printf("[container] deposit_result=FAIL error=%v\n", depositErr)
		fmt.Printf("[container] withdraw_result=SKIPPED\n")
		_ = chest.Close(ctx)
		fmt.Printf("[container] closed=true\n")
		return statusPARTIAL
	}
	fmt.Printf("[container] deposit_result=PASS item=%s count=1\n", itemName)

	withdrawCtx, withdrawCancel := context.WithTimeout(ctx, 5*time.Second)
	withdrawErr := chest.Withdraw(withdrawCtx, itemName, 1)
	withdrawCancel()
	if withdrawErr != nil {
		fmt.Printf("[container] withdraw_result=FAIL error=%v\n", withdrawErr)
		_ = chest.Close(ctx)
		fmt.Printf("[container] closed=true\n")
		return statusPARTIAL
	}
	fmt.Printf("[container] withdraw_result=PASS item=%s count=1\n", itemName)

	if err := chest.Close(ctx); err != nil {
		fmt.Printf("[container] closed=false error=%v\n", err)
		return statusPARTIAL
	}
	fmt.Printf("[container] closed=true\n")
	return statusPASS
}

func printFirstContainerItems(items map[int]feast.ItemStack, limit int) {
	printed := 0
	for slot := 0; slot < 128 && printed < limit; slot++ {
		stack, ok := items[slot]
		if !ok || !stack.Present {
			continue
		}
		fmt.Printf("[container] item slot=%d name=%s count=%d\n", slot, stack.Name, stack.Count)
		printed++
	}
}

func findDepositCandidate(bot *feast.Client) (string, bool) {
	inv := bot.InventorySnapshot()
	for slot := 9; slot <= 44; slot++ {
		stack := inv.Slots[slot]
		if stack.Present && stack.Count > 0 && stack.Name != "" {
			return stack.Name, true
		}
	}
	return "", false
}

func findStandableNear(bot *feast.Client, target feast.BlockPos, radius int) (feast.BlockPos, bool) {
	for r := 1; r <= radius; r++ {
		for dx := -r; dx <= r; dx++ {
			for dz := -r; dz <= r; dz++ {
				if absInt(dx)+absInt(dz) != r {
					continue
				}
				x := int(target.X) + dx
				z := int(target.Z) + dz
				surfaceY := bot.World().GetSurfaceY(x, z)
				if surfaceY == world.UnknownSurfaceY {
					continue
				}
				pos := feast.BlockPos{X: int32(x), Y: int32(surfaceY + 1), Z: int32(z)}
				if isSafeStandable(bot.World(), pos) {
					return pos, true
				}
			}
		}
	}
	return feast.BlockPos{}, false
}

func tryEntitySummary(bot *feast.Client) bool {
	entities := bot.Entities().All()
	if len(entities) == 0 {
		fmt.Printf("[entity] seen=false\n")
		fmt.Printf("[entity] result=SKIPPED\n")
		return false
	}
	ent := entities[0]
	box := ent.Hitbox()
	fmt.Printf("[entity] seen=true\n")
	fmt.Printf("[entity] type=%s\n", ent.Type)
	fmt.Printf("[entity] position=x=%.2f y=%.2f z=%.2f\n", ent.X, ent.Y, ent.Z)
	fmt.Printf("[entity] pose=%v\n", ent.Pose)
	fmt.Printf("[entity] hitbox=min(%.2f,%.2f,%.2f) max(%.2f,%.2f,%.2f)\n",
		box.MinX, box.MinY, box.MinZ, box.MaxX, box.MaxY, box.MaxZ)
	fmt.Printf("[entity] collision=%v\n", bot.World().IsEntityBlocking(box))
	fmt.Printf("[entity] result=OBSERVED\n")
	return true
}

func disconnect(bot *feast.Client) bool {
	_ = bot.Disconnect()
	shut := bot.ShutdownStatus()
	return shut.Requested && shut.TickLoopStopped && shut.ReadLoopStopped && shut.NavLoopStopped && shut.SocketClosed
}

func distance3(a, b feast.Vec3) float64 {
	dx := a.X - b.X
	dy := a.Y - b.Y
	dz := a.Z - b.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

func blockDistance(origin feast.Vec3, pos feast.BlockPos) float64 {
	dx := float64(pos.X) + 0.5 - origin.X
	dy := float64(pos.Y) + 0.5 - origin.Y
	dz := float64(pos.Z) + 0.5 - origin.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

func blockAABB(pos feast.BlockPos) world.AABB {
	return world.AABB{
		MinX: float64(pos.X),
		MinY: float64(pos.Y),
		MinZ: float64(pos.Z),
		MaxX: float64(pos.X) + 1,
		MaxY: float64(pos.Y) + 1,
		MaxZ: float64(pos.Z) + 1,
	}
}

func normalizeName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	return strings.TrimPrefix(name, "minecraft:")
}

func isUnsafeBlockName(name string) bool {
	switch normalizeName(name) {
	case "lava", "fire", "soul_fire", "magma_block", "cactus":
		return true
	default:
		return false
	}
}

func valueOrNone(v string) string {
	if v == "" {
		return "none"
	}
	return v
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (s scenarioStatus) String() string {
	switch s {
	case statusPASS:
		return "PASS"
	case statusPARTIAL:
		return "PARTIAL"
	default:
		return "FAIL"
	}
}
