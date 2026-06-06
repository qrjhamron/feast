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

type MovementProfile string

const (
	MovementBotLike   MovementProfile = "bot_like"
	MovementHumanLike MovementProfile = "human_like"
)

type MovementOptions struct {
	Profile   MovementProfile
	Tolerance float64
	Timeout   time.Duration
}

type MovementStats struct {
	PacketsSent         int
	ServerPositionsSeen int
	CorrectionsSeen     int
	TeleportsSeen       int
	Reached             bool
	StartX              float64
	StartY              float64
	StartZ              float64
	FinalX              float64
	FinalY              float64
	FinalZ              float64
	DistanceTraveled    float64
	FinalDistance       float64
	HasFirstTargetNode  bool
	FirstTargetNodeX    int
	FirstTargetNodeY    int
	FirstTargetNodeZ    int
	HasFirstPacketPos   bool
	FirstPacketX        float64
	FirstPacketY        float64
	FirstPacketZ        float64
	LastPacketX         float64
	LastPacketY         float64
	LastPacketZ         float64
	LastCorrectionX     float64
	LastCorrectionY     float64
	LastCorrectionZ     float64
	StuckReason         string
}

type MoveErrorReason string

const (
	MoveNoPath           MoveErrorReason = "no_path"
	MoveTimeout          MoveErrorReason = "timeout"
	MoveStuck            MoveErrorReason = "stuck"
	MoveServerCorrection MoveErrorReason = "server_correction"
)

type MoveResult struct {
	Reached             bool
	Reason              MoveErrorReason
	PacketsSent         int
	ServerPositionsSeen int
	CorrectionsSeen     int
	FinalDistance       float64
}

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

type resultTrackingClient struct {
	Client
	stats MovementStats
}

func (r *resultTrackingClient) TrackMovementStats(stats MovementStats) {
	r.stats = stats
}

func ExecuteWithResult(ctx context.Context, client Client, w *world.World, g goal.Goal, initialPath []move.Movement, opts ...MovementOptions) (MoveResult, error) {
	if len(initialPath) == 0 {
		cx, cy, cz, _, _ := client.GetPosition()
		return MoveResult{Reached: false, Reason: MoveNoPath, FinalDistance: finalDistanceToGoal(cx, cy, cz, g)}, fmt.Errorf("movement no path")
	}
	tracker := &resultTrackingClient{Client: client}
	err := Execute(ctx, tracker, w, g, initialPath, opts...)
	res := MoveResult{
		Reached:             tracker.stats.Reached,
		PacketsSent:         tracker.stats.PacketsSent,
		ServerPositionsSeen: tracker.stats.ServerPositionsSeen,
		CorrectionsSeen:     tracker.stats.CorrectionsSeen,
		FinalDistance:       tracker.stats.FinalDistance,
	}
	if err == nil {
		res.Reached = true
		return res, nil
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "timeout") || ctx != nil && ctx.Err() == context.DeadlineExceeded:
		res.Reason = MoveTimeout
	case strings.Contains(msg, "stuck"):
		res.Reason = MoveStuck
	case res.CorrectionsSeen > 0:
		res.Reason = MoveServerCorrection
	default:
		res.Reason = MoveNoPath
	}
	return res, err
}

func finalDistanceToGoal(x, y, z float64, g goal.Goal) float64 {
	switch v := g.(type) {
	case *goal.GoalBlock:
		return distance3D(x, y, z, float64(v.X)+0.5, float64(v.Y), float64(v.Z)+0.5)
	case *goal.GoalProximity:
		return distance3D(x, y, z, float64(v.X)+0.5, float64(v.Y), float64(v.Z)+0.5)
	case *goal.GoalXZ:
		return math.Hypot(x-(float64(v.X)+0.5), z-(float64(v.Z)+0.5))
	default:
		return 0
	}
}

func yawToFace(fromX, fromZ, toX, toZ float64) float32 {
	dx := toX - fromX
	dz := toZ - fromZ
	if dx == 0 && dz == 0 {
		return 0
	}
	return float32(-math.Atan2(dx, dz) * 180 / math.Pi)
}

func lerpYaw(current, target float32, maxStep float32) float32 {
	diff := target - current
	for diff < -180 {
		diff += 360
	}
	for diff > 180 {
		diff -= 360
	}
	if diff > maxStep {
		return current + maxStep
	}
	if diff < -maxStep {
		return current - maxStep
	}
	return target
}

