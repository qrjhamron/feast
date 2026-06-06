package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/qrjhamron/feast/pkg/feast"
	"github.com/qrjhamron/feast/pkg/nav/goal"
	"github.com/qrjhamron/feast/pkg/nav/planner"
	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/world"
)

var (
	runningCommandMu sync.Mutex
	isCommandRunning bool
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	host := getenv("MC_HOST", "127.0.0.1")
	port := getenv("MC_PORT", "25565")
	username := getenv("MC_USERNAME", "FeastGoBot")
	debug := os.Getenv("FEAST_DEBUG") == "true"

	bot := feast.NewClient(feast.Options{
		Host:     host,
		Port:     port,
		Username: username,
		Debug:    debug,
	})

	mainCtx, mainCancel := context.WithCancel(context.Background())
	defer mainCancel()

	stopChan := make(chan struct{})

	// Register chat listener
	bot.OnChat(func(e feast.ChatEvent) {
		msg := cleanChatMessage(e.Message)
		if msg == "" {
			return
		}

		fields := strings.Fields(msg)
		if len(fields) == 0 {
			return
		}
		cmd := fields[0]

		switch cmd {
		case "!help":
			sendSafeChat(bot, "commands: !pos,!inv,!find <block> [count] [radius],!nav <block|x y z> [radius],!navbuild <x y z>,!hpa <x y z>,!move <distance>,!swim [radius],!scaffold <length> [dir],!breakn <block> <count> [radius],!build10 <block>,!stop")

		case "!pos":
			pos := bot.Position()
			synced := bot.PositionSynced()
			sendSafeChat(bot, fmt.Sprintf("pos: x=%.2f y=%.2f z=%.2f synced=%t", pos.X, pos.Y, pos.Z, synced))

		case "!inv":
			inv := bot.InventorySnapshot()
			selected := inv.SelectedHotbarSlot
			heldSlot := 36 + selected
			heldStr := "empty"
			if stack, ok := inv.Slots[heldSlot]; ok && stack.Present && stack.Count > 0 {
				heldStr = fmt.Sprintf("%s x%d", normalizeBlockName(stack.Name), stack.Count)
			}
			sendSafeChat(bot, fmt.Sprintf("hotbar: selected=%d held=%s", selected, heldStr))

			var parts []string
			for i := 0; i < 9; i++ {
				slot := 36 + i
				stack := inv.Slots[slot]
				if stack.Present && stack.Count > 0 {
					parts = append(parts, fmt.Sprintf("%d=%s x%d", i, normalizeBlockName(stack.Name), stack.Count))
				} else {
					parts = append(parts, fmt.Sprintf("%d=empty", i))
				}
			}
			sendSafeChat(bot, strings.Join(parts, " | "))

		case "!stop":
			sendSafeChat(bot, "stopping "+username)
			mainCancel()
			close(stopChan)

		case "!find":
			go func() {
				if !acquireCommandLock() {
					sendSafeChat(bot, "busy: command already running")
					return
				}
				defer releaseCommandLock()

				block, count, radius, ok := parseFindArgs(msg)
				if !ok {
					sendSafeChat(bot, "usage: !find <block> [count] [radius]")
					return
				}

				resolved := resolveBlockName(block)
				hits := bot.FindBlocks(resolved, count, radius)

				sendSafeChat(bot, fmt.Sprintf("found %d %s:", len(hits), resolved))

				var chunk []string
				for i, h := range hits {
					chunk = append(chunk, fmt.Sprintf("%d) %d %d %d d=%.2f", i+1, h.X, h.Y, h.Z, h.Distance))
					if len(chunk) == 5 || i == len(hits)-1 {
						sendSafeChat(bot, strings.Join(chunk, " | "))
						chunk = nil
					}
				}
			}()

		case "!nav":
			go func() {
				if !acquireCommandLock() {
					sendSafeChat(bot, "busy: command already running")
					return
				}
				defer releaseCommandLock()

				block, x, y, z, isCoord, radius, ok := parseNavArgs(fields)
				if !ok {
					sendSafeChat(bot, "usage: !nav <block> [radius] or !nav <x> <y> <z>")
					return
				}

				ctx, cancel := context.WithTimeout(mainCtx, 60*time.Second)
				defer cancel()

				if isCoord {
					targetPos := feast.BlockPos{X: int32(x), Y: int32(y), Z: int32(z)}
					standPos, reason := classifyCoordNav(bot, targetPos)
					if reason != "" {
						hint := ""
						if reason == "target_unsafe" {
							hint = " (try !navbuild)"
						}
						sendSafeChat(bot, fmtLine("[nav] result=FAIL reason=%s%s", reason, hint))
						return
					}

					sendSafeChat(bot, fmtLine("[nav] target=coordinate block=%d,%d,%d stand=%d,%d,%d", x, y, z, standPos.X, standPos.Y, standPos.Z))

					err := bot.NavigateTo(ctx, goal.NewGoalBlock(int(standPos.X), int(standPos.Y), int(standPos.Z)))
					stats := bot.LastMovementStats()
					if err != nil {
						sendSafeChat(bot, fmtLine("[nav] result=FAIL reason=%v", err))
					} else {
						sendSafeChat(bot, fmtLine("[nav] result=OK reached=true packets=%d distance=%.2f", stats.PacketsSent, stats.DistanceTraveled))
					}
				} else {
					resolved := resolveBlockName(block)
					hit, found := bot.FindNearestBlock(resolved, radius)
					if !found {
						sendSafeChat(bot, fmt.Sprintf("[nav] failed reason=block_not_found block=%s", resolved))
						return
					}

					targetPos := feast.BlockPos{X: int32(hit.X), Y: int32(hit.Y), Z: int32(hit.Z)}
					standPos, foundStand := findSafeStandingNear(bot, targetPos, 5)
					if !foundStand {
						sendSafeChat(bot, "[nav] failed reason=no_safe_standing_found")
						return
					}

					sendSafeChat(bot, fmt.Sprintf("[nav] target=%s block=%d,%d,%d stand=%d,%d,%d", resolved, hit.X, hit.Y, hit.Z, standPos.X, standPos.Y, standPos.Z))
					sendSafeChat(bot, "[nav] local_astar=true")

					err := bot.NavigateTo(ctx, goal.NewGoalBlock(int(standPos.X), int(standPos.Y), int(standPos.Z)))
					stats := bot.LastMovementStats()
					if err != nil {
						sendSafeChat(bot, fmt.Sprintf("[nav] failed reason=%v", err))
					} else {
						sendSafeChat(bot, fmt.Sprintf("[nav] reached=true packets=%d distance=%.2f", stats.PacketsSent, stats.DistanceTraveled))
					}
				}
			}()

		case "!navbuild":
			go func() {
				if !acquireCommandLock() {
					sendSafeChat(bot, "busy: command already running")
					return
				}
				defer releaseCommandLock()

				x, y, z, ok := parseNavbuildArgs(fields)
				if !ok {
					sendSafeChat(bot, "usage: !navbuild <x> <y> <z>")
					return
				}

				ctx, cancel := context.WithTimeout(mainCtx, 180*time.Second)
				defer cancel()

				executeNavbuild(ctx, bot, x, y, z, func(s string) { sendSafeChat(bot, s) })
			}()

		case "!hpa":
			go func() {
				if !acquireCommandLock() {
					sendSafeChat(bot, "busy: command already running")
					return
				}
				defer releaseCommandLock()

				x, y, z, ok := parseHPAArgs(fields)
				if !ok {
					sendSafeChat(bot, "usage: !hpa <x> <y> <z>")
					return
				}

				hpaNav := bot.HPANav()
				if hpaNav == nil {
					sendSafeChat(bot, "[hpa] result=SKIPPED reason=api_not_exposed")
					return
				}

				pos := bot.Position()
				startX := int(math.Floor(pos.X))
				startY := int(math.Floor(pos.Y))
				startZ := int(math.Floor(pos.Z))

				sendSafeChat(bot, fmt.Sprintf("[hpa] start=%d,%d,%d", startX, startY, startZ))
				sendSafeChat(bot, fmt.Sprintf("[hpa] goal=%d,%d,%d", x, y, z))

				hpaStats := bot.HPAStats()
				sendSafeChat(bot, fmt.Sprintf("[hpa] graph_nodes=%d", hpaStats.GraphNodes))
				sendSafeChat(bot, fmt.Sprintf("[hpa] graph_edges=%d", hpaStats.GraphEdges))

				startArr := [3]int{startX, startY, startZ}
				goalArr := [3]int{x, y, z}
				goalDef := goal.NewGoalBlock(x, y, z)

				ctx, cancel := context.WithTimeout(mainCtx, 5*time.Second)
				defer cancel()

				res := hpaNav.Planner.PlanWithContext(ctx, startArr, goalArr, goalDef)
				abstractPathFound := (res.Status == planner.PlanFound)
				sendSafeChat(bot, fmt.Sprintf("[hpa] abstract_path_found=%t", abstractPathFound))

				refinedSegments := 0
				if abstractPathFound && res.Refiner != nil {
					for !res.Refiner.IsComplete() {
						segment := res.Refiner.NextSegment()
						if len(segment) == 0 {
							break
						}
						refinedSegments++
					}
				}
				sendSafeChat(bot, fmt.Sprintf("[hpa] refined_segments=%d", refinedSegments))

				fallbackUsed := false
				if !abstractPathFound {
					localRes := planner.Plan(ctx, startX, startY, startZ, goalDef, bot.World())
					if localRes.Status == planner.PlanFound {
						fallbackUsed = true
					}
				}
				sendSafeChat(bot, fmt.Sprintf("[hpa] fallback_used=%t", fallbackUsed))

				expectedSegments := 0
				if abstractPathFound && len(res.AbstractPath) > 1 {
					expectedSegments = len(res.AbstractPath) - 1
				}

				resultStr := "FAIL"
				if abstractPathFound && refinedSegments == expectedSegments {
					resultStr = "PASS"
				} else if fallbackUsed || (abstractPathFound && refinedSegments > 0) {
					resultStr = "PARTIAL"
				}
				sendSafeChat(bot, fmt.Sprintf("[hpa] result=%s", resultStr))
			}()

		case "!move":
			go func() {
				if !acquireCommandLock() {
					sendSafeChat(bot, "busy: command already running")
					return
				}
				defer releaseCommandLock()

				distance, ok := parseMoveArgs(fields)
				if !ok {
					sendSafeChat(bot, "usage: !move <distance>")
					return
				}

				sendSafeChat(bot, fmt.Sprintf("[move] requested=%d", distance))

				// Re-read the live position so movement plans from the bot's
				// actual feet, not a stale or surface-derived Y.
				x, _, z, yaw, _ := bot.GetPosition()
				feet := world.FeetBlockFromPosition(bot.Position())

				yawRad := float64(yaw) * math.Pi / 180.0
				dx := -math.Sin(yawRad)
				dz := math.Cos(yawRad)

				var targetPos feast.BlockPos
				found := false
				actualDistance := 0.0

				for step := distance; step >= 1; step-- {
					tx := x + dx*float64(step)
					tz := z + dz*float64(step)
					ix := int(math.Floor(tx))
					iz := int(math.Floor(tz))

					// Prefer a safe standing position near the bot's CURRENT feet
					// level instead of snapping to the surface. This keeps a bot
					// that is underground/in a cave from pathing up to the surface
					// (an impossible route, especially after terrain mutation).
					desired := feast.BlockPos{X: int32(ix), Y: feet.Y, Z: int32(iz)}
					if cand, ok := findSafeStandingNearPreferLevel(bot, desired, 3); ok {
						targetPos = cand
						actualDistance = float64(step)
						found = true
						break
					}
				}

				if !found {
					sendSafeChat(bot, "[move] result=FAIL reason=no_safe_loaded_target")
					return
				}

				sendSafeChat(bot, fmt.Sprintf("[move] actual_target_distance=%.2f", actualDistance))
				sendSafeChat(bot, fmt.Sprintf("[move] target=%d,%d,%d", targetPos.X, targetPos.Y, targetPos.Z))

				ctx, cancel := context.WithTimeout(mainCtx, 45*time.Second)
				defer cancel()

				err := bot.NavigateTo(ctx, goal.NewGoalBlock(int(targetPos.X), int(targetPos.Y), int(targetPos.Z)))
				stats := bot.LastMovementStats()

				if err != nil {
					sendSafeChat(bot, fmt.Sprintf("[move] result=FAIL reached=false packets=%d final_distance=%.2f", stats.PacketsSent, stats.FinalDistance))
				} else {
					sendSafeChat(bot, fmt.Sprintf("[move] result=OK reached=true packets=%d final_distance=%.2f", stats.PacketsSent, stats.FinalDistance))
				}
			}()

		case "!swim":
			go func() {
				if !acquireCommandLock() {
					sendSafeChat(bot, "busy: command already running")
					return
				}
				defer releaseCommandLock()

				radius, ok := parseSwimArgs(fields)
				if !ok {
					sendSafeChat(bot, "usage: !swim [radius]")
					return
				}

				hit, ok := bot.FindNearestBlock("water", radius)
				if !ok {
					sendSafeChat(bot, "[swim] result=SKIPPED reason=no_water_loaded")
					return
				}

				sendSafeChat(bot, fmt.Sprintf("[swim] water_found=true pos=%d,%d,%d", hit.X, hit.Y, hit.Z))

				ctx, cancel := context.WithTimeout(mainCtx, 30*time.Second)
				defer cancel()

				waterPos := feast.BlockPos{X: int32(hit.X), Y: int32(hit.Y), Z: int32(hit.Z)}
				botPos := bot.Position()
				if distanceSq(float64(waterPos.X)+0.5, float64(waterPos.Y)+0.5, float64(waterPos.Z)+0.5, botPos.X, botPos.Y, botPos.Z) > 16 {
					standPos, foundStand := findSafeStandingNear(bot, waterPos, 4)
					if foundStand {
						_ = bot.NavigateTo(ctx, goal.NewGoalBlock(int(standPos.X), int(standPos.Y), int(standPos.Z)))
					}
				}

				g := goal.NewGoalBlock(hit.X, hit.Y, hit.Z)
				_ = bot.NavigateTo(ctx, g)

				posAfter := bot.Position()
				currBlock, err := bot.World().GetBlock(int(math.Floor(posAfter.X)), int(math.Floor(posAfter.Y)), int(math.Floor(posAfter.Z)))
				entered := (err == nil && normalizeBlockName(currBlock.Name) == "water")

				if !entered {
					sendSafeChat(bot, "[swim] result=FAIL reason=could_not_enter_water")
					return
				}
				sendSafeChat(bot, "[swim] entered=true")

				var anotherWater world.BlockHit
				anotherFound := false
				for dx := -2; dx <= 2; dx++ {
					for dz := -2; dz <= 2; dz++ {
						for dy := -1; dy <= 1; dy++ {
							if dx == 0 && dz == 0 && dy == 0 {
								continue
							}
							wx := hit.X + dx
							wy := hit.Y + dy
							wz := hit.Z + dz
							block, err := bot.World().GetBlock(wx, wy, wz)
							if err == nil && normalizeBlockName(block.Name) == "water" {
								anotherWater = world.BlockHit{X: wx, Y: wy, Z: wz}
								anotherFound = true
								break
							}
						}
						if anotherFound {
							break
						}
					}
					if anotherFound {
						break
					}
				}

				moved := false
				if anotherFound {
					g2 := goal.NewGoalBlock(anotherWater.X, anotherWater.Y, anotherWater.Z)
					_ = bot.NavigateTo(ctx, g2)
					posFinal := bot.Position()
					if distanceSq(posAfter.X, posAfter.Y, posAfter.Z, posFinal.X, posFinal.Y, posFinal.Z) > 0.25 {
						moved = true
					}
				}

				sendSafeChat(bot, fmt.Sprintf("[swim] moved=%t", moved))
				sendSafeChat(bot, "[swim] result=PASS")
			}()

		case "!scaffold":
			go func() {
				if !acquireCommandLock() {
					sendSafeChat(bot, "busy: command already running")
					return
				}
				defer releaseCommandLock()

				length, dir, ok := parseScaffoldArgs(fields)
				if !ok {
					sendSafeChat(bot, "usage: !scaffold <length> [forward|north|south|east|west]")
					return
				}

				ctx, cancel := context.WithTimeout(mainCtx, 90*time.Second)
				defer cancel()

				executeScaffold(ctx, bot, length, dir, func(s string) { sendSafeChat(bot, s) })
			}()

		case "!breakn":
			go func() {
				if !acquireCommandLock() {
					sendSafeChat(bot, "busy: command already running")
					return
				}
				defer releaseCommandLock()

				block, count, radius, ok := parseBreakNArgs(fields)
				if !ok {
					sendSafeChat(bot, "usage: !breakn <block> <count> [radius]")
					return
				}

				sendSafeChat(bot, fmt.Sprintf("[breakn] target=%s count=%d radius=%d", block, count, radius))

				ctx, cancel := context.WithTimeout(mainCtx, 120*time.Second)
				defer cancel()

				broken := 0
				partialReason := ""

				for broken < count {
					targetPos, found := findNextBreakTarget(bot, block, radius)
					if !found {
						partialReason = "no_more_safe_targets"
						break
					}

					pos := bot.Position()
					dist := distance3(pos, feast.Vec3{X: float64(targetPos.X) + 0.5, Y: float64(targetPos.Y) + 0.5, Z: float64(targetPos.Z) + 0.5})

					if dist > 4.0 {
						standPos, foundStand := findSafeStandingNear(bot, targetPos, 4)
						if !foundStand {
							continue
						}

						err := bot.NavigateTo(ctx, goal.NewGoalBlock(int(standPos.X), int(standPos.Y), int(standPos.Z)))
						if err != nil {
							continue
						}
					}

					pos = bot.Position()
					dist = distance3(pos, feast.Vec3{X: float64(targetPos.X) + 0.5, Y: float64(targetPos.Y) + 0.5, Z: float64(targetPos.Z) + 0.5})
					if dist > 4.0 {
						continue
					}

					err := bot.BreakBlock(ctx, targetPos, feast.BreakOptions{AutoTool: true})
					if err != nil {
						continue
					}

					broken++
					if shouldLogProgress(broken, 5) {
						sendSafeChat(bot, fmtLine("[breakn] progress broken=%d/%d pos=%d,%d,%d", broken, count, targetPos.X, targetPos.Y, targetPos.Z))
					}
				}

				if broken == count {
					sendSafeChat(bot, fmt.Sprintf("[breakn] result=PASS broken=%d", broken))
				} else {
					sendSafeChat(bot, fmt.Sprintf("[breakn] result=PARTIAL broken=%d reason=%s", broken, partialReason))
				}
			}()

		case "!build10":
			go func() {
				if !acquireCommandLock() {
					sendSafeChat(bot, "busy: command already running")
					return
				}
				defer releaseCommandLock()

				block, ok := parseBuild10Args(fields)
				if !ok {
					sendSafeChat(bot, "usage: !build10 <block>")
					return
				}

				resolved := resolveBlockName(block)
				have := countBlocksInInventory(bot, resolved)
				if have < 100 {
					sendSafeChat(bot, fmt.Sprintf("[build10] result=PARTIAL reason=not_enough_blocks have=%d need=100", have))
					return
				}

				sendSafeChat(bot, fmt.Sprintf("[build10] block=%s", resolved))
				sendSafeChat(bot, "[build10] size=10x10")

				ctx, cancel := context.WithTimeout(mainCtx, 180*time.Second)
				defer cancel()

				pos := bot.Position()
				originX := int(math.Floor(pos.X))
				originY := int(math.Floor(pos.Y))
				originZ := int(math.Floor(pos.Z))

				gridY := originY - 1

				coords := generateBuild10Coordinates(originX, gridY, originZ)
				sortCoordinatesByDistance(coords, pos.X, pos.Y, pos.Z)

				placedCount := 0
				failedReason := ""
				failed := false

				for _, targetPos := range coords {
					if bot.World().IsSolid(world.BlockPos(targetPos)) {
						placedCount++
						continue
					}

					_, face, foundSupport := findPlacementSupport(bot, targetPos)
					if !foundSupport {
						continue
					}

					dist := distance3(bot.Position(), feast.Vec3{X: float64(targetPos.X) + 0.5, Y: float64(targetPos.Y) + 0.5, Z: float64(targetPos.Z) + 0.5})
					if dist > 5.0 {
						standPos, foundStand := findSafeStandingNear(bot, targetPos, 4)
						if !foundStand {
							failed = true
							failedReason = "no_standing_position"
							break
						}

						err := bot.NavigateTo(ctx, goal.NewGoalBlock(int(standPos.X), int(standPos.Y), int(standPos.Z)))
						if err != nil {
							failed = true
							failedReason = "navigation_failed"
							break
						}
					}

					if blockAABB(targetPos).Intersects(getBotAABB(bot)) {
						movedAway := false
						for _, dx := range []int{-1, 1, 0, 0} {
							for _, dz := range []int{0, 0, -1, 1} {
								standPos := feast.BlockPos{X: targetPos.X + int32(dx), Y: targetPos.Y + 1, Z: targetPos.Z + int32(dz)}
								if isSafeStanding(bot, standPos) && (standPos.X != targetPos.X || standPos.Z != targetPos.Z) {
									err := bot.NavigateTo(ctx, goal.NewGoalBlock(int(standPos.X), int(standPos.Y), int(standPos.Z)))
									if err == nil {
										movedAway = true
										break
									}
								}
							}
							if movedAway {
								break
							}
						}
						if !movedAway {
							failed = true
							failedReason = "overlaps_bot"
							break
						}
					}

					_, face, foundSupport = findPlacementSupport(bot, targetPos)
					if !foundSupport {
						continue
					}

					slot, _, foundBlock := findPlaceableHotbarItem(bot)
					if !foundBlock {
						failed = true
						failedReason = "out_of_blocks"
						break
					}

					if err := bot.SelectHotbarSlot(ctx, slot); err != nil {
						failed = true
						failedReason = "select_slot_failed"
						break
					}

					err := bot.PlaceBlockSurvival(ctx, targetPos, face)
					if err != nil {
						failed = true
						failedReason = "place_failed"
						break
					}

					if !bot.World().IsSolid(world.BlockPos(targetPos)) {
						failed = true
						failedReason = "place_verify_failed"
						break
					}
					placedCount++
				}

				if !failed && placedCount < 100 {
					for {
						progress := false
						for _, targetPos := range coords {
							if bot.World().IsSolid(world.BlockPos(targetPos)) {
								continue
							}

							_, face, foundSupport := findPlacementSupport(bot, targetPos)
							if !foundSupport {
								continue
							}

							dist := distance3(bot.Position(), feast.Vec3{X: float64(targetPos.X) + 0.5, Y: float64(targetPos.Y) + 0.5, Z: float64(targetPos.Z) + 0.5})
							if dist > 5.0 {
								standPos, foundStand := findSafeStandingNear(bot, targetPos, 4)
								if !foundStand {
									continue
								}
								err := bot.NavigateTo(ctx, goal.NewGoalBlock(int(standPos.X), int(standPos.Y), int(standPos.Z)))
								if err != nil {
									continue
								}
							}

							if blockAABB(targetPos).Intersects(getBotAABB(bot)) {
								movedAway := false
								for _, dx := range []int{-1, 1, 0, 0} {
									for _, dz := range []int{0, 0, -1, 1} {
										standPos := feast.BlockPos{X: targetPos.X + int32(dx), Y: targetPos.Y + 1, Z: targetPos.Z + int32(dz)}
										if isSafeStanding(bot, standPos) && (standPos.X != targetPos.X || standPos.Z != targetPos.Z) {
											err := bot.NavigateTo(ctx, goal.NewGoalBlock(int(standPos.X), int(standPos.Y), int(standPos.Z)))
											if err == nil {
												movedAway = true
												break
											}
										}
									}
									if movedAway {
										break
									}
								}
								if !movedAway {
									continue
								}
							}

							_, face, foundSupport = findPlacementSupport(bot, targetPos)
							if !foundSupport {
								continue
							}

							slot, _, foundBlock := findPlaceableHotbarItem(bot)
							if !foundBlock {
								failed = true
								failedReason = "out_of_blocks"
								break
							}

							if err := bot.SelectHotbarSlot(ctx, slot); err != nil {
								failed = true
								failedReason = "select_slot_failed"
								break
							}

							err := bot.PlaceBlockSurvival(ctx, targetPos, face)
							if err != nil {
								failed = true
								failedReason = "place_failed"
								break
							}

							if !bot.World().IsSolid(world.BlockPos(targetPos)) {
								failed = true
								failedReason = "place_verify_failed"
								break
							}
							placedCount++
							progress = true
						}
						if failed || !progress || placedCount >= 100 {
							break
						}
					}
				}

				if failed || placedCount < 100 {
					if failedReason == "" {
						failedReason = "place_failed"
					}
					sendSafeChat(bot, fmt.Sprintf("[build10] result=PARTIAL placed=%d reason=%s", placedCount, failedReason))
				} else {
					sendSafeChat(bot, fmt.Sprintf("[build10] placed=%d", placedCount))
					sendSafeChat(bot, "[build10] result=PASS")
				}
			}()
		}
	})

	if err := bot.Connect(); err != nil {
		log.Fatalf("connect failed: %v", err)
	}
	defer bot.Disconnect()

	fmt.Println("[adv] connected=true")

	// Wait until ready
	waitCtx, waitCancel := context.WithTimeout(mainCtx, 15*time.Second)
	if err := bot.WaitUntilReady(waitCtx); err != nil {
		waitCancel()
		log.Fatalf("wait ready failed: %v", err)
	}
	waitCancel()

	fmt.Println("[adv] ready=true")
	fmt.Printf("[adv] username=%s\n", username)
	fmt.Println("[adv] commands=!help,!pos,!inv,!find,!nav,!navbuild,!hpa,!move,!swim,!scaffold,!breakn,!build10,!stop")
	fmt.Printf("[adv] verbose=%t trace_packets=%t\n", advVerbose(), advTracePackets())

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigCh:
	case <-stopChan:
	}

	fmt.Println("[adv] shutting down")
}

