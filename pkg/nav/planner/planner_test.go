package planner

import (
	"context"
	"testing"
	"time"

	"github.com/qrjhamron/feast/pkg/nav/goal"
	"github.com/qrjhamron/feast/pkg/nav/move"
	"github.com/qrjhamron/feast/pkg/world"
)

func buildMazeWorld() *world.World {
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)

	// Create solid floor at y=63
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			ch.SetBlock(x, 63, z, world.BlockState{ID: 1})
			ch.SurfaceY[z*world.ChunkWidth+x] = 63
		}
	}

	// Create wall at x=7, z=6..10, y=64..65
	for z := 6; z <= 10; z++ {
		ch.SetBlock(7, 64, z, world.BlockState{ID: 1})
		ch.SetBlock(7, 65, z, world.BlockState{ID: 1})
	}

	w.AddChunk(ch)
	return w
}

func TestPlanner_Success(t *testing.T) {
	InvalidateCache()
	w := buildMazeWorld()
	g := goal.NewGoalBlock(10, 64, 8)

	ctx := context.Background()
	res := Plan(ctx, 5, 64, 8, g, w)
	path := res.Path

	if res.Status != PlanFound {
		t.Fatalf("expected PlanFound, got %s", res.Status)
	}
	if len(path) == 0 {
		t.Fatal("Plan returned empty path for solvable maze")
	}

	// Verify it reached the goal
	currPos := [3]int{5, 64, 8}
	for _, m := range path {
		currPos = m.Destination(currPos)
	}

	if currPos[0] != 10 || currPos[1] != 64 || currPos[2] != 8 {
		t.Errorf("Path did not reach goal. Ended at %v", currPos)
	}
}

func TestPlanner_SnapsStartYToSurface(t *testing.T) {
	InvalidateCache()
	w := buildMazeWorld()
	g := goal.NewGoalBlock(10, 64, 8)

	res := Plan(context.Background(), 5, 227, 8, g, w)
	if res.Status != PlanFound {
		t.Fatalf("expected PlanFound from detached Y start, got %s len=%d", res.Status, len(res.Path))
	}
	if len(res.Path) == 0 {
		t.Fatal("Plan returned empty path from detached Y start")
	}

	pos := [3]int{5, 64, 8}
	for _, m := range res.Path {
		pos = m.Destination(pos)
	}
	if !g.Satisfied(pos[0], pos[1], pos[2]) {
		t.Fatalf("path planned from surface did not reach goal, ended at %v", pos)
	}
}

func TestPlanner_BestSoFar(t *testing.T) {
	InvalidateCache()
	w := buildMazeWorld()
	// Build a complete box around start to make it impossible
	ch := world.NewChunk(0, 0)
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			ch.SetBlock(x, 63, z, world.BlockState{ID: 1})
		}
	}
	// Box around start (5, 64, 8) with tall walls to prevent escape via
	// vertical alternatives.
	for x := 4; x <= 6; x++ {
		for z := 7; z <= 9; z++ {
			if x == 5 && z == 8 {
				continue // air
			}
			for y := 64; y <= 80; y++ {
				ch.SetBlock(x, y, z, world.BlockState{ID: 1})
			}
		}
	}
	w.AddChunk(ch)

	g := goal.NewGoalBlock(10, 64, 8)

	// Short timeout to trigger best-so-far, or just let it exhaust maxNodes
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	res := Plan(ctx, 5, 64, 8, g, w)
	path := res.Path

	// It should return an empty path or partial path (in this case empty since it can't move)
	// But it shouldn't hang or crash.
	if len(path) > 0 {
		t.Errorf("Expected empty path due to box, got length %d", len(path))
	}
	if res.Status != PlanTimeout && res.Status != PlanNoPath && res.Status != PlanPartial {
		t.Fatalf("unexpected status for boxed start: %s", res.Status)
	}
}

func TestPlanner_BestSoFarPartial(t *testing.T) {
	InvalidateCache()
	w := buildMazeWorld()
	// Goal is unreachable but not boxed in, so it will explore until timeout
	// Let's put goal outside the chunk (since missing chunks are solid, it can't reach it)
	g := goal.NewGoalBlock(30, 64, 8)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	res := Plan(ctx, 5, 64, 8, g, w)
	path := res.Path

	// Should explore until timeout, returning some partial path towards x=15
	if len(path) == 0 {
		t.Fatal("Plan returned empty path, expected partial path towards unreachable goal")
	}
	if res.Status != PlanTimeout && res.Status != PlanPartial {
		t.Fatalf("unexpected status for unreachable goal: %s", res.Status)
	}

	// Verify it got closer
	currPos := [3]int{5, 64, 8}
	for _, m := range path {
		currPos = m.Destination(currPos)
	}

	startDist := g.Heuristic(5, 64, 8)
	endDist := g.Heuristic(currPos[0], currPos[1], currPos[2])
	if endDist >= startDist {
		t.Errorf("Partial path did not get closer to goal. Start dist: %v, End dist: %v", startDist, endDist)
	}
}

