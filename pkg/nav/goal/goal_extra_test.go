package goal

import (
	"math"
	"testing"

	"github.com/qrjhamron/feast/pkg/world"
)

// countingGoal records how many times Satisfied/Heuristic are invoked so tests
// can assert short-circuit behavior in composites.
type countingGoal struct {
	inner          Goal
	satisfiedCalls *int
}

func (g countingGoal) Satisfied(x, y, z int) bool {
	*g.satisfiedCalls++
	return g.inner.Satisfied(x, y, z)
}

func (g countingGoal) Heuristic(x, y, z int) float64 { return g.inner.Heuristic(x, y, z) }

func TestGoalBlockReached(t *testing.T) {
	g := NewGoalBlock(-3, 70, 12)
	if !g.Satisfied(-3, 70, 12) {
		t.Fatal("expected exact block to be reached")
	}
	for _, off := range [][3]int{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}, {-1, 0, 0}} {
		if g.Satisfied(-3+off[0], 70+off[1], 12+off[2]) {
			t.Fatalf("block goal must not be satisfied at offset %v", off)
		}
	}
}

func TestGoalXZIgnoresY(t *testing.T) {
	g := NewGoalXZ(5, -9)
	for _, y := range []int{-64, 0, 63, 200, 319} {
		if !g.Satisfied(5, y, -9) {
			t.Fatalf("GoalXZ must ignore Y, failed at y=%d", y)
		}
	}
	if g.Satisfied(6, 64, -9) || g.Satisfied(5, 64, -8) {
		t.Fatal("GoalXZ must still require X and Z to match")
	}
}

func TestGoalNegativeCoordinates(t *testing.T) {
	// Heuristics must be symmetric around the origin and never negative.
	gb := NewGoalBlock(-10, 64, -10)
	hNeg := gb.Heuristic(-20, 64, -20)
	hPos := gb.Heuristic(0, 64, 0)
	if hNeg <= 0 || hPos <= 0 {
		t.Fatalf("expected positive heuristics, got neg=%f pos=%f", hNeg, hPos)
	}
	if math.Abs(hNeg-hPos) > 1e-9 {
		t.Fatalf("heuristic must be symmetric across origin: neg=%f pos=%f", hNeg, hPos)
	}
	if md := gb.ManhattanDistance(-20, 64, -20); md != 20 {
		t.Fatalf("manhattan distance at negative coords = %d, want 20", md)
	}
}

func TestGoalNearRadiusBoundary(t *testing.T) {
	e := &world.Entity{X: -4.0, Y: 64.0, Z: 8.0}
	g := NewGoalNear(e, 2.0)

	// Just inside the radius (distance 2 exactly is inclusive).
	if !g.Satisfied(-6, 64, 8) {
		t.Fatal("expected satisfied exactly at radius boundary (distance 2.0)")
	}
	// Just outside the radius.
	if g.Satisfied(-7, 64, 8) {
		t.Fatal("expected NOT satisfied just outside radius (distance 3.0)")
	}
	// Heuristic must clamp to zero once inside the radius and be positive outside.
	if h := g.Heuristic(-5, 64, 8); h != 0 {
		t.Fatalf("expected zero heuristic inside radius, got %f", h)
	}
	if h := g.Heuristic(-20, 64, 8); h <= 0 {
		t.Fatalf("expected positive heuristic far outside radius, got %f", h)
	}

	// A nil entity must be safe and never satisfied.
	var nilGoal *GoalNear
	if nilGoal.Satisfied(0, 0, 0) {
		t.Fatal("nil GoalNear must not be satisfied")
	}
	empty := NewGoalNear(nil, 2.0)
	if empty.Satisfied(0, 0, 0) || !math.IsInf(empty.Heuristic(0, 0, 0), 1) {
		t.Fatal("GoalNear with nil entity must be unsatisfied with +Inf heuristic")
	}
}

func TestGoalCompositeShortCircuit(t *testing.T) {
	// CompositeAny must stop at the first satisfied sub-goal.
	calls := 0
	first := countingGoal{inner: NewGoalXZ(1, 1), satisfiedCalls: &calls}
	second := countingGoal{inner: NewGoalXZ(2, 2), satisfiedCalls: &calls}
	any := NewGoalComposite(CompositeAny, first, second)
	if !any.Satisfied(1, 0, 1) {
		t.Fatal("expected CompositeAny satisfied via first sub-goal")
	}
	if calls != 1 {
		t.Fatalf("CompositeAny must short-circuit after first satisfied sub-goal, got %d calls", calls)
	}

	// CompositeAll must stop at the first UNsatisfied sub-goal.
	calls = 0
	all := NewGoalComposite(CompositeAll, first, second)
	if all.Satisfied(9, 0, 9) {
		t.Fatal("expected CompositeAll unsatisfied")
	}
	if calls != 1 {
		t.Fatalf("CompositeAll must short-circuit after first unsatisfied sub-goal, got %d calls", calls)
	}

	// Empty composite is never satisfied and has an +Inf heuristic.
	emptyAll := NewGoalComposite(CompositeAll)
	if emptyAll.Satisfied(0, 0, 0) || !math.IsInf(emptyAll.Heuristic(0, 0, 0), 1) {
		t.Fatal("empty composite must be unsatisfied with +Inf heuristic")
	}
}

// TestGoalHeuristicMonotonicEnough documents the cost model the planner relies
// on: the raw (unscaled) octile heuristic decreases by exactly 1.0 per cardinal
// step toward the goal and sqrt(2) per diagonal step, and is never negative.
// The planner scales this by 0.5 to obtain an admissible lower bound (see the
// octileHeuristic doc comment).
func TestGoalHeuristicMonotonicEnough(t *testing.T) {
	g := NewGoalBlock(10, 64, 0)

	prev := g.Heuristic(0, 64, 0)
	for x := 1; x <= 10; x++ {
		cur := g.Heuristic(x, 64, 0)
		if cur > prev {
			t.Fatalf("heuristic increased moving toward goal at x=%d (%f > %f)", x, cur, prev)
		}
		if d := prev - cur; math.Abs(d-1.0) > 1e-9 {
			t.Fatalf("cardinal step toward goal should drop heuristic by 1.0, got %f at x=%d", d, x)
		}
		prev = cur
	}
	if h := g.Heuristic(10, 64, 0); h != 0 {
		t.Fatalf("heuristic at goal must be 0, got %f", h)
	}

	// Diagonal approach: each step toward (5,5) should drop the heuristic by
	// sqrt(2) until aligned.
	gd := NewGoalBlock(5, 64, 5)
	pv := gd.Heuristic(0, 64, 0)
	for s := 1; s <= 5; s++ {
		cur := gd.Heuristic(s, 64, s)
		if d := pv - cur; math.Abs(d-math.Sqrt2) > 1e-9 {
			t.Fatalf("diagonal step should drop heuristic by sqrt(2), got %f at step=%d", d, s)
		}
		pv = cur
	}
}