// ─── Local Helpers ────────────────────────────────────────────────────────────

func acquireCommandLock() bool {
	runningCommandMu.Lock()
	defer runningCommandMu.Unlock()
	if isCommandRunning {
		return false
	}
	isCommandRunning = true
	return true
}

func releaseCommandLock() {
	runningCommandMu.Lock()
	defer runningCommandMu.Unlock()
	isCommandRunning = false
}

func cleanChatMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}

	if idx := strings.Index(msg, ": "); idx != -1 {
		sub := strings.TrimSpace(msg[idx+2:])
		if strings.HasPrefix(sub, "!") {
			return sub
		}
	}

	if strings.HasPrefix(msg, "!") {
		return msg
	}

	if idx := strings.Index(msg, "!"); idx != -1 {
		sub := strings.TrimSpace(msg[idx:])
		return sub
	}

	return ""
}

func resolveBlockName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.TrimPrefix(name, "minecraft:")
	switch name {
	case "oak", "wood", "log":
		return "oak_log"
	case "grass":
		return "grass_block"
	}
	return name
}

func normalizeBlockName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	return strings.TrimPrefix(name, "minecraft:")
}

func parseFindArgs(msg string) (block string, count int, radius int, ok bool) {
	fields := strings.Fields(msg)
	if len(fields) < 2 {
		return "", 0, 0, false
	}
	block = fields[1]
	count = 1
	radius = 64

	if len(fields) >= 3 {
		var err error
		count, err = strconv.Atoi(fields[2])
		if err != nil {
			return "", 0, 0, false
		}
	}
	if len(fields) >= 4 {
		var err error
		radius, err = strconv.Atoi(fields[3])
		if err != nil {
			return "", 0, 0, false
		}
	}

	if count < 1 {
		count = 1
	} else if count > 20 {
		count = 20
	}

	if radius < 1 {
		radius = 1
	} else if radius > 128 {
		radius = 128
	}

	return block, count, radius, true
}

