package feast

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync/atomic"
	"time"

	"github.com/qrjhamron/feast/pkg/nav/executor"
	"github.com/qrjhamron/feast/pkg/nav/goal"
	"github.com/qrjhamron/feast/pkg/nav/move"
	"github.com/qrjhamron/feast/pkg/nav/planner"
	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/state"
	"github.com/qrjhamron/feast/pkg/world"
)

// Advanced: NavigateTo2 cancels any existing navigation and starts a new one to the given coordinates.
// It is the low-level coordinate version; prefer [Client.NavigateTo] for goal-based navigation.
func (c *Client) NavigateTo2(x, y, z int) error {
	if !c.PositionSynced() {
		return ErrPositionNotSynced
	}
	w := c.World()
	if w == nil {
		c.logNavFailed("world_not_ready")
		c.bus.Emit(state.NavFailedEvent{Reason: "world not ready"})
		return ErrNotReady
	}

	c.StopNavigation()

	c.navMu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	c.navCtx = ctx
	c.navCancel = cancel
	c.navMu.Unlock()

	startXf, startYf, startZf, _, _ := c.GetPosition()
	startX := int(math.Floor(startXf))
	startY := int(math.Floor(startYf))
	startZ := int(math.Floor(startZf))
	g, goalY := c.navigationGoal(x, y, z, startY)

	c.bus.Emit(state.NavStartEvent{
		FromX: startX, FromY: startY, FromZ: startZ,
		ToX: x, ToY: goalY, ToZ: z,
	})

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer c.markManagedGoroutine()()
		defer c.clearNavigationContext(ctx)
		if !c.acquireNavigationMovement(ctx, time.Second) {
			c.logNavFailed("movement_authority_busy")
			c.bus.Emit(state.NavFailedEvent{Reason: "movement authority busy"})
			return
		}
		defer c.releaseNavigationMovement()

		lastProgressX, lastProgressY, lastProgressZ := startX, startY, startZ
		stagnantNavigationCycles := 0
		noteNavigationProgress := func(reason string) bool {
			ax, ay, az, _, _ := c.GetPosition()
			ix, iy, iz := int(math.Floor(ax)), int(math.Floor(ay)), int(math.Floor(az))
			if ix == lastProgressX && iy == lastProgressY && iz == lastProgressZ {
				stagnantNavigationCycles++
			} else {
				stagnantNavigationCycles = 0
				lastProgressX, lastProgressY, lastProgressZ = ix, iy, iz
			}
			if stagnantNavigationCycles >= 3 {
				c.logNavFailed(reason)
				c.bus.Emit(state.NavFailedEvent{Reason: reason})
				return false
			}
			return true
		}

		// Replan/execute loop: planner is time-bounded and may return partial paths.
		// Replan only on: cancellation, stuck (handled in executor), goal reached, or
		// a world block update that intersects the remaining path.
		for attempt := 1; ; attempt++ {
			select {
			case <-ctx.Done():
				c.logNavFailed("canceled")
				return
			default:
			}

			px, py, pz, _, _ := c.GetPosition()
			currX, currY, currZ := int(math.Floor(px)), int(math.Floor(py)), int(math.Floor(pz))
			if c.world == nil {
				c.logNavFailed("world_not_ready")
				c.bus.Emit(state.NavFailedEvent{Reason: "world not ready"})
				return
			}
			planY, ok := c.resolveStartY(currX, currY, currZ)
			if !ok {
				c.logNavFailed("chunk_not_loaded_at_feet")
				c.bus.Emit(state.NavFailedEvent{Reason: "surface not loaded at current position"})
				return
			}
			if g.Satisfied(currX, planY, currZ) {
				c.logNavArrived(currX, currY, currZ)
				c.bus.Emit(state.NavArrivedEvent{X: currX, Y: currY, Z: currZ})
				return
			}

			// Let planner choose adaptive timeout budgets by start-to-goal distance.
			planCtx, planCancel := context.WithCancel(ctx)
			c.logNavPlanning(attempt, currX, planY, currZ, x, goalY, z)

			dist := math.Abs(float64(currX-x)) + math.Abs(float64(planY-goalY)) + math.Abs(float64(currZ-z))
			using := "FlatAStar"
			if dist > 100 && c.hpaNav != nil {
				using = "HPA*"
			}
			if using == "HPA*" {
				deadline := time.Now().Add(2 * time.Second)
				for c.hpaClusters != nil && c.hpaClusters.BuiltCount() < 2 && time.Now().Before(deadline) {
					time.Sleep(100 * time.Millisecond)
				}
				err := c.hpaNav.Navigate(planCtx, newNavEventingClient(c), [3]int{currX, planY, currZ}, [3]int{x, goalY, z}, g)
				planCancel()
				if err != nil {
					goalChunkX := blockToChunkCoord(x)
					goalChunkZ := blockToChunkCoord(z)
					if !c.world.HasChunk(goalChunkX, goalChunkZ) {
						wpX, wpZ, ok := progressiveWaypoint(currX, currZ, x, z, c.world)
						if ok {
							wpGoal := goal.NewGoalBlock(wpX, goalY, wpZ)
							wpCtx, wpCancel := context.WithCancel(ctx)
							wpRes := planner.Plan(wpCtx, currX, planY, currZ, wpGoal, c.world)
							wpCancel()
							if wpRes.Status != planner.PlanNoPath && len(wpRes.Path) > 1 {
								c.logNavPathFound(len(wpRes.Path))
								execCtx, execCancel := context.WithCancel(ctx)
								err = executor.Execute(execCtx, newNavEventingClient(c), c.world, wpGoal, wpRes.Path, c.ActiveOptions())
								execCancel()
								if err != nil && execCtx.Err() == nil {
									c.logNavFailed(fmt.Sprintf("executor_error:%v", err))
									c.bus.Emit(state.NavFailedEvent{Reason: err.Error()})
									return
								}
								if err := c.sendOnGroundStabilize(5); err != nil {
									c.logNavFailed(fmt.Sprintf("stabilize_error:%v", err))
									c.bus.Emit(state.NavFailedEvent{Reason: err.Error()})
									return
								}
								select {
								case <-ctx.Done():
									return
								case <-time.After(3 * time.Second):
								}
								continue
							}
							if err := c.progressiveStrideToward(ctx, wpX, goalY, wpZ); err != nil {
								// Try the direct flat fallback below before surfacing a navigation failure.
							} else {
								if err := c.sendOnGroundStabilize(5); err != nil {
									c.logNavFailed(fmt.Sprintf("stabilize_error:%v", err))
									c.bus.Emit(state.NavFailedEvent{Reason: err.Error()})
									return
								}
								time.Sleep(100 * time.Millisecond)
								if !noteNavigationProgress("stuck_after_progressive_cycles") {
									return
								}
								select {
								case <-ctx.Done():
									return
								case <-time.After(3 * time.Second):
								}
								continue
							}
						}
					}
					flatCtx, flatCancel := context.WithCancel(ctx)
					res := planner.Plan(flatCtx, currX, planY, currZ, g, c.world)
					flatCancel()
					if res.Status == planner.PlanNoPath || len(res.Path) == 0 {
						c.logNavFailed("no_path")
						c.bus.Emit(state.NavFailedEvent{Reason: "no path found"})
						return
					}
					c.logNavPathFound(len(res.Path))
					path := res.Path
					execCtx, execCancel := context.WithCancel(ctx)
					err = executor.Execute(execCtx, newNavEventingClient(c), c.world, g, path, c.ActiveOptions())
					execCancel()
					if err != nil && execCtx.Err() == nil {
						c.logNavFailed(fmt.Sprintf("executor_error:%v", err))
						c.bus.Emit(state.NavFailedEvent{Reason: err.Error()})
						return
					}
					if err := c.sendOnGroundStabilize(5); err != nil {
						c.logNavFailed(fmt.Sprintf("stabilize_error:%v", err))
						c.bus.Emit(state.NavFailedEvent{Reason: err.Error()})
						return
					}
					if !noteNavigationProgress("stuck_after_replans") {
						return
					}
					time.Sleep(100 * time.Millisecond)
					continue
				}
				continue
			}

			res := planner.Plan(planCtx, currX, planY, currZ, g, c.world)
			planCancel()
			if res.Status == planner.PlanNoPath || len(res.Path) == 0 {
				c.logNavFailed("no_path")
				c.bus.Emit(state.NavFailedEvent{Reason: "no path found"})
				return
			}
			c.logNavPathFound(len(res.Path))
			path := res.Path

			// Replan trigger: if a block update intersects our remaining path, cancel
			// the current executor run and restart planning from the next loop iteration.
			remaining := buildPathFootprint([3]int{currX, planY, currZ}, path)
			replanCh := make(chan struct{}, 1)
			execCtx, execCancel := context.WithCancel(ctx)
			blockUpdateHandlerID, onErr := c.bus.On("block_update", func(e state.Event) {
				planner.InvalidateCache()
				ev, ok := e.(state.BlockUpdateEvent)
				if !ok {
					return
				}
				if execCtx.Err() != nil || ctx.Err() != nil {
					return
				}
				if footprintIntersects(remaining, int(ev.X), int(ev.Y), int(ev.Z)) {
					select {
					case replanCh <- struct{}{}:
					default:
					}
				}
			})
			if onErr != nil {
				execCancel()
				c.logNavFailed(fmt.Sprintf("handler_error:%v", onErr))
				c.bus.Emit(state.NavFailedEvent{Reason: onErr.Error()})
				return
			}
			sectionUpdateHandlerID, onErr := c.bus.On("section_blocks_update", func(e state.Event) {
				planner.InvalidateCache()
				ev, ok := e.(state.SectionBlocksUpdateEvent)
				if !ok {
					return
				}
				if execCtx.Err() != nil || ctx.Err() != nil {
					return
				}
				for _, u := range ev.Updates {
					if footprintIntersects(remaining, int(u.X), int(u.Y), int(u.Z)) {
						select {
						case replanCh <- struct{}{}:
						default:
						}
						return
					}
				}
			})
			if onErr != nil {
				execCancel()
				c.bus.Off(blockUpdateHandlerID)
				c.logNavFailed(fmt.Sprintf("handler_error:%v", onErr))
				c.bus.Emit(state.NavFailedEvent{Reason: onErr.Error()})
				return
			}
			go func() {
				select {
				case <-execCtx.Done():
				case <-replanCh:
					execCancel()
				}
			}()

			err := executor.Execute(execCtx, newNavEventingClient(c), c.world, g, path, c.ActiveOptions())
			execCancel()
			c.bus.Off(blockUpdateHandlerID)
			c.bus.Off(sectionUpdateHandlerID)
			if err != nil && execCtx.Err() == nil {
				c.logNavFailed(fmt.Sprintf("executor_error:%v", err))
				c.bus.Emit(state.NavFailedEvent{Reason: err.Error()})
				return
			}
			if err := c.sendOnGroundStabilize(5); err != nil {
				c.logNavFailed(fmt.Sprintf("stabilize_error:%v", err))
				c.bus.Emit(state.NavFailedEvent{Reason: err.Error()})
				return
			}
			if !noteNavigationProgress("stuck_after_replans") {
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()

	return nil
}

func (c *Client) acquireNavigationMovement(ctx context.Context, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		c.moveMu.Lock()
		if !c.moving {
			c.moving = true
			c.moveMu.Unlock()
			return true
		}
		c.moveMu.Unlock()
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

func (c *Client) releaseNavigationMovement() {
	c.moveMu.Lock()
	c.moving = false
	c.moveMu.Unlock()
}

func (c *Client) logNavFailed(reason string) {
	log.Printf("[nav] failed reason=%s", reason)
}

func (c *Client) logNavArrived(x, y, z int) {
	log.Printf("[nav] arrived at (%d,%d,%d)", x, y, z)
}

func (c *Client) logNavPlanning(attempt, fromX, fromY, fromZ, toX, toY, toZ int) {
	log.Printf("[nav] planning attempt=%d from=(%d,%d,%d) to=(%d,%d,%d)", attempt, fromX, fromY, fromZ, toX, toY, toZ)
}

func (c *Client) logNavPathFound(steps int) {
	log.Printf("[nav] path found steps=%d", steps)
}

func blockToChunkCoord(block int) int {
	chunk := block / 16
	if block < 0 && block%16 != 0 {
		chunk--
	}
	return chunk
}

func (c *Client) resolveGoalY(goalX, goalZ, fallbackY int) int {
	surfaceY := c.world.GetSurfaceY(goalX, goalZ)
	if surfaceY != world.UnknownSurfaceY && surfaceY > world.MinY-1 && c.isStandableAt(goalX, surfaceY+1, goalZ) {
		return surfaceY + 1
	}
	return fallbackY
}

func (c *Client) navigationGoal(x, y, z, fallbackY int) (goal.Goal, int) {
	if y == 0 {
		return goal.NewGoalXZ(x, z), c.resolveGoalY(x, z, fallbackY)
	}
	return goal.NewGoalProximity(x, y, z, 2), y
}

func (c *Client) resolveStartY(currX, currY, currZ int) (int, bool) {
	surfaceY := c.world.GetSurfaceY(currX, currZ)
	if surfaceY == world.UnknownSurfaceY {
		return currY, false
	}
	if surfaceY <= world.MinY-1 {
		return currY, true
	}
	resolved := surfaceY + 1
	if !c.isStandableAt(currX, resolved, currZ) {
		return currY, true
	}
	if currY > resolved+3 || currY < resolved-6 {
		return resolved, true
	}
	return currY, true
}

func (c *Client) isStandableAt(x, y, z int) bool {
	return c.world != nil &&
		c.world.IsPassable(x, y, z) &&
		c.world.IsPassable(x, y+1, z) &&
		!c.world.IsPassable(x, y-1, z)
}

func progressiveWaypoint(currX, currZ, goalX, goalZ int, w *world.World) (int, int, bool) {
	dx := float64(goalX - currX)
	dz := float64(goalZ - currZ)
	totalDist := math.Hypot(dx, dz)
	if totalDist < 1 {
		return goalX, goalZ, true
	}
	unitX := dx / totalDist
	unitZ := dz / totalDist

	minStep := 16
	maxStep := int(totalDist)
	if maxStep < minStep {
		maxStep = minStep
	}

	lastLoadedX := currX
	lastLoadedZ := currZ
	lastLoadedDist := 0
	foundUnloaded := false
	for d := 1; d <= maxStep; d++ {
		tx := currX + int(math.Round(unitX*float64(d)))
		tz := currZ + int(math.Round(unitZ*float64(d)))
		if !w.HasChunk(blockToChunkCoord(tx), blockToChunkCoord(tz)) {
			foundUnloaded = true
			break
		}
		lastLoadedX = tx
		lastLoadedZ = tz
		lastLoadedDist = d
	}

	if foundUnloaded && lastLoadedDist >= minStep {
		return lastLoadedX, lastLoadedZ, true
	}

	// Ensure at least 16 blocks forward progress per leg.
	targetDist := minStep
	if targetDist > maxStep {
		targetDist = maxStep
	}
	wpX := currX + int(math.Round(unitX*float64(targetDist)))
	wpZ := currZ + int(math.Round(unitZ*float64(targetDist)))
	return wpX, wpZ, true
}

func (c *Client) sendOnGroundStabilize(times int) error {
	x, y, z, yaw, pitch := c.GetPosition()
	blockX := int(math.Floor(x))
	blockY := int(math.Floor(y))
	blockZ := int(math.Floor(z))
	onGround := c.onGroundAt(blockX, blockY, blockZ)
	for i := 0; i < times; i++ {
		pkt := &protocol.PlayServerboundSetPlayerPositionAndRotationPacket{
			X: x, Y: y, Z: z, Yaw: yaw, Pitch: pitch, OnGround: onGround,
		}
		if err := c.WritePacket(pkt); err != nil {
			return err
		}
		time.Sleep(50 * time.Millisecond)
	}
	return nil
}

func (c *Client) progressiveStrideToward(ctx context.Context, targetX, _ /*targetY*/, targetZ int) error {
	const (
		WalkSpeed = 0.215
		TickRate  = 50 * time.Millisecond
	)
	x0, _, z0, _, _ := c.GetPosition()
	feetX := int(math.Floor(x0))
	feetZ := int(math.Floor(z0))
	cx, cz := blockToChunkCoord(feetX), blockToChunkCoord(feetZ)
	if !c.World().IsChunkLoaded(cx, cz) {
		return ErrChunkNotLoaded
	}

	seq := atomic.LoadUint64(&c.positionSyncSeq)
	lastCorrX, lastCorrZ := math.MaxFloat64, math.MaxFloat64
	sameCorrCount := 0
	angles := []float64{0, 20, -20, 40, -40, 60, -60, 90, -90}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		x, y, z, yaw, pitch := c.GetPosition()
		dx := float64(targetX) - x
		dz := float64(targetZ) - z
		dist := math.Hypot(dx, dz)
		if dist <= WalkSpeed {
			blockX := targetX
			blockY := int(math.Floor(y))
			blockZ := targetZ
			pkt := &protocol.PlayServerboundSetPlayerPositionAndRotationPacket{
				X: float64(targetX), Y: y, Z: float64(targetZ), Yaw: yaw, Pitch: pitch, OnGround: c.onGroundAt(blockX, blockY, blockZ),
			}
			if err := c.WritePacket(pkt); err != nil {
				return err
			}
			return nil
		}
		ux := dx / dist
		uz := dz / dist
		var (
			nx, ny, nz  float64
			passable    bool
			ground      bool
			sawUnloaded bool
			foundStep   bool
		)
		airMode := sameCorrCount >= 3
		for _, deg := range angles {
			rad := deg * math.Pi / 180.0
			rx := ux*math.Cos(rad) - uz*math.Sin(rad)
			rz := ux*math.Sin(rad) + uz*math.Cos(rad)
			candX := x + rx*WalkSpeed
			candZ := z + rz*WalkSpeed
			candBlockX := int(math.Floor(candX))
			candBlockZ := int(math.Floor(candZ))
			if !c.world.HasChunk(blockToChunkCoord(candBlockX), blockToChunkCoord(candBlockZ)) {
				sawUnloaded = true
				continue
			}
			surfaceY := c.world.GetSurfaceY(candBlockX, candBlockZ)
			if surfaceY == world.UnknownSurfaceY || surfaceY <= world.MinY-1 {
				continue
			}
			footY := surfaceY + 1
			if airMode {
				footY = surfaceY + 2
			}
			// Keep within normal step-up/step-down range unless we are in air-mode recovery.
			if !airMode && math.Abs(float64(footY)-y) > 1.25 {
				continue
			}
			passable = c.world.IsPassable(candBlockX, footY, candBlockZ) && c.world.IsPassable(candBlockX, footY+1, candBlockZ)
			ground = !c.world.IsPassable(candBlockX, footY-1, candBlockZ)
			if !passable {
				continue
			}
			if !airMode && !ground {
				continue
			}
			nx, ny, nz = candX, float64(footY), candZ
			foundStep = true
			break
		}
		if !foundStep {
			if sawUnloaded {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(250 * time.Millisecond):
					continue
				}
			}
			// The local surface cache can lag behind the server-accepted player
			// height immediately after teleports or corrections. Fall back to a
			// tiny same-level step; the correction guard below will stop if the
			// server rejects this position repeatedly.
			nx, ny, nz = x+ux*WalkSpeed, y, z+uz*WalkSpeed
		}
		stepBlockX := int(math.Floor(nx))
		stepBlockY := int(math.Floor(ny))
		stepBlockZ := int(math.Floor(nz))
		pkt := &protocol.PlayServerboundSetPlayerPositionAndRotationPacket{
			X: nx, Y: ny, Z: nz, Yaw: yaw, Pitch: pitch, OnGround: c.onGroundAt(stepBlockX, stepBlockY, stepBlockZ),
		}
		if err := c.WritePacket(pkt); err != nil {
			return err
		}

		newSeq := atomic.LoadUint64(&c.positionSyncSeq)
		if newSeq != seq {
			seq = newSeq
			cx, _, cz, _, _ := c.GetPosition()
			if math.Abs(cx-lastCorrX) <= 0.05 && math.Abs(cz-lastCorrZ) <= 0.05 {
				sameCorrCount++
			} else {
				lastCorrX, lastCorrZ = cx, cz
				sameCorrCount = 0
			}
			if sameCorrCount >= 10 {
				return fmt.Errorf("rubber-band detected after %d repeated corrections at (%.2f,%.2f)", sameCorrCount, cx, cz)
			}
		}
		time.Sleep(TickRate)
	}
}

type footprintCell struct {
	x, y, z int
}

func buildPathFootprint(start [3]int, path []move.Movement) map[footprintCell]struct{} {
	out := make(map[footprintCell]struct{}, len(path)*2)
	out[footprintCell{x: start[0], y: start[1], z: start[2]}] = struct{}{}
	pos := start
	for _, m := range path {
		switch mm := m.(type) {
		case move.MoveWalkLine:
			for i := 0; i < mm.Steps; i++ {
				pos = [3]int{pos[0] + mm.Dx, pos[1], pos[2] + mm.Dz}
				out[footprintCell{x: pos[0], y: pos[1], z: pos[2]}] = struct{}{}
			}
		case move.MoveWalkDiagonalLine:
			for i := 0; i < mm.Steps; i++ {
				pos = [3]int{pos[0] + mm.Dx, pos[1], pos[2] + mm.Dz}
				out[footprintCell{x: pos[0], y: pos[1], z: pos[2]}] = struct{}{}
			}
		default:
			pos = m.Destination(pos)
			out[footprintCell{x: pos[0], y: pos[1], z: pos[2]}] = struct{}{}
		}
	}
	return out
}

func footprintIntersects(fp map[footprintCell]struct{}, x, y, z int) bool {
	// Conservative: changes to the target block, headroom, or floor can invalidate the step.
	for dy := -1; dy <= 2; dy++ {
		if _, ok := fp[footprintCell{x: x, y: y + dy, z: z}]; ok {
			return true
		}
	}
	return false
}

func yawToFace(fromX, fromZ, toX, toZ int) float32 {
	dx := float64(toX - fromX)
	dz := float64(toZ - fromZ)
	if dx == 0 && dz == 0 {
		return 0
	}
	return float32(-math.Atan2(dx, dz) * 180 / math.Pi)
}

type navEventingClient struct {
	c *Client

	lastBlockPos [3]int
	lastMovedAt  time.Time
	emittedStuck bool
	debugPackets int
}

func newNavEventingClient(c *Client) *navEventingClient {
	return &navEventingClient{
		c:            c,
		lastBlockPos: [3]int{math.MinInt, math.MinInt, math.MinInt},
		lastMovedAt:  time.Now(),
	}
}

func (n *navEventingClient) GetPosition() (x, y, z float64, yaw, pitch float32) {
	return n.c.GetPosition()
}

func (n *navEventingClient) EntityID() int32 { return n.c.EntityID() }

func (n *navEventingClient) TrackMovementStats(stats executor.MovementStats) {
	n.c.statsTrackMu.Lock()
	n.c.lastMovementStats = stats
	n.c.statsTrackMu.Unlock()
}

func (n *navEventingClient) PositionSyncSeq() uint64 {
	return n.c.PositionSyncSeq()
}

func (n *navEventingClient) WritePacket(p protocol.Packet) error {
	if pkt, ok := p.(*protocol.PlayServerboundSetPlayerPositionAndRotationPacket); ok {
		if n.c.opts.Debug && n.debugPackets < 5 {
			n.debugPackets++
			log.Printf("[move-debug] packet=%d x=%.3f y=%.3f z=%.3f yaw=%.2f pitch=%.2f on_ground=%v",
				n.debugPackets, pkt.X, pkt.Y, pkt.Z, pkt.Yaw, pkt.Pitch, pkt.OnGround)
		}
		n.c.teleportMu.Lock()
		n.c.teleportMu.Unlock()
		n.c.stateMu.Lock()
		n.c.player.X = pkt.X
		n.c.player.Y = pkt.Y
		n.c.player.Z = pkt.Z
		n.c.player.Yaw = pkt.Yaw
		n.c.player.Pitch = pkt.Pitch
		n.c.player.OnGround = pkt.OnGround
		n.c.stateMu.Unlock()
		n.c.bus.Emit(state.NavStepEvent{X: pkt.X, Y: pkt.Y, Z: pkt.Z})

		bp := [3]int{int(math.Floor(pkt.X)), int(math.Floor(pkt.Y)), int(math.Floor(pkt.Z))}
		if bp != n.lastBlockPos {
			n.lastBlockPos = bp
			n.lastMovedAt = time.Now()
			n.emittedStuck = false
		} else if !n.emittedStuck && time.Since(n.lastMovedAt) >= 3*time.Second {
			n.emittedStuck = true
			n.c.bus.Emit(state.NavStuckEvent{
				X: bp[0], Y: bp[1], Z: bp[2],
				Reason: "no movement observed in last 3s",
			})
		}
	}
	return n.c.WritePacket(p)
}

// Advanced: StopNavigation cleanly cancels the active navigation task.
func (c *Client) StopNavigation() {
	c.navMu.Lock()
	defer c.navMu.Unlock()
	if c.navCancel != nil {
		c.navCancel()
		c.navCancel = nil
		c.navCtx = nil
	}
}

func (c *Client) clearNavigationContext(ctx context.Context) {
	c.navMu.Lock()
	defer c.navMu.Unlock()
	if c.navCtx == ctx {
		c.navCtx = nil
		c.navCancel = nil
	}
}
