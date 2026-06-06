package world

import (
	"sync"
	"testing"

	"github.com/qrjhamron/feast/pkg/protocol"
)

func TestEntityStore_ConcurrentUpdateRemove(t *testing.T) {
	s := NewEntityStore()
	s.Upsert(&Entity{ID: 1, X: 10, Y: 64, Z: 10})

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				s.UpdateDelta(1, 1, 0, -1)
				s.UpdateRotation(1, 90, 0)
				_, _ = s.Get(1)
				_ = s.All()
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for j := 0; j < 1000; j++ {
			s.Upsert(&Entity{ID: 2, X: float64(j), Y: 64, Z: 0})
			s.Remove(2)
		}
	}()

	wg.Wait()

	if _, ok := s.Get(1); !ok {
		t.Fatalf("entity 1 missing")
	}
}

func TestEntityStore_GetReturnsSnapshot(t *testing.T) {
	s := NewEntityStore()
	s.Upsert(&Entity{ID: 1, X: 10, Y: 64, Z: 10, Yaw: 15})

	got, ok := s.Get(1)
	if !ok {
		t.Fatalf("expected entity to exist")
	}
	got.X = 999
	got.Yaw = 180

	got2, ok := s.Get(1)
	if !ok {
		t.Fatalf("expected entity to exist")
	}
	if got2.X != 10 || got2.Yaw != 15 {
		t.Fatalf("store mutated through snapshot: %+v", got2)
	}
}

func TestEntityStore_UpdatePositionAndVelocity(t *testing.T) {
	s := NewEntityStore()
	s.Upsert(&Entity{ID: 1, X: 10, Y: 64, Z: 10})

	s.UpdatePosition(1, 20.5, 65.0, 30.5)
	got, ok := s.Get(1)
	if !ok || got.X != 20.5 || got.Y != 65.0 || got.Z != 30.5 {
		t.Fatalf("expected pos updated: %+v", got)
	}

	s.UpdateVelocity(1, 8000, -4000, 0)
	got, _ = s.Get(1)
	if got.VX != 1.0 || got.VY != -0.5 || got.VZ != 0.0 {
		t.Fatalf("expected velocity updated: %+v", got)
	}
}

func TestEntityStore_MathScaling(t *testing.T) {
	s := NewEntityStore()
	// Relative Movement
	s.Upsert(&Entity{ID: 2, X: 10, Y: 64, Z: -5})
	s.UpdateDelta(2, 4096, -2048, 8192)
	got, _ := s.Get(2)
	if got.X != 11.0 || got.Y != 63.5 || got.Z != -3.0 { // X+1.0, Y-0.5, Z+2.0
		t.Fatalf("expected relative pos X=11.0, Y=63.5, Z=-3.0, got X=%.1f, Y=%.1f, Z=%.1f", got.X, got.Y, got.Z)
	}

	// Velocity Scaling
	s.Upsert(&Entity{ID: 3, X: 0, Y: 0, Z: 0})
	s.UpdateVelocity(3, 8000, -4000, 0)
	got, _ = s.Get(3)
	if got.VX != 1.0 || got.VY != -0.5 || got.VZ != 0.0 {
		t.Fatalf("expected velocity VX=1.0, VY=-0.5, VZ=0.0, got VX=%.1f, VY=%.1f, VZ=%.1f", got.VX, got.VY, got.VZ)
	}
}

func TestEntityHitboxesAndCollisions(t *testing.T) {
	// 1. Standing/sneaking/swimming/crawling hitboxes
	playerStanding := HitboxForEntityType("player", protocol.PoseStanding)
	if playerStanding.MaxY != 1.8 || playerStanding.MaxX != 0.3 || playerStanding.MinX != -0.3 {
		t.Errorf("unexpected player standing hitbox: %+v", playerStanding)
	}

	playerSneaking := HitboxForEntityType("player", protocol.PoseSneaking)
	if playerSneaking.MaxY != 1.5 {
		t.Errorf("unexpected player sneaking hitbox: %+v", playerSneaking)
	}

	playerSwimming := HitboxForEntityType("player", protocol.PoseSwimming)
	if playerSwimming.MaxY != 0.6 {
		t.Errorf("unexpected player swimming hitbox: %+v", playerSwimming)
	}

	// 2. Collision overlap / No collision / Entity removal
	w := NewWorld()
	store := NewEntityStore()
	w.SetEntityStore(store)

	ent := &Entity{
		ID:   10,
		Type: "zombie",
		X:    10.0,
		Y:    64.0,
		Z:    10.0,
		Pose: protocol.PoseStanding,
	}
	store.Upsert(ent)

	// Target bounding box that overlaps the zombie
	// Zombie hitbox should be centered at 10.0, 10.0 and feet at 64.0.
	// Bounding box: Min [9.9, 64.0, 9.9] Max [10.1, 65.0, 10.1]
	boxOverlapping := AABB{
		MinX: 9.9, MinY: 64.0, MinZ: 9.9,
		MaxX: 10.1, MaxY: 65.0, MaxZ: 10.1,
	}
	if !w.IsEntityBlocking(boxOverlapping) {
		t.Errorf("expected entity to block box")
	}

	// Bounding box that does NOT overlap the zombie (e.g. far away)
	boxFar := AABB{
		MinX: 0.0, MinY: 64.0, MinZ: 0.0,
		MaxX: 1.0, MaxY: 65.0, MaxZ: 1.0,
	}
	if w.IsEntityBlocking(boxFar) {
		t.Errorf("expected entity to NOT block box")
	}

	// 3. Entity removal
	store.Remove(10)
	if w.IsEntityBlocking(boxOverlapping) {
		t.Errorf("expected box to NOT be blocked after entity removal")
	}
}