func TestSmoothPath_CompressesStraightRuns(t *testing.T) {
	start := [3]int{0, 64, 0}
	path := []move.Movement{
		move.MoveWalk{Dx: 1, Dz: 0},
		move.MoveWalk{Dx: 1, Dz: 0},
		move.MoveWalk{Dx: 1, Dz: 0},
		move.MoveWalk{Dx: 1, Dz: 0},
		move.MoveJump{Dx: 1, Dz: 0},
	}

	out := smoothPath(start, path)
	if len(out) != 2 {
		t.Fatalf("expected 2 movements after smoothing, got %d", len(out))
	}
	if _, ok := out[0].(move.MoveWalkLine); !ok {
		t.Fatalf("expected first movement to be MoveWalkLine, got %T", out[0])
	}
	line := out[0].(move.MoveWalkLine)
	if line.Steps != 4 || line.Dx != 1 || line.Dz != 0 {
		t.Fatalf("unexpected line movement: %#v", line)
	}
}

func TestPlanWithExclusions_AvoidsExcludedPosition(t *testing.T) {
	InvalidateCache()
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			ch.SetBlock(x, 63, z, world.BlockState{ID: 1})
		}
	}
	w.AddChunk(ch)

	g := goal.NewGoalBlock(8, 64, 8)
	start := [3]int{5, 64, 8}
	excluded := [3]int{6, 64, 8}

	res := PlanWithExclusions(context.Background(), start[0], start[1], start[2], g, w, [][3]int{excluded})
	path := res.Path
	if len(path) == 0 {
		t.Fatal("expected non-empty path")
	}
	if res.Status != PlanFound {
		t.Fatalf("expected PlanFound, got %s", res.Status)
	}

	pos := start
	for _, m := range path {
		pos = m.Destination(pos)
		if pos == excluded {
			t.Fatalf("path stepped into excluded position %v", excluded)
		}
	}
	if !g.Satisfied(pos[0], pos[1], pos[2]) {
		t.Fatalf("path did not reach goal, ended at %v", pos)
	}
}

