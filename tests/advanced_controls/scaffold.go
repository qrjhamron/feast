package main

import (
	"context"
	"math"
	"time"

	"github.com/qrjhamron/feast/pkg/feast"
	"github.com/qrjhamron/feast/pkg/nav/goal"
	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/world"
)

// ─── Scaffold step planning ────────────────────────────────────────────────
//
// The scaffold planner is intentionally pure: it decides what a single forward
// scaffold step should do given only block queries, so it can be unit-tested
// without a live server. The runtime is responsible for actually placing blocks
// (survival inventory only) and moving the bot.
//
// Position semantics follow pkg/world: a feet block is the block the player
// stands in; the ground block (feet.Y-1) supports it; the head block (feet.Y+1)
// is the upper hitbox half.

// scaffoldView abstracts the block queries the planner needs.
type scaffoldView interface {
	// loaded reports whether the block's chunk is loaded.
	loaded(p feast.BlockPos) bool
	// passable reports whether the bot can occupy the block (air/water, not a
	// solid or hazard).
	passable(p feast.BlockPos) bool
	// solid reports whether the block is collidable support.
	solid(p feast.BlockPos) bool
}

type scaffoldAction int

const (
	// scaffoldFlat advances forward at the same Y; may place a bridging block
	// below the next feet.
	scaffoldFlat scaffoldAction = iota
	// scaffoldClimb advances forward and one block up; may place a step block.
	scaffoldClimb
	// scaffoldUnreachableHeight means the forward path requires more than one
	// block of vertical climb in a single step.
	scaffoldUnreachableHeight
	// scaffoldBlocked means the forward path is obstructed and cannot be solved
	// by a single placement.
	scaffoldBlocked
)

func (a scaffoldAction) String() string {
	switch a {
	case scaffoldFlat:
		return "flat"
	case scaffoldClimb:
		return "climb"
	case scaffoldUnreachableHeight:
		return "unreachable_height"
	default:
		return "blocked"
	}
}

// scaffoldStep is the plan for a single forward step.
type scaffoldStep struct {
	action   scaffoldAction
	nextFeet feast.BlockPos
	place    bool
	placePos feast.BlockPos // air block to fill (face resolved by the runtime)
	reason   string
}

func feetAbove(p feast.BlockPos) feast.BlockPos { return feast.BlockPos{X: p.X, Y: p.Y + 1, Z: p.Z} }
func feetBelow(p feast.BlockPos) feast.BlockPos { return feast.BlockPos{X: p.X, Y: p.Y - 1, Z: p.Z} }

// standClear reports whether a feet block and its head block are both loaded and
// passable (i.e. the bot can stand there).
func standClear(v scaffoldView, feet feast.BlockPos) bool {
	head := feetAbove(feet)
	return v.loaded(feet) && v.passable(feet) && v.loaded(head) && v.passable(head)
}

// planScaffoldStep decides the next scaffold action from the current feet block
// and a cardinal forward direction (dx,dz one of ±1 on a single axis).
func planScaffoldStep(v scaffoldView, feet feast.BlockPos, dx, dz int) scaffoldStep {
	fwd := feast.BlockPos{X: feet.X + int32(dx), Y: feet.Y, Z: feet.Z + int32(dz)}

	// 1) Flat advance: the forward cell at the same level is standable.
	if standClear(v, fwd) {
		ground := feetBelow(fwd)
		if !v.loaded(ground) {
			return scaffoldStep{action: scaffoldBlocked, reason: "ground_not_loaded"}
		}
		if v.solid(ground) {
			return scaffoldStep{action: scaffoldFlat, nextFeet: fwd}
		}
		// Bridge: place a block below the next feet. Do not place inside the bot.
		if isBotOccupied(feet, ground) {
			return scaffoldStep{action: scaffoldBlocked, reason: "place_inside_bot"}
		}
		return scaffoldStep{action: scaffoldFlat, nextFeet: fwd, place: true, placePos: ground}
	}

	// 2) Climb one block up.
	up := feast.BlockPos{X: feet.X + int32(dx), Y: feet.Y + 1, Z: feet.Z + int32(dz)}
	aboveHead := feast.BlockPos{X: feet.X, Y: feet.Y + 2, Z: feet.Z} // headroom for the step-up

	headroomClear := v.loaded(aboveHead) && v.passable(aboveHead)
	if !standClear(v, up) || !headroomClear {
		// Cannot step up one block. If the column ahead is solid for two or more
		// blocks, the target height is unreachable in a single scaffold step.
		if v.loaded(up) && v.solid(up) {
			return scaffoldStep{action: scaffoldUnreachableHeight, reason: "height_unreachable"}
		}
		return scaffoldStep{action: scaffoldBlocked, reason: "obstructed"}
	}

	// The step block is the forward cell at the current level. To climb onto the
	// cell above it, that forward cell must be solid support. In a cardinal
	// single-step model a placeable step here would require the forward cell to
	// be air with a clear head — but that is exactly the flat case handled
	// above — so a climb only proceeds when a real step already exists.
	if v.solid(fwd) {
		return scaffoldStep{action: scaffoldClimb, nextFeet: up}
	}
	return scaffoldStep{action: scaffoldBlocked, reason: "no_step_to_climb"}
}