func parseNavArgs(fields []string) (block string, x, y, z int, isCoord bool, radius int, ok bool) {
	if len(fields) < 2 {
		return "", 0, 0, 0, false, 0, false
	}

	if len(fields) >= 4 {
		vx, errX := strconv.Atoi(fields[1])
		vy, errY := strconv.Atoi(fields[2])
		vz, errZ := strconv.Atoi(fields[3])
		if errX == nil && errY == nil && errZ == nil {
			return "", vx, vy, vz, true, 0, true
		}
	}

	block = fields[1]
	radius = 64
	if len(fields) >= 3 {
		var err error
		radius, err = strconv.Atoi(fields[2])
		if err != nil {
			radius = 64
		}
	}
	if radius < 1 {
		radius = 1
	} else if radius > 128 {
		radius = 128
	}
	return block, 0, 0, 0, false, radius, true
}

func parseHPAArgs(fields []string) (x, y, z int, ok bool) {
	if len(fields) < 4 {
		return 0, 0, 0, false
	}
	vx, errX := strconv.Atoi(fields[1])
	vy, errY := strconv.Atoi(fields[2])
	vz, errZ := strconv.Atoi(fields[3])
	if errX != nil || errY != nil || errZ != nil {
		return 0, 0, 0, false
	}
	return vx, vy, vz, true
}

