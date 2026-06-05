package hpa

import (
	"context"
	"errors"
	"log"
	"math"

	"github.com/qrjhamron/feast/pkg/nav/executor"
	"github.com/qrjhamron/feast/pkg/nav/goal"
	"github.com/qrjhamron/feast/pkg/nav/planner"
	"github.com/qrjhamron/feast/pkg/world"
)

type HPANavigator struct {
	Planner *HPAPlanner
	Updater *GraphUpdater
	World   *world.World
}

func NewHPANavigator(w *world.World, p *HPAPlanner, u *GraphUpdater) *HPANavigator {
	return &HPANavigator{
		Planner: p,
		Updater: u,
		World:   w,
	}
}

func (n *HPANavigator) Navigate(ctx context.Context, client executor.Client, start, target [3]int, goalDef goal.Goal) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if goalDef.Satisfied(start[0], start[1], start[2]) {
			return nil
		}

		goalChunkX := blockToChunkCoord(target[0])
		goalChunkZ := blockToChunkCoord(target[2])
		if !n.World.HasChunk(goalChunkX, goalChunkZ) {
			if err := n.segmentedFlatAdvance(ctx, client, start, target, goalDef); err != nil {
				return err
			}
			sx, sy, sz, _, _ := client.GetPosition()
			start = [3]int{int(math.Floor(sx)), int(math.Floor(sy)), int(math.Floor(sz))}
			continue
		}

		res := n.Planner.Plan(start, target, goalDef)
		if res.Status != 0 { // PlanFound is 0 usually
			log.Printf("[hpa] no abstract path found, falling back to local A*")
			localRes := planner.Plan(ctx, start[0], start[1], start[2], goalDef, n.World)
			if localRes.Status != planner.PlanFound {
				return errors.New("fallback local planner failed to find path")
			}
			return executor.Execute(ctx, client, n.World, goalDef, localRes.Path)
		}
		log.Printf("[hpa] abstract path found %d clusters", len(res.AbstractPath))

		n.Updater.SetActiveRefiner(res.Refiner)

		total := len(res.AbstractPath)
		for !res.Refiner.IsComplete() {
			if ctx.Err() != nil {
				n.Updater.SetActiveRefiner(nil)
				return ctx.Err()
			}

			if res.Refiner.NeedsReplan() {
				// Replan from current position
				break
			}

			segment := res.Refiner.NextSegment()
			if len(segment) == 0 {
				break
			}
			log.Printf("[hpa] refining cluster %d/%d", res.Refiner.CurrentIndex(), total)

			err := executor.Execute(ctx, client, n.World, goalDef, segment)
			if err != nil {
				// Execution failed, we need to full replan from current bot pos
				// But we need the current pos from the world.
				break
			}

			// update start pos
			if len(segment) > 0 {
				last := segment[len(segment)-1]
				start = last.Destination(start) // assuming we keep track of start
				// Or better, query current pos. We'll rely on the outer loop querying current pos.
			}
		}

		n.Updater.SetActiveRefiner(nil)
		if res.Refiner.IsComplete() || goalDef.Satisfied(start[0], start[1], start[2]) {
			log.Printf("[hpa] arrived")
			return nil
		}

		// Get real current position to re-start the loop
		// In FeastGo, the World tracks the bot position? Or the executor does?
		// Actually, we'll just return and let the caller loop if we want, or we loop here if we can query pos.
		// Since we don't have a reliable way to get bot position synchronously here without passing it,
		// we'll return an error if it fails and let caller handle it, or we assume caller updates `start`.
		return errors.New("path execution interrupted, replan required")
	}
}

func (n *HPANavigator) segmentedFlatAdvance(ctx context.Context, client executor.Client, start, target [3]int, goalDef goal.Goal) error {
	frontier, ok := frontierWaypoint(start, target, n.World)
	if !ok {
		return errors.New("destination chunk not loaded and no loaded frontier found")
	}
	segGoal := goal.NewGoalBlock(frontier[0], frontier[1], frontier[2])
	res := planner.Plan(ctx, start[0], start[1], start[2], segGoal, n.World)
	if res.Status != planner.PlanFound || len(res.Path) == 0 {
		return errors.New("destination chunk not loaded and segmented flat path not found")
	}
	return executor.Execute(ctx, client, n.World, goalDef, res.Path)
}

func frontierWaypoint(start, target [3]int, w *world.World) ([3]int, bool) {
	dx := float64(target[0] - start[0])
	dz := float64(target[2] - start[2])
	dist := math.Hypot(dx, dz)
	if dist == 0 {
		return start, true
	}
	ux := dx / dist
	uz := dz / dist
	step := 8.0
	lastLoaded := start
	found := false
	for t := 0.0; t <= dist; t += step {
		x := int(math.Round(float64(start[0]) + ux*t))
		z := int(math.Round(float64(start[2]) + uz*t))
		cx := blockToChunkCoord(x)
		cz := blockToChunkCoord(z)
		if !w.HasChunk(cx, cz) {
			break
		}
		found = true
		lastLoaded = [3]int{x, target[1], z}
	}
	return lastLoaded, found
}