// isBotOccupied reports whether pos overlaps the bot's two-block hitbox standing
// at feet (feet block and head block).
func isBotOccupied(feet, pos feast.BlockPos) bool {
	if pos.X != feet.X || pos.Z != feet.Z {
		return false
	}
	return pos.Y == feet.Y || pos.Y == feet.Y+1
}

// directionToDXDZ converts a named direction string to cardinal dx/dz deltas.
// "forward" is resolved from yaw (locked at start of run, never re-read mid-run).
func directionToDXDZ(dir string, yaw float32) (dx, dz int, cardinal string) {
	switch dir {
	case "north":
		return 0, -1, "north"
	case "south":
		return 0, 1, "south"
	case "east":
		return 1, 0, "east"
	case "west":
		return -1, 0, "west"
	default: // "forward"
		dx, dz, _ = forwardFromYaw(yaw)
		return dx, dz, dxdzToCardinal(dx, dz)
	}
}

func dxdzToCardinal(dx, dz int) string {
	switch {
	case dz == -1:
		return "north"
	case dz == 1:
		return "south"
	case dx == 1:
		return "east"
	default:
		return "west"
	}
}

// forwardFromYaw maps a yaw to a cardinal forward step and the block face that
// faces the bot's forward direction.
func forwardFromYaw(yaw float32) (dx, dz int, face feast.Direction) {
	normYaw := yaw
	for normYaw < 0 {
		normYaw += 360
	}
	for normYaw >= 360 {
		normYaw -= 360
	}
	switch {
	case normYaw >= 315 || normYaw < 45:
		return 0, 1, feast.FaceSouth
	case normYaw >= 45 && normYaw < 135:
		return -1, 0, feast.FaceWest
	case normYaw >= 135 && normYaw < 225:
		return 0, -1, feast.FaceNorth
	default:
		return 1, 0, feast.FaceEast
	}
}

// botScaffoldView is the live-world implementation of scaffoldView.
type botScaffoldView struct {
	bot *feast.Client
}

func (v botScaffoldView) loaded(p feast.BlockPos) bool {
	return v.bot.World().IsBlockLoaded(world.BlockPos(p))
}

func (v botScaffoldView) solid(p feast.BlockPos) bool {
	return v.bot.World().IsSolid(world.BlockPos(p))
}

func (v botScaffoldView) passable(p feast.BlockPos) bool {
	w := v.bot.World()
	if !w.IsPassable(int(p.X), int(p.Y), int(p.Z)) {
		return false
	}
	block, err := w.GetBlock(int(p.X), int(p.Y), int(p.Z))
	if err != nil {
		return false
	}
	return !isUnsafeBlockName(block.Name)
}

