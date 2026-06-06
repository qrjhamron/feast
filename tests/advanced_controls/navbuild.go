package main

import (
	"context"
	"strconv"
	"time"

	"github.com/qrjhamron/feast/pkg/feast"
	"github.com/qrjhamron/feast/pkg/nav/goal"
	"github.com/qrjhamron/feast/pkg/world"
)

// maxNavbuildPlacements caps how many blocks a scaffold-assisted route may place
// to avoid griefing or runaway towers.
const maxNavbuildPlacements = 64

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// parseNavbuildArgs parses "!navbuild <x> <y> <z>".
func parseNavbuildArgs(fields []string) (x, y, z int, ok bool) {
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

// cardinalToward returns the cardinal step (one axis, ±1) that makes the most
// progress from feet toward target on the horizontal plane. ok is false when
// already aligned horizontally with the target column.
func cardinalToward(feet, target feast.BlockPos) (dx, dz int, ok bool) {
	ddx := int(target.X - feet.X)
	ddz := int(target.Z - feet.Z)
	if ddx == 0 && ddz == 0 {
		return 0, 0, false
	}
	absX := ddx
	if absX < 0 {
		absX = -absX
	}
	absZ := ddz
	if absZ < 0 {
		absZ = -absZ
	}
	if absX >= absZ {
		if ddx > 0 {
			return 1, 0, true
		}
		return -1, 0, true
	}
	if ddz > 0 {
		return 0, 1, true
	}
	return 0, -1, true
}

// classifyCoordNav decides how to navigate to an explicit coordinate. It returns
// the standing target to use and a non-empty reason when navigation should be
// refused (the caller reports the reason instead of moving).
func classifyCoordNav(bot *feast.Client, target feast.BlockPos) (stand feast.BlockPos, reason string) {
	w := bot.World()
	if w == nil || !w.IsBlockLoaded(world.BlockPos(target)) {
		return target, "chunk_unloaded"
	}
	if isSafeStanding(bot, target) {
		return target, ""
	}
	// Search for a nearby safe standing spot, preferring the smallest vertical
	// difference so we don't pick a wildly different Y.
	if near, ok := findSafeStandingNearPreferLevel(bot, target, 3); ok {
		return near, ""
	}
	return target, "target_unsafe"
}

// findSafeStandingNearPreferLevel finds the nearest safe standing position to
// target, breaking ties toward the same Y level (small vertical mismatch).
func findSafeStandingNearPreferLevel(bot *feast.Client, target feast.BlockPos, radius int) (feast.BlockPos, bool) {
	best := feast.BlockPos{}
	found := false
	bestScore := 1 << 30
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			for dz := -radius; dz <= radius; dz++ {
				cand := feast.BlockPos{X: target.X + int32(dx), Y: target.Y + int32(dy), Z: target.Z + int32(dz)}
				if !isSafeStanding(bot, cand) {
					continue
				}
				// Weight vertical mismatch heavily, then horizontal distance.
				score := 4*absInt(dy) + absInt(dx) + absInt(dz)
				if !found || score < bestScore {
					found = true
					bestScore = score
					best = cand
				}
			}
		}
	}
	return best, found
}

// executeNavbuild attempts to reach a goal coordinate, first with normal
// navigation and then, if that fails because of a gap/height, with a limited
// survival scaffold-assisted route directed toward the goal.
func executeNavbuild(ctx context.Context, bot *feast.Client, x, y, z int, say func(string)) {
	target := feast.BlockPos{X: int32(x), Y: int32(y), Z: int32(z)}
	say(fmtLine("[navbuild] goal=%d,%d,%d", x, y, z))

	w := bot.World()
	if w == nil || !w.IsBlockLoaded(world.BlockPos(target)) {
		say("[navbuild] result=FAIL reason=chunk_unloaded")
		return
	}

	// 1) Try normal navigation to a safe standing position.
	stand, reason := classifyCoordNav(bot, target)
	if reason == "" {
		navCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
		err := bot.NavigateTo(navCtx, goal.NewGoalBlock(int(stand.X), int(stand.Y), int(stand.Z)))
		cancel()
		if err == nil {
			say("[navbuild] normal_nav=true")
			say("[navbuild] result=PASS")
			return
		}
		say(fmtLine("[navbuild] normal_nav=false reason=%v", err))
	} else {
		say(fmtLine("[navbuild] normal_nav=false reason=%s", reason))
	}

	// 2) Scaffold-assisted route directed toward the goal.
	_, _, ok := findPlaceableHotbarItem(bot)
	if !ok {
		say("[navbuild] scaffold_attempt=false reason=no_placeable_blocks")
		say("[navbuild] result=PARTIAL reason=no_placeable_blocks")
		return
	}
	say("[navbuild] scaffold_attempt=true")

	view := botScaffoldView{bot: bot}
	placed := 0
	reached := false
	stopReason := ""

	for step := 0; step < 128; step++ {
		select {
		case <-ctx.Done():
			stopReason = "canceled"
			goto done
		default:
		}

		feet := world.FeetBlockFromPosition(bot.Position())
		// Close enough horizontally and within one vertical block → arrived.
		if absInt(int(feet.X)-x) <= 1 && absInt(int(feet.Z)-z) <= 1 && absInt(int(feet.Y)-y) <= 1 {
			reached = true
			goto done
		}

		dx, dz, hasDir := cardinalToward(feet, target)
		if !hasDir {
			reached = absInt(int(feet.Y)-y) <= 1
			stopReason = "aligned_no_vertical_path"
			goto done
		}

		plan := planScaffoldStep(view, feet, dx, dz)
		switch plan.action {
		case scaffoldUnreachableHeight:
			stopReason = "height_unreachable"
			goto done
		case scaffoldBlocked:
			stopReason = plan.reason
			goto done
		}

		if plan.place {
			if placed >= maxNavbuildPlacements {
				stopReason = "placement_cap_reached"
				goto done
			}
			slot, _, ok := findPlaceableHotbarItem(bot)
			if !ok {
				stopReason = "out_of_blocks"
				goto done
			}
			if err := bot.SelectHotbarSlot(ctx, slot); err != nil {
				stopReason = "select_slot_failed"
				goto done
			}
			_, face, found := findPlacementSupport(bot, plan.placePos)
			if !found {
				stopReason = "no_placement_support"
				goto done
			}
			if err := bot.PlaceBlockSurvival(ctx, plan.placePos, face); err != nil {
				stopReason = "place_failed"
				goto done
			}
			if !bot.World().IsSolid(world.BlockPos(plan.placePos)) {
				stopReason = "place_verify_failed"
				goto done
			}
			placed++
		}

		moveCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		err := bot.NavigateTo(moveCtx, goal.NewGoalBlock(int(plan.nextFeet.X), int(plan.nextFeet.Y), int(plan.nextFeet.Z)))
		cancel()
		if err != nil {
			stopReason = "movement_failed"
			goto done
		}
		got := world.FeetBlockFromPosition(bot.Position())
		if got.X != plan.nextFeet.X || got.Z != plan.nextFeet.Z {
			stopReason = "move_off_target"
			goto done
		}
		time.Sleep(50 * time.Millisecond)
	}
	stopReason = "max_steps"

done:
	say(fmtLine("[navbuild] placed=%d", placed))
	say(fmtLine("[navbuild] reached=%t", reached))
	switch {
	case reached:
		say("[navbuild] result=PASS")
	case placed > 0:
		say(fmtLine("[navbuild] result=PARTIAL reason=%s", stopReason))
	default:
		say(fmtLine("[navbuild] result=FAIL reason=%s", stopReason))
	}
}
