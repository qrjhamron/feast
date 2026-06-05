package goal

import (
	"math"

	"github.com/user/feastgo/pkg/world"
)

// Goal defines the interface for pathfinding goals.
type Goal interface {
	Satisfied(x, y, z int) bool
	Heuristic(x, y, z int) float64
}

// ManhattanEstimator optionally provides a Manhattan-like distance hint from a
// position to a goal for adaptive planner budgets.
type ManhattanEstimator interface {
	ManhattanDistance(x, y, z int) int
}

// GoalXZ is a goal to reach a specific X and Z coordinate, ignoring Y.
type GoalXZ struct {
	X, Z int
}

// NewGoalXZ creates a new GoalXZ.
func NewGoalXZ(x, z int) *GoalXZ {
	return &GoalXZ{X: x, Z: z}
}

// Satisfied returns true if the current X and Z match the goal.
func (g *GoalXZ) Satisfied(x, y, z int) bool {
	return x == g.X && z == g.Z
}

// Heuristic returns a lower bound on the remaining movement cost.
//
// Given movement costs Walk=1, Jump=2, Fall=0.5 per dropped block (plus terrain
// multipliers), the cheapest way to change X/Z by 1 is modeled as 0.5. Use a
// scaled Manhattan distance to remain admissible under diagonal/parkour moves.
func (g *GoalXZ) Heuristic(x, y, z int) float64 {
	return octileHeuristic(x, 0, z, g.X, 0, g.Z)
}

func (g *GoalXZ) ManhattanDistance(x, y, z int) int {
	return absInt(x-g.X) + absInt(z-g.Z)
}

// GoalBlock is a goal to reach a specific X, Y, and Z coordinate.
type GoalBlock struct {
	X, Y, Z int
}

// NewGoalBlock creates a new GoalBlock.
func NewGoalBlock(x, y, z int) *GoalBlock {
	return &GoalBlock{X: x, Y: y, Z: z}
}

// Satisfied returns true if the current X, Y, and Z match the goal.
func (g *GoalBlock) Satisfied(x, y, z int) bool {
	return x == g.X && y == g.Y && z == g.Z
}

// Heuristic returns a lower bound on the remaining movement cost.
//
// This stays admissible by only accounting for the minimum per-step cost to
// change X/Z and ignoring Y (since vertical changes are coupled to horizontal
// moves in the available movement set).
func (g *GoalBlock) Heuristic(x, y, z int) float64 {
	return octileHeuristic(x, y, z, g.X, g.Y, g.Z)
}

func (g *GoalBlock) ManhattanDistance(x, y, z int) int {
	return absInt(x-g.X) + absInt(y-g.Y) + absInt(z-g.Z)
}

// GoalProximity is a goal to be within a certain distance from a target X, Y, Z.
type GoalProximity struct {
	X, Y, Z  int
	Distance float64
}

// NewGoalProximity creates a new GoalProximity.
func NewGoalProximity(x, y, z int, distance float64) *GoalProximity {
	return &GoalProximity{X: x, Y: y, Z: z, Distance: distance}
}

// Satisfied returns true if the distance to the target is less than or equal to the specified distance.
func (g *GoalProximity) Satisfied(x, y, z int) bool {
	return g.distanceTo(x, y, z) <= g.Distance
}

// Heuristic returns a lower bound on the remaining movement cost, or 0 if inside.
func (g *GoalProximity) Heuristic(x, y, z int) float64 {
	h := octileHeuristic(x, y, z, g.X, g.Y, g.Z) - g.Distance
	if h <= 0 {
		return 0
	}
	return h
}

func (g *GoalProximity) ManhattanDistance(x, y, z int) int {
	d := absInt(x-g.X) + absInt(y-g.Y) + absInt(z-g.Z)
	r := int(math.Floor(g.Distance))
	if d <= r {
		return 0
	}
	return d - r
}