func parseMoveArgs(fields []string) (distance int, ok bool) {
	if len(fields) < 2 {
		return 0, false
	}
	d, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, false
	}
	if d < 1 {
		d = 1
	} else if d > 150 {
		d = 150
	}
	return d, true
}

func parseSwimArgs(fields []string) (radius int, ok bool) {
	radius = 64
	if len(fields) >= 2 {
		var err error
		radius, err = strconv.Atoi(fields[1])
		if err != nil {
			radius = 64
		}
	}
	if radius < 1 {
		radius = 1
	} else if radius > 128 {
		radius = 128
	}
	return radius, true
}

func parseScaffoldArgs(fields []string) (length int, dir string, ok bool) {
	if len(fields) < 2 {
		return 0, "", false
	}
	l, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, "", false
	}
	if l < 1 {
		l = 1
	} else if l > 32 {
		l = 32
	}
	// Optional direction argument.
	if len(fields) >= 3 {
		d := strings.ToLower(fields[2])
		switch d {
		case "north", "south", "east", "west", "forward":
			return l, d, true
		default:
			return 0, "", false
		}
	}
	return l, "forward", true
}

func parseBreakNArgs(fields []string) (block string, count, radius int, ok bool) {
	if len(fields) < 3 {
		return "", 0, 0, false
	}
	block = fields[1]

	c, err := strconv.Atoi(fields[2])
	if err != nil {
		return "", 0, 0, false
	}
	if c < 1 {
		c = 1
	} else if c > 50 {
		c = 50
	}

	r := 32
	if len(fields) >= 4 {
		var err error
		r, err = strconv.Atoi(fields[3])
		if err != nil {
			r = 32
		}
	}
	if r < 1 {
		r = 1
	} else if r > 128 {
		r = 128
	}
	return block, c, r, true
}

