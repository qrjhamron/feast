package executor

import (
	"testing"
	"time"

	"github.com/qrjhamron/feast/pkg/nav/move"
	"github.com/qrjhamron/feast/pkg/world"
)

// NOTE: the full Execute loop is real-time paced (one ~50ms tick per movement
// update), so benchmarking it end-to-end would measure wall-clock sleeps rather
// than CPU. These benchmarks instead exercise the CPU-bound per-tick pipeline
// the executor runs on every update (collision/hitbox checks, target position
// math, on_ground derivation, and stuck detection), which is the part worth
// optimizing.

func benchExecWorld() *world.World {
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			ch.SetBlock(x, 63, z, world.BlockState{Name: "stone", Solid: true})
		}
	}
	w.AddChunk(ch)
	return w
}

func BenchmarkExecutorHitboxCheck(b *testing.B) {
	w := benchExecWorld()
	b.ReportAllocs()
	b.ResetTimer()
	var ok bool
	for i := 0; i < b.N; i++ {
		ok = playerCollisionClear(w, 5.3, 64.0, 5.7)
	}
	_ = ok
}

// computeTick mirrors the executor's per-tick position pipeline (minus pacing).
func computeTick(w *world.World, cx, cy, cz, destX, destY, destZ float64) (float64, float64, bool) {
	m := move.MoveWalk{Dx: 1}
	speed := horizontalSpeedPerTick(m, false)
	tx, tz := nextHorizontalPosition(cx, cz, destX, destZ, speed)
	tx, tz = collisionAwareHorizontalPosition(w, cx, cz, tx, tz, cy)
	onGround := w.IsOnGround(world.Vec3{X: tx, Y: cy, Z: tz})
	_ = yawToFace(cx, cz, destX, destZ)
	return tx, tz, onGround
}

func BenchmarkExecutorMovementPacketGeneration(b *testing.B) {
	w := benchExecWorld()
	b.ReportAllocs()
	b.ResetTimer()
	var ax, az float64
	for i := 0; i < b.N; i++ {
		tx, tz, _ := computeTick(w, 5.5, 64.0, 5.5, 6.5, 64.0, 5.5)
		ax, az = tx, tz
	}
	_, _ = ax, az
}

// benchFlatPathCompute walks `steps` blocks east, 5 ticks per block, running the
// full per-tick compute pipeline (no real-time sleeps).
func benchFlatPathCompute(b *testing.B, steps int) {
	w := benchExecWorld()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cx, cy, cz := 0.5, 64.0, 0.5
		for s := 0; s < steps; s++ {
			destX, destY, destZ := float64(s+1)+0.5, 64.0, 0.5
			for tick := 0; tick < 5; tick++ {
				tx, tz, _ := computeTick(w, cx, cy, cz, destX, destY, destZ)
				cx, cz = tx, tz
			}
			_ = destY
		}
	}
}

func BenchmarkExecutorFlatPath10(b *testing.B)  { benchFlatPathCompute(b, 10) }
func BenchmarkExecutorFlatPath100(b *testing.B) { benchFlatPathCompute(b, 100) }

func BenchmarkExecutorStuckDetection(b *testing.B) {
	now := time.Unix(1000, 0)
	// ~120 samples spanning a 6s window at 50ms cadence, the realistic worst
	// case the detector scans each tick.
	base := make([]positionSample, 0, 120)
	for i := 0; i < 120; i++ {
		base = append(base, positionSample{
			at: now.Add(time.Duration(i-120) * 50 * time.Millisecond),
			x:  1 + float64(i)*0.001, y: 64, z: 1,
		})
	}
	b.ReportAllocs()
	b.ResetTimer()
	var stuckCount int
	for i := 0; i < b.N; i++ {
		h := make([]positionSample, len(base))
		copy(h, base)
		h = pruneHistory(h, 6*time.Second, now)
		if stuck(h, 5*time.Second, 0.1, now) {
			stuckCount++
		}
	}
	_ = stuckCount
}

func BenchmarkExecutorAdjacentStepCompute(b *testing.B) {
	// Compute-only cost of the adjacent-step physics for one walk tick.
	w := benchExecWorld()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cx, cy, cz := 5.5, 64.0, 5.5
		destX, destY, destZ := 6.5, 64.0, 5.5
		_ = reachedStepDestination(cx, cy, cz, destX, destY, destZ)
		tx, tz, _ := computeTick(w, cx, cy, cz, destX, destY, destZ)
		_, _ = tx, tz
	}
}