func TestPlan_CacheWithinWindow(t *testing.T) {
	w := buildMazeWorld()
	g := goal.NewGoalBlock(10, 64, 8)
	start := [3]int{5, 64, 8}

	InvalidateCache()
	first := Plan(context.Background(), start[0], start[1], start[2], g, w)
	if first.Status != PlanFound || len(first.Path) == 0 {
		t.Fatalf("expected first plan found with path, got %s len=%d", first.Status, len(first.Path))
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	second := Plan(cancelled, start[0], start[1], start[2], g, w)
	if second.Status != first.Status || len(second.Path) != len(first.Path) {
		t.Fatalf("expected cached plan result, got status=%s len=%d", second.Status, len(second.Path))
	}
}

func TestAdaptiveTimeout(t *testing.T) {
	if d := adaptiveTimeout(10); d != 800*time.Millisecond {
		t.Fatalf("distance<20 timeout mismatch: %s", d)
	}
	if d := adaptiveTimeout(20); d != 2*time.Second {
		t.Fatalf("distance 20..100 timeout mismatch: %s", d)
	}
	if d := adaptiveTimeout(150); d != 1000*time.Millisecond {
		t.Fatalf("distance>100 timeout mismatch: %s", d)
	}
}

func TestMaxNodesForDistance(t *testing.T) {
	if got := maxNodesForDistance(50); got != 10000 {
		t.Fatalf("maxNodesForDistance(50)=%d want 10000", got)
	}
	if got := maxNodesForDistance(150); got <= 10000 {
		t.Fatalf("maxNodesForDistance(150)=%d want >10000", got)
	}
}

func TestPlannerCache_OldestEvictedAtMax(t *testing.T) {
	p := NewPlanner()
	p.cacheMax = 2

	k1 := cacheKey{start: [3]int{1, 64, 1}, goal: "g1"}
	k2 := cacheKey{start: [3]int{2, 64, 2}, goal: "g2"}
	k3 := cacheKey{start: [3]int{3, 64, 3}, goal: "g3"}

	p.setCachedPlan(k1, PlanResult{Status: PlanFound})
	p.setCachedPlan(k2, PlanResult{Status: PlanFound})
	p.setCachedPlan(k3, PlanResult{Status: PlanFound})

	if _, ok := p.getCachedPlan(k1); ok {
		t.Fatalf("expected oldest key to be evicted")
	}
	if _, ok := p.getCachedPlan(k2); !ok {
		t.Fatalf("expected second key to remain in cache")
	}
	if _, ok := p.getCachedPlan(k3); !ok {
		t.Fatalf("expected newest key to remain in cache")
	}
}

func TestEntityAwarePathfinding(t *testing.T) {
	w := world.NewWorld()
	ch := world.NewChunk(0, 0)
	for x := 0; x < 16; x++ {
		for z := 0; z < 16; z++ {
			ch.SetBlock(x, 63, z, world.BlockState{ID: 1})
		}
	}
	w.AddChunk(ch)

	store := world.NewEntityStore()
	w.SetEntityStore(store)

	zombie := &world.Entity{
		ID:   99,
		Type: "zombie",
		X:    6.0,
		Y:    64.0,
		Z:    8.0,
		Pose: "standing",
	}
	store.Upsert(zombie)

	g := goal.NewGoalBlock(8, 64, 8)
	start := [3]int{5, 64, 8}

	p1 := NewPlanner()
	p1.SetOptions(PlannerOptions{AvoidEntities: false})
	res1 := p1.Plan(context.Background(), start[0], start[1], start[2], g, w)
	if res1.Status != PlanFound {
		t.Fatalf("AvoidEntities=false: expected path to be found, got %s", res1.Status)
	}
	hasX6 := false
	for _, pos := range getPathIntermediateCoordinates(start, res1.Path) {
		if pos[0] == 6 && pos[2] == 8 {
			hasX6 = true
		}
	}
	if !hasX6 {
		t.Errorf("AvoidEntities=false: expected path to step through (6, 64, 8)")
	}

	p2 := NewPlanner()
	p2.SetOptions(PlannerOptions{AvoidEntities: true})
	res2 := p2.Plan(context.Background(), start[0], start[1], start[2], g, w)
	if res2.Status != PlanFound {
		t.Fatalf("AvoidEntities=true: expected path to be found, got %s", res2.Status)
	}
	for _, pos := range getPathIntermediateCoordinates(start, res2.Path) {
		if pos[0] == 6 && pos[2] == 8 {
			t.Errorf("AvoidEntities=true: path should NOT step through (6, 64, 8)")
		}
	}

	store.Remove(99)
	p2.InvalidateCache()
	res3 := p2.Plan(context.Background(), start[0], start[1], start[2], g, w)
	if res3.Status != PlanFound {
		t.Fatalf("Entity removed: expected path to be found, got %s", res3.Status)
	}
	hasX6AfterRemove := false
	for _, pos := range getPathIntermediateCoordinates(start, res3.Path) {
		if pos[0] == 6 && pos[2] == 8 {
			hasX6AfterRemove = true
		}
	}
	if !hasX6AfterRemove {
		t.Errorf("Entity removed: expected path to step through (6, 64, 8) again")
	}

	floatingZombie := &world.Entity{
		ID:   100,
		Type: "zombie",
		X:    6.0,
		Y:    65.5,
		Z:    8.0,
		Pose: "standing",
	}
	store.Upsert(floatingZombie)

	pStand := NewPlanner()
	pStand.SetOptions(PlannerOptions{AvoidEntities: true, Pose: "standing"})
	resStand := pStand.Plan(context.Background(), start[0], start[1], start[2], g, w)
	for _, pos := range getPathIntermediateCoordinates(start, resStand.Path) {
		if pos[0] == 6 && pos[2] == 8 {
			t.Errorf("Standing pose: should NOT step through (6, 64, 8)")
		}
	}

	pCrawl := NewPlanner()
	pCrawl.SetOptions(PlannerOptions{AvoidEntities: true, Pose: "crawling"})
	resCrawl := pCrawl.Plan(context.Background(), start[0], start[1], start[2], g, w)
	hasX6Crawl := false
	for _, pos := range getPathIntermediateCoordinates(start, resCrawl.Path) {
		if pos[0] == 6 && pos[2] == 8 {
			hasX6Crawl = true
		}
	}
	if !hasX6Crawl {
		t.Errorf("Crawling pose: expected to step through (6, 64, 8)")
	}
}

func getPathIntermediateCoordinates(start [3]int, path []move.Movement) [][3]int {
	var coords [][3]int
	curr := start
	for _, m := range path {
		switch lm := m.(type) {
		case move.MoveWalkLine:
			for i := 0; i < lm.Steps; i++ {
				curr = [3]int{curr[0] + lm.Dx, curr[1], curr[2] + lm.Dz}
				coords = append(coords, curr)
			}
		default:
			curr = m.Destination(curr)
			coords = append(coords, curr)
		}
	}
	return coords
}