func parseBuild10Args(fields []string) (block string, ok bool) {
	if len(fields) < 2 {
		return "", false
	}
	return fields[1], true
}

func sendSafeChat(bot *feast.Client, msg string) {
	for _, line := range splitChatLines(msg) {
		_ = bot.Chat(line)
	}
}

func isSafeStanding(bot *feast.Client, pos feast.BlockPos) bool {
	w := bot.World()
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

func isUnsafeBlockName(name string) bool {
	switch normalizeBlockName(name) {
	case "lava", "fire", "soul_fire", "magma_block", "cactus":
		return true
	default:
		return false
	}
}

func findSafeStandingNear(bot *feast.Client, target feast.BlockPos, radius int) (feast.BlockPos, bool) {
	type candidate struct {
		pos  feast.BlockPos
		dist float64
	}
	var list []candidate

	for dx := -radius; dx <= radius; dx++ {
		for dz := -radius; dz <= radius; dz++ {
			for dy := -radius; dy <= radius; dy++ {
				cx := int(target.X) + dx
				cy := int(target.Y) + dy
				cz := int(target.Z) + dz

				pos := feast.BlockPos{X: int32(cx), Y: int32(cy), Z: int32(cz)}
				if isSafeStanding(bot, pos) {
					dist := math.Sqrt(float64(dx*dx + dy*dy + dz*dz))
					list = append(list, candidate{pos: pos, dist: dist})
				}
			}
		}
	}

	if len(list) == 0 {
		return feast.BlockPos{}, false
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].dist < list[j].dist
	})

	return list[0].pos, true
}

