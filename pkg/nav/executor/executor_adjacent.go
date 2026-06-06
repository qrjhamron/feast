package executor

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/qrjhamron/feast/pkg/nav/move"
	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/world"
)

// executeAdjacentStep drives the player from its current position to the
// standing center of an adjacent feet block (|dx|, |dy|, |dz| each <= 1) over a
// bounded number of ticks. It applies the same horizontal speed, collision
// sliding, gravity/jump model and fractional-aware on_ground semantics as the
// main Execute loop, so it produces server-plausible movement.
//
// Unlike Execute it performs NO planning, replanning, stuck-detection, history
// tracking or sprint management — the caller owns those concerns. It is the
// low-level one-block movement primitive intended to back future scaffold,
// tunnel-mining and staircase-mining routines, which all need a single reliable
// adjacent step (optionally after placing/breaking a block). It is intentionally
// unexported and is NOT part of the public API yet.
//
// Returns the per-call MovementStats and, on failure, a sentinel-wrapped error
// (errMovementStuck / errMovementTimeout) so callers can classify the outcome
// with errors.Is. A no-op (already at the target feet block) returns Noop=true,
// Reached=true and a nil error.
func executeAdjacentStep(ctx context.Context, client Client, w *world.World, toFeet [3]int, opt MovementOptions) (MovementStats, error) {
	if opt.Profile == "" {
		opt.Profile = MovementBotLike
	}

	var stats MovementStats
	sx, sy, sz, syaw, spitch := client.GetPosition()
	stats.StartX, stats.StartY, stats.StartZ = sx, sy, sz
	fromFeet := [3]int{int(math.Floor(sx)), int(math.Floor(sy)), int(math.Floor(sz))}

	dx := toFeet[0] - fromFeet[0]
	dy := toFeet[1] - fromFeet[1]
	dz := toFeet[2] - fromFeet[2]

	// Reject non-adjacent targets up front with a clear reason: this primitive
	// is defined only for single-block neighbors.
	if absI(dx) > 1 || absI(dy) > 1 || absI(dz) > 1 {
		stats.StuckReason = "target is not an adjacent block"
		stats.FinalX, stats.FinalY, stats.FinalZ = sx, sy, sz
		return stats, fmt.Errorf("%w: target %v not adjacent to %v", errMovementStuck, toFeet, fromFeet)
	}
	if dx == 0 && dy == 0 && dz == 0 {
		stats.Noop = true
		stats.Reached = true
		stats.FinalX, stats.FinalY, stats.FinalZ = sx, sy, sz
		return stats, nil
	}

	destX := float64(toFeet[0]) + 0.5
	destY := float64(toFeet[1])
	destZ := float64(toFeet[2]) + 0.5
	stats.HasFirstTargetNode = true
	stats.FirstTargetNodeX, stats.FirstTargetNodeY, stats.FirstTargetNodeZ = toFeet[0], toFeet[1], toFeet[2]

	// Choose the primitive only to size the tick budget and decide jump physics.
	var m move.Movement
	switch {
	case dy == 1:
		m = move.MoveJump{Dx: dx, Dz: dz}
	case dy == -1:
		m = move.MoveFall{Dx: dx, Dy: -1, Dz: dz}
	case dx != 0 && dz != 0:
		m = move.MoveWalkDiagonal{Dx: dx, Dz: dz}
	default:
		m = move.MoveWalk{Dx: dx, Dz: dz}
	}
	_, isJump := m.(move.MoveJump)
	ticks := movementTicks(m, false)
	if ticks <= 0 {
		ticks = 1
	}

	recordPacket := func(pkt *protocol.PlayServerboundSetPlayerPositionAndRotationPacket) error {
		stats.PacketsSent++
		if !stats.HasFirstPacketPos {
			stats.HasFirstPacketPos = true
			stats.FirstPacketX, stats.FirstPacketY, stats.FirstPacketZ = pkt.X, pkt.Y, pkt.Z
		}
		stats.LastPacketX, stats.LastPacketY, stats.LastPacketZ = pkt.X, pkt.Y, pkt.Z
		return client.WritePacket(pkt)
	}

	expected := time.Now()
stepLoop:
	for attempt := 0; attempt < 2; attempt++ {
		asx, asy, asz, _, _ := client.GetPosition()
		yPos := asy
		yVel := 0.0
		if isJump {
			yVel = 0.42
		}
		for i := 0; i < ticks; i++ {
			expected = expected.Add(50 * time.Millisecond)
			if err := sleepUntilTick(ctx, expected); err != nil {
				stats.FinalX, stats.FinalY, stats.FinalZ, _, _ = client.GetPosition()
				return stats, err
			}

			cx, cy, cz, cyaw, cpitch := client.GetPosition()
			ix, iy, iz := int(math.Floor(cx)), int(math.Floor(cy)), int(math.Floor(cz))
			if reachedStepDestination(cx, cy, cz, destX, destY, destZ) {
				stats.Reached = true
				break stepLoop
			}

			speed := horizontalSpeedPerTick(m, false)
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
			// Step-up snap: when a support block appears under the probed feet
			// and we still need to gain height, settle onto the block top.
			if !isJump && onGroundAt(w, int(math.Floor(tx)), int(math.Floor(ty)), int(math.Floor(tz))) && ty < destY && destY > cy {
				ty = destY
				yPos = ty
			}
			// on_ground reflects a genuine resting contact (fractional-aware);
			// a jump is never on the ground until it re-snaps to an integer Y.
			onGround := !isJump && w.IsOnGround(world.Vec3{X: tx, Y: ty, Z: tz})

			yaw := cyaw
			if rm, ok := m.(move.RotatingMovement); ok {
				yaw = rm.DesiredYaw(fromFeet)
			} else if math.Hypot(destX-cx, destZ-cz) > 0.001 {
				yaw = yawToFace(cx, cz, destX, destZ)
			}

			pkt := &protocol.PlayServerboundSetPlayerPositionAndRotationPacket{
				X: tx, Y: ty, Z: tz, Yaw: yaw, Pitch: cpitch, OnGround: onGround,
			}
			if err := recordPacket(pkt); err != nil {
				stats.FinalX, stats.FinalY, stats.FinalZ, _, _ = client.GetPosition()
				return stats, err
			}
		}

		ax, ay, az, _, _ := client.GetPosition()
		if reachedStepDestination(ax, ay, az, destX, destY, destZ) {
			stats.Reached = true
			break
		}
		// If the first attempt made no headway at all, a second attempt is
		// unlikely to help; still retry once to ride out a transient stall.
		_ = asx
		_ = asy
		_ = asz
	}

	fx, fy, fz, _, _ := client.GetPosition()
	stats.FinalX, stats.FinalY, stats.FinalZ = fx, fy, fz
	stats.DistanceTraveled = distance3D(sx, sy, sz, fx, fy, fz)
	stats.FinalDistance = distance3D(fx, fy, fz, destX, destY, destZ)
	_, _ = syaw, spitch

	if !stats.Reached {
		stats.StuckReason = "adjacent step did not reach target"
		return stats, fmt.Errorf("%w: adjacent step to %v did not reach target", errMovementStuck, toFeet)
	}
	return stats, nil
}

// sleepUntilTick waits until the next tick boundary, honoring context
// cancellation. A deadline cancellation maps to the movement-timeout sentinel.
func sleepUntilTick(ctx context.Context, until time.Time) error {
	d := time.Until(until)
	if d <= 0 {
		select {
		case <-ctx.Done():
			return ctxMovementErr(ctx)
		default:
			return nil
		}
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctxMovementErr(ctx)
	case <-timer.C:
		return nil
	}
}

func ctxMovementErr(ctx context.Context) error {
	if ctx.Err() == context.DeadlineExceeded {
		return errMovementTimeout
	}
	return ctx.Err()
}

func absI(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
