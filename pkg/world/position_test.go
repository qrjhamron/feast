package world

import "testing"

func TestFeetBlockFromPosition(t *testing.T) {
	tests := []struct {
		name string
		pos  Vec3
		want BlockPos
	}{
		{"resting integer", Vec3{X: 10.5, Y: 57.0, Z: -3.5}, BlockPos{X: 10, Y: 57, Z: -4}},
		{"falling fractional", Vec3{X: 10.5, Y: 57.7, Z: -3.5}, BlockPos{X: 10, Y: 57, Z: -4}},
		{"zero", Vec3{X: 0, Y: 0, Z: 0}, BlockPos{X: 0, Y: 0, Z: 0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FeetBlockFromPosition(tt.pos); got != tt.want {
				t.Fatalf("FeetBlockFromPosition(%v) = %v; want %v", tt.pos, got, tt.want)
			}
		})
	}
}

func TestGroundAndHeadBlockSemantics(t *testing.T) {
	feet := BlockPos{X: 10, Y: 57, Z: -3}

	ground := GroundBlockFromFeet(feet)
	if ground != (BlockPos{X: 10, Y: 56, Z: -3}) {
		t.Fatalf("GroundBlockFromFeet = %v; want feetY-1", ground)
	}

	head := HeadBlockFromFeet(feet)
	if head != (BlockPos{X: 10, Y: 58, Z: -3}) {
		t.Fatalf("HeadBlockFromFeet = %v; want feetY+1", head)
	}

	center := StandingCenter(feet)
	if center.X != 10.5 || center.Y != 57.0 || center.Z != -2.5 {
		t.Fatalf("StandingCenter = %v; want (10.5,57,-2.5)", center)
	}
}

func TestNegativeCoordinateFeetBlock(t *testing.T) {
	// Negative coordinates must floor toward negative infinity, not truncate.
	pos := Vec3{X: -198.332, Y: 57.0, Z: -3.5}
	got := FeetBlockFromPosition(pos)
	want := BlockPos{X: -199, Y: 57, Z: -4}
	if got != want {
		t.Fatalf("FeetBlockFromPosition(%v) = %v; want %v", pos, got, want)
	}

	// A negative fractional feet Y still floors down.
	feet := FeetBlockFromPosition(Vec3{X: -1.0, Y: -0.3, Z: -1.0})
	if feet.Y != -1 {
		t.Fatalf("negative fractional feet Y floored to %d; want -1", feet.Y)
	}
}

func TestIsOnGroundSemantics(t *testing.T) {
	w := NewWorld()
	ch := NewChunk(0, 0)
	// Solid floor at y=56, air above.
	ch.SetBlock(2, 56, 2, BlockState{Name: "stone", Solid: true})
	w.AddChunk(ch)

	// Resting exactly on the block top (feet at y=57.0, support at 56).
	if !w.IsOnGround(Vec3{X: 2.5, Y: 57.0, Z: 2.5}) {
		t.Fatal("resting on block top must report on_ground=true")
	}
	// Falling: same feet block but Y still above the block top.
	if w.IsOnGround(Vec3{X: 2.5, Y: 57.6, Z: 2.5}) {
		t.Fatal("descending Y above block top must report on_ground=false")
	}
	// No support below.
	if w.IsOnGround(Vec3{X: 2.5, Y: 70.0, Z: 2.5}) {
		t.Fatal("no support below must report on_ground=false")
	}
	// Unloaded chunk is optimistic.
	if !w.IsOnGround(Vec3{X: 1000.5, Y: 57.0, Z: 1000.5}) {
		t.Fatal("unloaded chunk should be optimistic on_ground=true")
	}
}

func onGroundWorld(t *testing.T) *World {
	t.Helper()
	w := NewWorld()
	ch := NewChunk(0, 0)
	// Solid stone platform at y=63 under the column (8,*,8).
	ch.SetBlock(8, 63, 8, BlockState{Name: "stone", Solid: true})
	w.AddChunk(ch)
	return w
}

func TestOnGroundTrueOnlyWithSupport(t *testing.T) {
	w := onGroundWorld(t)
	// Feet at y=64.0 rest on the block top at 63.
	if !w.IsOnGround(Vec3{X: 8.5, Y: 64.0, Z: 8.5}) {
		t.Fatal("supported + resting must be on_ground=true")
	}
	// Same feet block but no support (move column to 9,*,9 which is air).
	if w.IsOnGround(Vec3{X: 9.5, Y: 64.0, Z: 9.5}) {
		t.Fatal("no support must be on_ground=false")
	}
}

func TestOnGroundFalseWhileFalling(t *testing.T) {
	w := onGroundWorld(t)
	// Well above the platform, descending.
	if w.IsOnGround(Vec3{X: 8.5, Y: 70.3, Z: 8.5}) {
		t.Fatal("airborne must be on_ground=false")
	}
}

func TestOnGroundFalseWhileYChangingDown(t *testing.T) {
	w := onGroundWorld(t)
	// Feet block is 64 (support below at 63) but Y is mid-block descending.
	if w.IsOnGround(Vec3{X: 8.5, Y: 64.45, Z: 8.5}) {
		t.Fatal("Y above block top with support below must still be on_ground=false")
	}
}

func TestOnGroundAfterLanding(t *testing.T) {
	w := onGroundWorld(t)
	// After landing, Y snaps to the integer block top.
	if !w.IsOnGround(Vec3{X: 8.5, Y: 64.0, Z: 8.5}) {
		t.Fatal("post-landing resting position must be on_ground=true")
	}
}
