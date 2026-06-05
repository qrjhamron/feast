package executor

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/qrjhamron/feast/pkg/nav/goal"
	"github.com/qrjhamron/feast/pkg/nav/move"
	"github.com/qrjhamron/feast/pkg/nav/planner"
	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/world"
)

type Client interface {
	GetPosition() (x, y, z float64, yaw, pitch float32)
	WritePacket(p protocol.Packet) error
}

type movementAuthority interface {
	AcquireMovement() bool
	ReleaseMovement()
	IsMoving() bool
}

type movementAuthorityBy interface {
	AcquireMovementBy(by string) bool
	ReleaseMovementBy(by string)
}

type entityIDProvider interface {
	EntityID() int32
}

type positionSample struct {
	at      time.Time
	x, y, z float64
}

// Execute executes a path to a goal, handling stuck detection and replanning.
func Execute(ctx context.Context, client Client, w *world.World, g goal.Goal, initialPath []move.Movement) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	if mab, ok := client.(movementAuthorityBy); ok {
		if !mab.AcquireMovementBy("executor") {
			return fmt.Errorf("movement authority busy")
		}
		defer mab.ReleaseMovementBy("executor")
	} else if ma, ok := client.(movementAuthority); ok {
		if !ma.AcquireMovement() {
			return fmt.Errorf("movement authority busy")
		}
		defer ma.ReleaseMovement()
	}

	path := expandLineMovements(initialPath)
	cx, cy, cz, cyaw, cpitch := client.GetPosition()
	currentPos := [3]int{int(math.Floor(cx)), int(math.Floor(cy)), int(math.Floor(cz))}
	var history []positionSample

	if g.Satisfied(currentPos[0], currentPos[1], currentPos[2]) {
		return nil
	}

	// Track sprinting based on water transitions.
	sprinting := false
	inWater := inWaterAt(w, int(math.Floor(cx)), int(math.Floor(cy)), int(math.Floor(cz)))

	currentPlanTotal := len(path)
	currentPlanStep := 0
	expectedTime := time.Now()
	for len(path) > 0 {
		if currentPlanTotal == 0 {
			currentPlanTotal = len(path)
		}
		currentPlanStep++

		m := path[0]
		path = path[1:]

		cx, cy, cz, cyaw, cpitch = client.GetPosition()
		currentPos = [3]int{int(math.Floor(cx)), int(math.Floor(cy)), int(math.Floor(cz))}
		if g.Satisfied(currentPos[0], currentPos[1], currentPos[2]) {
			return nil
		}

		edgeStart := currentPos
		dest := m.Destination(edgeStart)
		destX := float64(dest[0]) + 0.5
		destY := float64(dest[1])
		destZ := float64(dest[2]) + 0.5

		ticks := movementTicks(m, sprinting)
		if cost := m.Cost(w, edgeStart); !math.IsInf(cost, 1) && !math.IsNaN(cost) {
			minTicks := int(math.Ceil(cost))
			if minTicks > ticks {
				ticks = minTicks
			}
		}
		if ticks <= 0 {
			ticks = 1
		}

		replanned := false
		stepAdvanced := false
		for attempt := 0; attempt < 2; attempt++ {
			attemptStartX, attemptStartY, attemptStartZ, _, _ := client.GetPosition()
			var yVel float64
			yPos := attemptStartY
			isJump := false
			switch m.(type) {
			case move.MoveJump:
				yVel = 0.42
				isJump = true
			}
			for i := 0; i < ticks; i++ {
				expectedTime = expectedTime.Add(50 * time.Millisecond)
				sleep := time.Until(expectedTime)
				if sleep > 0 {
					timer := time.NewTimer(sleep)
					select {
					case <-ctx.Done():
						timer.Stop()
						return ctx.Err()
					case <-timer.C:
					}
				} else {
					select {
					case <-ctx.Done():
						return ctx.Err()
					default:
					}
				}

				cx, cy, cz, cyaw, cpitch = client.GetPosition()
				ix, iy, iz := int(math.Floor(cx)), int(math.Floor(cy)), int(math.Floor(cz))
				if g.Satisfied(ix, iy, iz) {
					return nil
				}
				nowInWater := inWaterAt(w, ix, iy, iz)
				if idp, ok := client.(entityIDProvider); ok && nowInWater != inWater {
					action := protocol.PlayerCommandStartSprinting
					if nowInWater {
						action = protocol.PlayerCommandStopSprinting
					}
					pkt := &protocol.PlayServerboundPlayerCommandPacket{
						EntityID:  idp.EntityID(),
						ActionID:  action,
						JumpBoost: 0,
					}
					if err := client.WritePacket(pkt); err != nil {
						return err
					}
					sprinting = !nowInWater
					inWater = nowInWater
				} else {
					inWater = nowInWater
				}

				actPos := [3]int{ix, iy, iz}

				// Record movement history (for stuck detection).
				now := time.Now()
				history = append(history, positionSample{at: now, x: cx, y: cy, z: cz})
				history = pruneHistory(history, 6*time.Second, now)

				if stuck(history, 5*time.Second, 0.1, now) {
					avoid := dest
					res := planAvoid(ctx, actPos, g, w, avoid)
					if res.Status == planner.PlanCancelled {
						return ctx.Err()
					}
					if len(res.Path) == 0 {
						return fmt.Errorf("planner: replanning produced empty path (status=%v)", res.Status)
					}
					path = res.Path
					replanned = true
					currentPlanTotal = len(path)
					currentPlanStep = 0
					history = nil
					break
				}

				tx, tz := nextHorizontalPosition(cx, cz, destX, destZ, horizontalSpeedPerTick(m, sprinting))
				if isJump {
					if i == 0 {
						yPos += yVel
					} else {
						yVel = (yVel - 0.08) * 0.98
						yPos += yVel
					}
				} else if onGroundAt(w, ix, iy, iz) {
					yVel = 0
					yPos = cy
				} else {
					yVel = (yVel - 0.08) * 0.98
					yPos = cy + yVel
				}
				ty := yPos
				probeOnGround := onGroundAt(w, int(math.Floor(tx)), int(math.Floor(yPos)), int(math.Floor(tz)))
				if !isJump && probeOnGround && ty < destY && destY > cy {
					ty = destY
					yPos = ty
				}
				onGround := onGroundAt(w, int(math.Floor(tx)), int(math.Floor(ty)), int(math.Floor(tz)))
				if onGround {
					yVel = 0
				}

				if rm, ok := m.(move.RotatingMovement); ok {
					cyaw = rm.DesiredYaw(edgeStart)
				}

				pkt := protocol.PlayServerboundSetPlayerPositionAndRotationPacket{
					X:        tx,
					Y:        ty,
					Z:        tz,
					Yaw:      cyaw,
					Pitch:    cpitch,
					OnGround: onGround,
				}
				if err := client.WritePacket(&pkt); err != nil {
					return err
				}
			}
			if replanned {
				break
			}

			ax, ay, az, _, _ := client.GetPosition()
			if reachedStepDestination(ax, ay, az, destX, destY, destZ) {
				stepAdvanced = true
				break
			}
			if attempt == 0 && distance3D(attemptStartX, attemptStartY, attemptStartZ, ax, ay, az) >= 0.1 {
				continue
			}
		}

		if replanned {
			continue
		}
		if !stepAdvanced {
			return fmt.Errorf("movement did not advance after retry")
		}

		currentPos = dest
	}

	return nil
}

