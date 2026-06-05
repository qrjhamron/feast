package goal

import (
	"testing"

	"github.com/qrjhamron/feast/pkg/world"
)

func TestGoalXZ(t *testing.T) {
	g := NewGoalXZ(10, 20)

	if !g.Satisfied(10, 5, 20) {
		t.Errorf("GoalXZ should be satisfied at (10, 5, 20)")
	}
	if g.Satisfied(10, 5, 21) {
		t.Errorf("GoalXZ should not be satisfied at (10, 5, 21)")
	}

	h1 := g.Heuristic(10, 5, 20)
	h2 := g.Heuristic(0, 5, 0)

	if h1 != 0 {
		t.Errorf("Heuristic should be 0 at target, got %f", h1)
	}
	if h2 <= h1 {
		t.Errorf("Heuristic should be larger further away")
	}
}

func TestGoalBlock(t *testing.T) {
	g := NewGoalBlock(10, 15, 20)

	if !g.Satisfied(10, 15, 20) {
		t.Errorf("GoalBlock should be satisfied at (10, 15, 20)")
	}
	if g.Satisfied(10, 16, 20) {
		t.Errorf("GoalBlock should not be satisfied at (10, 16, 20)")
	}

	h1 := g.Heuristic(10, 15, 20)
	h2 := g.Heuristic(0, 0, 0)

	if h1 != 0 {
		t.Errorf("Heuristic should be 0 at target, got %f", h1)
	}
	if h2 <= h1 {
		t.Errorf("Heuristic should be larger further away")
	}
}

func TestGoalProximity(t *testing.T) {
	g := NewGoalProximity(10, 15, 20, 5.0)

	if !g.Satisfied(12, 15, 20) {
		t.Errorf("GoalProximity should be satisfied at (12, 15, 20)")
	}
	if !g.Satisfied(10, 15, 25) {
		t.Errorf("GoalProximity should be satisfied at (10, 15, 25)")
	}
	if g.Satisfied(0, 0, 0) {
		t.Errorf("GoalProximity should not be satisfied at (0, 0, 0)")
	}

	h1 := g.Heuristic(12, 15, 20)
	h2 := g.Heuristic(20, 15, 20)

	if h1 != 0 {
		t.Errorf("Heuristic should be 0 at target, got %f", h1)
	}
	if h2 <= h1 {
		t.Errorf("Heuristic should be larger further away")
	}
}

func TestGoalNear(t *testing.T) {
	e := &world.Entity{X: 10, Y: 64, Z: 10}
	g := NewGoalNear(e, 2.0)
	if !g.Satisfied(11, 64, 10) {
		t.Fatalf("expected near goal to be satisfied")
	}
	e.X = 30
	if g.Satisfied(11, 64, 10) {
		t.Fatalf("expected near goal to update with entity movement")
	}
}

func TestGoalY(t *testing.T) {
	g := NewGoalY(80)
	if !g.Satisfied(0, 80, 0) {
		t.Fatalf("expected goalY satisfied")
	}
	if g.Satisfied(0, 79, 0) {
		t.Fatalf("expected goalY not satisfied")
	}
	if g.Heuristic(0, 78, 0) <= 0 {
		t.Fatalf("expected positive heuristic away from target Y")
	}
}

func TestGoalComposite(t *testing.T) {
	xz := NewGoalXZ(10, 10)
	y := NewGoalY(64)
	all := NewGoalComposite(CompositeAll, xz, y)
	any := NewGoalComposite(CompositeAny, xz, y)
	if all.Satisfied(10, 63, 10) {
		t.Fatalf("expected AND goal to fail")
	}
	if !any.Satisfied(10, 63, 10) {
		t.Fatalf("expected OR goal to satisfy")
	}
}