func distance3(a, b feast.Vec3) float64 {
	dx := a.X - b.X
	dy := a.Y - b.Y
	dz := a.Z - b.Z
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

func distanceSq(x1, y1, z1, x2, y2, z2 float64) float64 {
	dx := x1 - x2
	dy := y1 - y2
	dz := z1 - z2
	return dx*dx + dy*dy + dz*dz
}

func findPlaceableHotbarItem(bot *feast.Client) (int, feast.ItemStack, bool) {
	inv := bot.InventorySnapshot()
	for i := 0; i < 9; i++ {
		stack := inv.Slots[36+i]
		if stack.Present && stack.Count > 0 && feast.IsPlaceableBlockItem(stack) {
			return i, stack, true
		}
	}
	return -1, feast.ItemStack{}, false
}

func countBlocksInInventory(bot *feast.Client, blockName string) int {
	inv := bot.InventorySnapshot()
	total := 0
	for slot := 9; slot <= 44; slot++ {
		stack := inv.Slots[slot]
		if stack.Present && stack.Count > 0 && normalizeBlockName(stack.Name) == normalizeBlockName(blockName) {
			total += stack.Count
		}
	}
	return total
}

func generateBuild10Coordinates(ox, oy, oz int) []feast.BlockPos {
	var coords []feast.BlockPos
	for dx := -5; dx <= 4; dx++ {
		for dz := -5; dz <= 4; dz++ {
			coords = append(coords, feast.BlockPos{
				X: int32(ox + dx),
				Y: int32(oy),
				Z: int32(oz + dz),
			})
		}
	}
	return coords
}

func sortCoordinatesByDistance(coords []feast.BlockPos, botX, botY, botZ float64) {
	sort.Slice(coords, func(i, j int) bool {
		di := distanceSq(float64(coords[i].X)+0.5, float64(coords[i].Y)+0.5, float64(coords[i].Z)+0.5, botX, botY, botZ)
		dj := distanceSq(float64(coords[j].X)+0.5, float64(coords[j].Y)+0.5, float64(coords[j].Z)+0.5, botX, botY, botZ)
		return di < dj
	})
}

func findPlacementSupport(bot *feast.Client, pos feast.BlockPos) (feast.BlockPos, protocol.Direction, bool) {
	below := feast.BlockPos{X: pos.X, Y: pos.Y - 1, Z: pos.Z}
	if bot.World().IsBlockLoaded(below) && bot.World().IsSolid(below) {
		return below, feast.FaceUp, true
	}
	north := feast.BlockPos{X: pos.X, Y: pos.Y, Z: pos.Z + 1}
	if bot.World().IsBlockLoaded(north) && bot.World().IsSolid(north) {
		return north, feast.FaceNorth, true
	}
	south := feast.BlockPos{X: pos.X, Y: pos.Y, Z: pos.Z - 1}
	if bot.World().IsBlockLoaded(south) && bot.World().IsSolid(south) {
		return south, feast.FaceSouth, true
	}
	west := feast.BlockPos{X: pos.X + 1, Y: pos.Y, Z: pos.Z}
	if bot.World().IsBlockLoaded(west) && bot.World().IsSolid(west) {
		return west, feast.FaceWest, true
	}
	east := feast.BlockPos{X: pos.X - 1, Y: pos.Y, Z: pos.Z}
	if bot.World().IsBlockLoaded(east) && bot.World().IsSolid(east) {
		return east, feast.FaceEast, true
	}
	return feast.BlockPos{}, 0, false
}

func getBotAABB(bot *feast.Client) world.AABB {
	pos := bot.Position()
	return world.AABB{
		MinX: pos.X - 0.3,
		MaxX: pos.X + 0.3,
		MinY: pos.Y,
		MaxY: pos.Y + 1.8,
		MinZ: pos.Z - 0.3,
		MaxZ: pos.Z + 0.3,
	}
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

func generateScaffoldTarget(currX, currY, currZ int, yaw float32) (target feast.BlockPos, face protocol.Direction, dx, dz int) {
	dx, dz, face = forwardFromYaw(yaw)
	target = feast.BlockPos{
		X: int32(currX + dx),
		Y: int32(currY - 1),
		Z: int32(currZ + dz),
	}
	return target, face, dx, dz
}

func isSupportingBot(bot *feast.Client, pos feast.BlockPos) bool {
	bx := int(math.Floor(bot.Position().X))
	by := int(math.Floor(bot.Position().Y))
	bz := int(math.Floor(bot.Position().Z))
	return pos.X == int32(bx) && pos.Z == int32(bz) && pos.Y < int32(by)
}

func findNextBreakTarget(bot *feast.Client, name string, radius int) (feast.BlockPos, bool) {
	resolved := resolveBlockName(name)
	hits := bot.FindBlocks(resolved, 100, radius)
	for _, hit := range hits {
		pos := feast.BlockPos{X: int32(hit.X), Y: int32(hit.Y), Z: int32(hit.Z)}
		if isSupportingBot(bot, pos) {
			continue
		}
		if normalizeBlockName(hit.Block.Name) == "bedrock" || normalizeBlockName(hit.Block.Name) == "barrier" {
			continue
		}
		if !bot.World().IsBlockLoaded(world.BlockPos(pos)) {
			continue
		}
		return pos, true
	}
	return feast.BlockPos{}, false
}