// waitForSolid polls until the block at pos is solid or timeout elapses.
func waitForSolid(ctx context.Context, bot *feast.Client, pos feast.BlockPos, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if bot.World().IsSolid(world.BlockPos(pos)) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// scaffoldDirectStep moves the bot one adjacent block step by sending position
// packets directly, without going through the full NavigateTo/A* replan loop.
//
// Why not NavigateTo for each scaffold step?
//   - NavigateTo's A* planner may not recognize a freshly placed block as solid.
//   - resolveStartY inside NavigateTo2 may pick a wrong Y after block mutations.
//   - The stuck detection timeout (5 s, 3 stagnant cycles) fires quickly on
//     flat 1-block moves, yielding "stuck_after_replans" for perfectly fine ground.
//
// Strategy: walk at constant speed toward target center for up to 4 s, declare
// arrival when within 0.45 blocks horizontally. Fall back to NavigateTo once if
// the direct walk fails (e.g., server correction pushes bot off path).
func scaffoldDirectStep(ctx context.Context, bot *feast.Client, targetFeet feast.BlockPos) error {
	const (
		walkSpeed = 0.215 // blocks/tick (normal walk)
		tickRate  = 50 * time.Millisecond
		tolerance = 0.45 // horizontal arrival tolerance
		maxTicks  = 80   // 4 s max for one adjacent step
	)

	targetX := float64(targetFeet.X) + 0.5
	targetY := float64(targetFeet.Y)
	targetZ := float64(targetFeet.Z) + 0.5

	arrived := false
	for tick := 0; tick < maxTicks; tick++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		cx, cy, cz, cyaw, cpitch := bot.GetPosition()
		dx := targetX - cx
		dz := targetZ - cz
		horiz := math.Hypot(dx, dz)

		if horiz <= tolerance && math.Abs(targetY-cy) <= 2.0 {
			// Stabilise at exact target center.
			onGround := bot.World().IsOnGround(world.Vec3{X: targetX, Y: targetY, Z: targetZ})
			pkt := &protocol.PlayServerboundSetPlayerPositionAndRotationPacket{
				X: targetX, Y: targetY, Z: targetZ,
				Yaw: cyaw, Pitch: cpitch, OnGround: onGround,
			}
			_ = bot.WritePacket(pkt)
			arrived = true
			break
		}

		// Compute next step at walk speed.
		var nx, nz float64
		if horiz <= walkSpeed {
			nx, nz = targetX, targetZ
		} else {
			nx = cx + (dx/horiz)*walkSpeed
			nz = cz + (dz/horiz)*walkSpeed
		}
		// Snap Y to target when within step-up range.
		ny := cy
		if math.Abs(targetY-cy) <= 1.5 {
			ny = targetY
		}

		onGround := bot.World().IsOnGround(world.Vec3{X: nx, Y: ny, Z: nz})

		var newYaw float32
		if horiz > 0.001 {
			newYaw = float32(-math.Atan2(dx, dz) * 180 / math.Pi)
		} else {
			newYaw = cyaw
		}

		pkt := &protocol.PlayServerboundSetPlayerPositionAndRotationPacket{
			X: nx, Y: ny, Z: nz,
			Yaw: newYaw, Pitch: cpitch, OnGround: onGround,
		}
		if err := bot.WritePacket(pkt); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(tickRate):
		}
	}

	if arrived {
		return nil
	}

	// Direct walk timed out. Fall back to NavigateTo once.
	navCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	return bot.NavigateTo(navCtx, goal.NewGoalBlock(int(targetFeet.X), int(targetFeet.Y), int(targetFeet.Z)))
}

// scaffoldResult is the outcome of a scaffold run.
type scaffoldResult struct {
	length       int
	dir          string
	block        string
	placed       int
	movedSteps   int
	climbedSteps int
	result       string // PASS / PARTIAL / FAIL
	reason       string
}