func movementTicks(m move.Movement, sprinting bool) int {
	const (
		walkTicksPerBlock = 5  // 250ms
		jumpTicks         = 10 // 500ms
	)

	ticks := 5
	switch mm := m.(type) {
	case move.MoveWalk:
		ticks = walkTicksPerBlock
	case move.MoveSwim:
		ticks = 20
	case move.MoveClimb:
		ticks = 8
	case move.MoveWalkDiagonal:
		ticks = 7
	case move.MoveWalkLine:
		ticks = walkTicksPerBlock * mm.Steps
	case move.MoveWalkDiagonalLine:
		ticks = walkTicksPerBlock * mm.Steps
	case move.MoveJump:
		ticks = jumpTicks
	case move.MoveFall:
		// Falls are driven by physics; this is a conservative cap so we keep
		// sending updates while descending.
		ticks = int(math.Max(1, math.Ceil(math.Abs(float64(mm.Dy))*10)))
	case move.MoveParkour:
		ticks = jumpTicks
	default:
		// Unknown movement type: still send a few updates so we don't appear idle.
		ticks = walkTicksPerBlock
	}

	if sprinting {
		switch m.(type) {
		case move.MoveWalk, move.MoveWalkDiagonal, move.MoveWalkLine, move.MoveWalkDiagonalLine:
			ticks = int(math.Ceil(float64(ticks) / 1.3))
			if ticks <= 0 {
				ticks = 1
			}
		}
	}
	return ticks
}