func (g *GoalProximity) distanceTo(x, y, z int) float64 {
	dx := float64(x - g.X)
	dy := float64(y - g.Y)
	dz := float64(z - g.Z)
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

type GoalNear struct {
	Entity *world.Entity
	Radius float64
}

func NewGoalNear(entity *world.Entity, radius float64) *GoalNear {
	return &GoalNear{Entity: entity, Radius: radius}
}

func (g *GoalNear) Satisfied(x, y, z int) bool {
	if g == nil || g.Entity == nil {
		return false
	}
	dx := float64(x) - g.Entity.X
	dy := float64(y) - g.Entity.Y
	dz := float64(z) - g.Entity.Z
	return math.Sqrt(dx*dx+dy*dy+dz*dz) <= g.Radius
}

func (g *GoalNear) Heuristic(x, y, z int) float64 {
	if g == nil || g.Entity == nil {
		return math.Inf(1)
	}
	targetX := int(math.Floor(g.Entity.X))
	targetY := int(math.Floor(g.Entity.Y))
	targetZ := int(math.Floor(g.Entity.Z))
	h := octileHeuristic(x, y, z, targetX, targetY, targetZ) - g.Radius
	if h <= 0 {
		return 0
	}
	return h
}

func (g *GoalNear) ManhattanDistance(x, y, z int) int {
	if g == nil || g.Entity == nil {
		return math.MaxInt
	}
	targetX := int(math.Floor(g.Entity.X))
	targetY := int(math.Floor(g.Entity.Y))
	targetZ := int(math.Floor(g.Entity.Z))
	d := absInt(x-targetX) + absInt(y-targetY) + absInt(z-targetZ)
	r := int(math.Floor(g.Radius))
	if d <= r {
		return 0
	}
	return d - r
}

type GoalY struct {
	TargetY int
}

func NewGoalY(targetY int) *GoalY {
	return &GoalY{TargetY: targetY}
}

func (g *GoalY) Satisfied(x, y, z int) bool {
	return y == g.TargetY
}

func (g *GoalY) Heuristic(x, y, z int) float64 {
	return math.Abs(float64(y-g.TargetY)) * 0.5
}

func (g *GoalY) ManhattanDistance(x, y, z int) int {
	return absInt(y - g.TargetY)
}

type CompositeMode int

const (
	CompositeAll CompositeMode = iota
	CompositeAny
)

type GoalComposite struct {
	Mode  CompositeMode
	Goals []Goal
}

func NewGoalComposite(mode CompositeMode, goals ...Goal) *GoalComposite {
	return &GoalComposite{Mode: mode, Goals: goals}
}

func (g *GoalComposite) Satisfied(x, y, z int) bool {
	if len(g.Goals) == 0 {
		return false
	}
	if g.Mode == CompositeAny {
		for _, sub := range g.Goals {
			if sub != nil && sub.Satisfied(x, y, z) {
				return true
			}
		}
		return false
	}
	for _, sub := range g.Goals {
		if sub == nil || !sub.Satisfied(x, y, z) {
			return false
		}
	}
	return true
}

func (g *GoalComposite) Heuristic(x, y, z int) float64 {
	if len(g.Goals) == 0 {
		return math.Inf(1)
	}
	if g.Mode == CompositeAny {
		best := math.Inf(1)
		for _, sub := range g.Goals {
			if sub == nil {
				continue
			}
			h := sub.Heuristic(x, y, z)
			if h < best {
				best = h
			}
		}
		return best
	}
	worst := 0.0
	for _, sub := range g.Goals {
		if sub == nil {
			return math.Inf(1)
		}
		h := sub.Heuristic(x, y, z)
		if h > worst {
			worst = h
		}
	}
	return worst
}

func (g *GoalComposite) ManhattanDistance(x, y, z int) int {
	if len(g.Goals) == 0 {
		return math.MaxInt
	}
	if g.Mode == CompositeAny {
		best := math.MaxInt
		for _, sub := range g.Goals {
			me, ok := sub.(ManhattanEstimator)
			if !ok {
				continue
			}
			d := me.ManhattanDistance(x, y, z)
			if d < best {
				best = d
			}
		}
		return best
	}
	worst := 0
	for _, sub := range g.Goals {
		me, ok := sub.(ManhattanEstimator)
		if !ok {
			return math.MaxInt
		}
		d := me.ManhattanDistance(x, y, z)
		if d > worst {
			worst = d
		}
	}
	return worst
}

func octileHeuristic(x1, y1, z1, x2, y2, z2 int) float64 {
	dx := math.Abs(float64(x1 - x2))
	dz := math.Abs(float64(z1 - z2))
	dy := math.Abs(float64(y1 - y2))
	horizontal := math.Max(dx, dz) + (math.Sqrt2-1.0)*math.Min(dx, dz)
	return horizontal + dy*0.5
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