// executeScaffold runs a survival scaffold of up to `length` forward steps.
// dir is "forward", "north", "south", "east", or "west".
//
// Key correctness properties:
//   - Direction is resolved from yaw ONCE at start, before any block operations
//     that rotate the bot. It is never re-read during the run.
//   - Each scaffold step uses scaffoldDirectStep (direct position packets) not
//     full NavigateTo, so fresh block placements do not confuse the planner.
//   - After PlaceBlockSurvival, we poll for the world block update before moving.
//   - Position is re-read from bot.Position() at the top of every iteration.
//   - If the place target overlaps the bot AABB, position is re-read and the
//     target recomputed; if still overlapping, PARTIAL reason=blocked_by_entity.
func executeScaffold(ctx context.Context, bot *feast.Client, length int, dir string, say func(string)) scaffoldResult {
	res := scaffoldResult{result: "PASS"}

	if length < 1 {
		length = 1
	}
	if length > 32 {
		length = 32
	}
	res.length = length

	// Lock direction NOW, before any block operations change yaw.
	_, _, _, yaw, _ := bot.GetPosition()
	dx, dz, cardinal := directionToDXDZ(dir, yaw)
	res.dir = cardinal

	// Need at least one placeable block; if none, PARTIAL immediately.
	_, stack, ok := findPlaceableHotbarItem(bot)
	if !ok {
		say(fmtLine("[scaffold] length=%d dir=%s block=none", length, cardinal))
		say("[scaffold] result=PARTIAL placed=0 moved_steps=0 climbed_steps=0 reason=no_placeable_blocks")
		res.result = "PARTIAL"
		res.reason = "no_placeable_blocks"
		return res
	}
	blockName, _ := feast.BlockNameFromItem(stack)
	res.block = normalizeBlockName(blockName)

	say(fmtLine("[scaffold] length=%d dir=%s block=%s", length, cardinal, res.block))

	view := botScaffoldView{bot: bot}

	finish := func(result, reason string) scaffoldResult {
		res.result = result
		res.reason = reason
		if reason == "" {
			say(fmtLine("[scaffold] result=%s placed=%d moved_steps=%d climbed_steps=%d",
				result, res.placed, res.movedSteps, res.climbedSteps))
		} else {
			say(fmtLine("[scaffold] result=%s placed=%d moved_steps=%d climbed_steps=%d reason=%s",
				result, res.placed, res.movedSteps, res.climbedSteps, reason))
		}
		return res
	}

	for i := 0; i < length; i++ {
		select {
		case <-ctx.Done():
			return finish("PARTIAL", "canceled")
		default:
		}

		// Always re-read feet from live position.
		feet := world.FeetBlockFromPosition(bot.Position())

		step := planScaffoldStep(view, feet, dx, dz)

		switch step.action {
		case scaffoldUnreachableHeight:
			return finish("PARTIAL", "height_unreachable")
		case scaffoldBlocked:
			return finish("PARTIAL", step.reason)
		}

		placed := false
		if step.place {
			if isBotOccupied(feet, step.placePos) {
				return finish("PARTIAL", "place_inside_bot")
			}

			// Check whether the placement target overlaps the bot AABB.
			// If it does, re-read position and recompute feet. If the overlap
			// persists after the re-read, the bot is physically in the way and
			// we must not attempt the placement.
			if blockAABB(step.placePos).Intersects(getBotAABB(bot)) {
				// Re-read live position and recheck.
				time.Sleep(100 * time.Millisecond)
				feet = world.FeetBlockFromPosition(bot.Position())
				if blockAABB(step.placePos).Intersects(getBotAABB(bot)) {
					say(fmtLine("[scaffold] step=%d current=%d,%d,%d next=%d,%d,%d ground=%d,%d,%d placed=false moved=false dist=0 reason=blocked_by_entity",
						i+1, feet.X, feet.Y, feet.Z,
						step.nextFeet.X, step.nextFeet.Y, step.nextFeet.Z,
						step.placePos.X, step.placePos.Y, step.placePos.Z))
					return finish("PARTIAL", "blocked_by_entity")
				}
				// Position shifted; replan this step.
				step = planScaffoldStep(view, feet, dx, dz)
				if step.action != scaffoldFlat && step.action != scaffoldClimb {
					return finish("PARTIAL", step.reason)
				}
			}

			slot, _, ok := findPlaceableHotbarItem(bot)
			if !ok {
				return finish("PARTIAL", "out_of_blocks")
			}
			if err := bot.SelectHotbarSlot(ctx, slot); err != nil {
				return finish("PARTIAL", "select_slot_failed")
			}
			_, face, found := findPlacementSupport(bot, step.placePos)
			if !found {
				return finish("PARTIAL", "no_placement_support")
			}
			if err := bot.PlaceBlockSurvival(ctx, step.placePos, face); err != nil {
				return finish("PARTIAL", "place_failed")
			}
			// Wait for server block-update to propagate before moving onto it.
			if !waitForSolid(ctx, bot, step.placePos, 600*time.Millisecond) {
				return finish("PARTIAL", "place_not_confirmed")
			}
			res.placed++
			placed = true
		}

		// Use direct position-packet movement, not full NavigateTo, for the
		// one-block adjacent step. See scaffoldDirectStep for rationale.
		stepCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
		err := scaffoldDirectStep(stepCtx, bot, step.nextFeet)
		cancel()
		if err != nil {
			detail := err.Error()
			say(fmtLine("[scaffold] step=%d move_failed detail=%s", i+1, detail))
			return finish("PARTIAL", "move_failed")
		}

		// Re-read and verify XZ after move.
		got := world.FeetBlockFromPosition(bot.Position())
		moved := got.X == step.nextFeet.X && got.Z == step.nextFeet.Z
		if !moved {
			return finish("PARTIAL", "move_off_target")
		}

		// Compute horizontal distance from start feet to current position.
		dist := math.Sqrt(math.Pow(float64(got.X-feet.X), 2) + math.Pow(float64(got.Z-feet.Z), 2))

		// When not placing, show the actual ground block under nextFeet for the log.
		groundPos := step.placePos
		if !step.place {
			groundPos = feast.BlockPos{X: step.nextFeet.X, Y: step.nextFeet.Y - 1, Z: step.nextFeet.Z}
		}

		say(fmtLine("[scaffold] step=%d current=%d,%d,%d next=%d,%d,%d ground=%d,%d,%d placed=%t moved=%t dist=%.2f",
			i+1,
			feet.X, feet.Y, feet.Z,
			step.nextFeet.X, step.nextFeet.Y, step.nextFeet.Z,
			groundPos.X, groundPos.Y, groundPos.Z,
			placed, moved, dist))

		if step.action == scaffoldClimb {
			res.climbedSteps++
		} else {
			res.movedSteps++
		}
	}

	return finish("PASS", "")
}