// Execute executes a path to a goal, handling stuck detection and replanning.
func Execute(ctx context.Context, client Client, w *world.World, g goal.Goal, initialPath []move.Movement, opts ...MovementOptions) (err error) {
	var opt MovementOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	if opt.Profile == "" {
		opt.Profile = MovementBotLike
	}
	if opt.Profile != MovementBotLike && opt.Profile != MovementHumanLike {
		opt.Profile = MovementBotLike
	}

	if opt.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opt.Timeout)
		defer cancel()
	}

	var targetX, targetY, targetZ float64
	var hasTarget bool
	if gb, ok := g.(*goal.GoalBlock); ok {
		targetX, targetY, targetZ = float64(gb.X)+0.5, float64(gb.Y), float64(gb.Z)+0.5
		hasTarget = true
	} else if gp, ok := g.(*goal.GoalProximity); ok {
		targetX, targetY, targetZ = float64(gp.X)+0.5, float64(gp.Y), float64(gp.Z)+0.5
		hasTarget = true
	} else if gz, ok := g.(*goal.GoalXZ); ok {
		targetX, targetZ = float64(gz.X)+0.5, float64(gz.Z)+0.5
		hasTarget = true
	}

	var stats MovementStats
	var firstTargetX, firstTargetY, firstTargetZ float64
	startX, startY, startZ, _, _ := client.GetPosition()
	stats.StartX = startX
	stats.StartY = startY
	stats.StartZ = startZ
	defer func() {
		cx, cy, cz, _, _ := client.GetPosition()
		stats.FinalX = cx
		stats.FinalY = cy
		stats.FinalZ = cz
		stats.DistanceTraveled = distance3D(startX, startY, startZ, cx, cy, cz)
		if hasTarget {
			if _, ok := g.(*goal.GoalXZ); ok {
				stats.FinalDistance = math.Hypot(cx-targetX, cz-targetZ)
			} else {
				stats.FinalDistance = distance3D(cx, cy, cz, targetX, targetY, targetZ)
			}
		}
		if err == nil {
			stats.Reached = true
		} else if stats.StuckReason == "" {
			stats.StuckReason = err.Error()
		}
		if st, ok := client.(interface{ TrackMovementStats(MovementStats) }); ok {
			st.TrackMovementStats(stats)
		}
		nodeStr := "none"
		centerStr := "none"
		if stats.HasFirstTargetNode {
			nodeStr = fmt.Sprintf("(%d,%d,%d)", stats.FirstTargetNodeX, stats.FirstTargetNodeY, stats.FirstTargetNodeZ)
			centerStr = fmt.Sprintf("(%.3f,%.3f,%.3f)", firstTargetX, firstTargetY, firstTargetZ)
		}
		fmt.Printf("[move] local_astar_node=%s\n", nodeStr)
		fmt.Printf("[move] executor_target_center=%s\n", centerStr)
		fmt.Printf("[move] distance_traveled=%.3f\n", stats.DistanceTraveled)
		fmt.Printf("[move] corrections_seen=%d\n", stats.CorrectionsSeen)
		fmt.Printf("[move] reached=%t\n", stats.Reached)
	}()

	var lastSeq uint64
	var hasSeq bool
	var seqTracker interface{ PositionSyncSeq() uint64 }
	if ct, ok := client.(interface{ PositionSyncSeq() uint64 }); ok {
		seqTracker = ct
		lastSeq = seqTracker.PositionSyncSeq()
		hasSeq = true
	}

	select {
	case <-ctx.Done():
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("movement timeout exceeded")
		}
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

	// Check if already satisfied or within tolerance at start
	if hasTarget && opt.Tolerance > 0 {
		var dist float64
		if _, ok := g.(*goal.GoalXZ); ok {
			dist = math.Hypot(cx-targetX, cz-targetZ)
		} else {
			dist = distance3D(cx, cy, cz, targetX, targetY, targetZ)
		}
		if dist <= opt.Tolerance {
			return nil
		}
	}
	if g.Satisfied(currentPos[0], currentPos[1], currentPos[2]) {
		return nil
	}

	writePacket := func(p protocol.Packet) error {
		stats.PacketsSent++
		if pkt, ok := p.(*protocol.PlayServerboundSetPlayerPositionAndRotationPacket); ok {
			if !stats.HasFirstPacketPos {
				stats.HasFirstPacketPos = true
				stats.FirstPacketX = pkt.X
				stats.FirstPacketY = pkt.Y
				stats.FirstPacketZ = pkt.Z
			}
			stats.LastPacketX = pkt.X
			stats.LastPacketY = pkt.Y
			stats.LastPacketZ = pkt.Z
		}
		return client.WritePacket(p)
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
		if hasTarget && opt.Tolerance > 0 {
			var dist float64
			if _, ok := g.(*goal.GoalXZ); ok {
				dist = math.Hypot(cx-targetX, cz-targetZ)
			} else {
				dist = distance3D(cx, cy, cz, targetX, targetY, targetZ)
			}
			if dist <= opt.Tolerance {
				return nil
			}
		}
		if g.Satisfied(currentPos[0], currentPos[1], currentPos[2]) {
			return nil
		}

		edgeStart := currentPos
		dest := m.Destination(edgeStart)
		destX := float64(dest[0]) + 0.5
		destY := float64(dest[1])
		destZ := float64(dest[2]) + 0.5
		if !stats.HasFirstTargetNode {
			stats.HasFirstTargetNode = true
			stats.FirstTargetNodeX = dest[0]
			stats.FirstTargetNodeY = dest[1]
			stats.FirstTargetNodeZ = dest[2]
			firstTargetX = destX
			firstTargetY = destY
			firstTargetZ = destZ
		}

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
						if ctx.Err() == context.DeadlineExceeded {
							return fmt.Errorf("movement timeout exceeded")
						}
						return ctx.Err()
					case <-timer.C:
					}
				} else {
					select {
					case <-ctx.Done():
						if ctx.Err() == context.DeadlineExceeded {
							return fmt.Errorf("movement timeout exceeded")
						}
						return ctx.Err()
					default:
					}
				}

				cx, cy, cz, cyaw, cpitch = client.GetPosition()
				ix, iy, iz := int(math.Floor(cx)), int(math.Floor(cy)), int(math.Floor(cz))
				if hasTarget && opt.Tolerance > 0 {
					var dist float64
					if _, ok := g.(*goal.GoalXZ); ok {
						dist = math.Hypot(cx-targetX, cz-targetZ)
					} else {
						dist = distance3D(cx, cy, cz, targetX, targetY, targetZ)
					}
					if dist <= opt.Tolerance {
						return nil
					}
				}
				if g.Satisfied(ix, iy, iz) {
					return nil
				}

				if hasSeq {
					currSeq := seqTracker.PositionSyncSeq()
					if currSeq != lastSeq {
						stats.CorrectionsSeen++
						stats.ServerPositionsSeen++
						stats.TeleportsSeen++
						lastSeq = currSeq
						cx, cy, cz, cyaw, cpitch = client.GetPosition()
						stats.LastCorrectionX = cx
						stats.LastCorrectionY = cy
						stats.LastCorrectionZ = cz
						yPos = cy
					}
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
					if err := writePacket(pkt); err != nil {
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
						if ctx.Err() == context.DeadlineExceeded {
							return fmt.Errorf("movement timeout exceeded")
						}
						return ctx.Err()
					}
					if len(res.Path) == 0 {
						stats.StuckReason = fmt.Sprintf("replanning produced empty path (status=%v)", res.Status)
						return fmt.Errorf("movement stuck: replanning produced empty path (status=%v)", res.Status)
					}
					path = res.Path
					replanned = true
					currentPlanTotal = len(path)
					currentPlanStep = 0
					history = nil
					break
				}

				speed := horizontalSpeedPerTick(m, sprinting)
				if opt.Profile == MovementHumanLike {
					progress := float64(i+1) / float64(ticks)
					mult := math.Sin(progress * math.Pi)
					if mult < 0.5 {
						mult = 0.5
					}
					speed = speed * mult
				}

				tx, tz := nextHorizontalPosition(cx, cz, destX, destZ, speed)
				tx, tz = collisionAwareHorizontalPosition(w, cx, cz, tx, tz, cy)
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

				var targetYaw float32
				if rm, ok := m.(move.RotatingMovement); ok {
					targetYaw = rm.DesiredYaw(edgeStart)
				} else if dx, dz := destX-cx, destZ-cz; math.Hypot(dx, dz) > 0.001 {
					targetYaw = yawToFace(cx, cz, destX, destZ)
				} else {
					targetYaw = cyaw
				}

				if opt.Profile == MovementHumanLike {
					cyaw = lerpYaw(cyaw, targetYaw, 20.0)
				} else {
					cyaw = targetYaw
				}

				pkt := protocol.PlayServerboundSetPlayerPositionAndRotationPacket{
					X:        tx,
					Y:        ty,
					Z:        tz,
					Yaw:      cyaw,
					Pitch:    cpitch,
					OnGround: onGround,
				}
				if err := writePacket(&pkt); err != nil {
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
			stats.StuckReason = "did not advance after retry"
			return fmt.Errorf("movement stuck: did not advance after retry")
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

func collisionAwareHorizontalPosition(w *world.World, cx, cz, tx, tz, y float64) (float64, float64) {
	if playerCollisionClear(w, tx, y, tz) {
		return tx, tz
	}

	candidates := [][2]float64{
		{tx, cz},
		{cx, tz},
	}
	for _, cand := range candidates {
		if playerCollisionClear(w, cand[0], y, cand[1]) {
			return cand[0], cand[1]
		}
	}

	for scale := 0.5; scale >= 0.125; scale *= 0.5 {
		candX := cx + (tx-cx)*scale
		candZ := cz + (tz-cz)*scale
		if playerCollisionClear(w, candX, y, candZ) {
			return candX, candZ
		}
	}
	return tx, tz
}

func playerCollisionClear(w *world.World, x, y, z float64) bool {
	if w == nil {
		return true
	}
	const (
		halfWidth = 0.3
		epsilon   = 1.0e-7
	)
	minX := int(math.Floor(x - halfWidth + epsilon))
	maxX := int(math.Floor(x + halfWidth - epsilon))
	minZ := int(math.Floor(z - halfWidth + epsilon))
	maxZ := int(math.Floor(z + halfWidth - epsilon))
	feetY := int(math.Floor(y))

	for bx := minX; bx <= maxX; bx++ {
		for bz := minZ; bz <= maxZ; bz++ {
			for _, by := range []int{feetY, feetY + 1} {
				pos := world.BlockPos{X: int32(bx), Y: int32(by), Z: int32(bz)}
				if !w.IsBlockLoaded(pos) {
					continue
				}
				if !w.IsPassable(bx, by, bz) {
					return false
				}
			}
		}
	}
	return true
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