func expandLineMovements(path []move.Movement) []move.Movement {
	out := make([]move.Movement, 0, len(path))
	for _, m := range path {
		switch mm := m.(type) {
		case move.MoveWalkLine:
			for i := 0; i < mm.Steps; i++ {
				out = append(out, move.MoveWalk{Dx: mm.Dx, Dz: mm.Dz})
			}
		case move.MoveWalkDiagonalLine:
			for i := 0; i < mm.Steps; i++ {
				out = append(out, move.MoveWalkDiagonal{Dx: mm.Dx, Dz: mm.Dz})
			}
		default:
			out = append(out, m)
		}
	}
	return out
}

func horizontalSpeedPerTick(m move.Movement, sprinting bool) float64 {
	speed := 0.215
	if sprinting {
		speed = 0.280
	}
	switch m.(type) {
	case move.MoveSwim:
		return 0.100
	case move.MoveClimb:
		return 0.1176
	default:
		return speed
	}
}

func nextHorizontalPosition(cx, cz, destX, destZ, maxStep float64) (float64, float64) {
	dx := destX - cx
	dz := destZ - cz
	dist := math.Hypot(dx, dz)
	if dist == 0 || dist <= maxStep {
		return destX, destZ
	}
	scale := maxStep / dist
	return cx + dx*scale, cz + dz*scale
}

func reachedStepDestination(x, y, z, destX, destY, destZ float64) bool {
	horizontal := math.Hypot(destX-x, destZ-z)
	return horizontal <= 0.35 && math.Abs(destY-y) <= 2.0
}

func pruneHistory(h []positionSample, window time.Duration, now time.Time) []positionSample {
	cutoff := now.Add(-window)
	keep := 0
	for keep < len(h) && h[keep].at.Before(cutoff) {
		keep++
	}
	if keep == 0 {
		return h
	}
	out := make([]positionSample, 0, len(h)-keep)
	out = append(out, h[keep:]...)
	return out
}

func stuck(h []positionSample, lookback time.Duration, threshold float64, now time.Time) bool {
	cutoff := now.Add(-lookback)
	// Require we actually have samples spanning the full lookback window,
	// otherwise we'd classify "not moved yet" as stuck immediately.
	if len(h) == 0 || h[0].at.After(cutoff) {
		return false
	}
	var (
		minX, maxX float64
		minY, maxY float64
		minZ, maxZ float64
		inited     bool
	)
	for i := len(h) - 1; i >= 0; i-- {
		if h[i].at.Before(cutoff) {
			break
		}
		if !inited {
			minX, maxX = h[i].x, h[i].x
			minY, maxY = h[i].y, h[i].y
			minZ, maxZ = h[i].z, h[i].z
			inited = true
			continue
		}
		if h[i].x < minX {
			minX = h[i].x
		}
		if h[i].x > maxX {
			maxX = h[i].x
		}
		if h[i].y < minY {
			minY = h[i].y
		}
		if h[i].y > maxY {
			maxY = h[i].y
		}
		if h[i].z < minZ {
			minZ = h[i].z
		}
		if h[i].z > maxZ {
			maxZ = h[i].z
		}
	}
	if !inited {
		return false
	}
	dx := maxX - minX
	dy := maxY - minY
	dz := maxZ - minZ
	return math.Sqrt(dx*dx+dy*dy+dz*dz) < threshold
}

func distance3D(ax, ay, az, bx, by, bz float64) float64 {
	dx := bx - ax
	dy := by - ay
	dz := bz - az
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

func planAvoid(ctx context.Context, start [3]int, g goal.Goal, w *world.World, avoid [3]int) planner.PlanResult {
	return planner.PlanWithExclusions(ctx, start[0], start[1], start[2], g, w, [][3]int{avoid})
}

func inWaterAt(w *world.World, x, y, z int) bool {
	if w == nil {
		return false
	}
	b, err := w.GetBlock(x, y, z)
	if err != nil {
		return false
	}
	n := strings.ToLower(b.Name)
	return n == "water" || strings.HasSuffix(n, ":water")
}

func onGroundAt(w *world.World, feetX, feetY, feetZ int) bool {
	if w == nil {
		return true
	}
	chunkX := floorDiv(feetX, world.ChunkWidth)
	chunkZ := floorDiv(feetZ, world.ChunkDepth)
	if !w.HasChunk(chunkX, chunkZ) {
		return true
	}
	return !w.IsPassable(feetX, feetY-1, feetZ)
}

func floorDiv(a, b int) int {
	q := a / b
	r := a % b
	if r != 0 && ((r < 0) != (b < 0)) {
		q--
	}
	return q
}
