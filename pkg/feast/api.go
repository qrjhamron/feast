// Package feast provides a Go-native offline-mode Minecraft Java Edition
// Protocol 765 bot client.
//
// FeastGo targets Minecraft Java Edition 1.20.4 in offline mode only. It is
// alpha/experimental and does not support online-mode authentication,
// encryption, or multi-version protocols.
//
// The beginner path is [Connect], [Client.WaitUntilReady], [Client.State],
// typed event hooks, waiter helpers, world queries, navigation, chat, and
// block/container actions. Low-level packet, raw event-bus, HPA*,
// movement-authority, and smoke-planning APIs are marked Advanced in their
// GoDoc.
//
// # Quick start
//
//	bot, err := feast.Connect(ctx, feast.Options{
//	    Host:     "127.0.0.1",
//	    Port:     "25565",
//	    Username: "FeastGoBot",
//	})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer bot.Disconnect()
//
//	bot.OnChat(func(e feast.ChatEvent) {
//	    fmt.Println("chat:", e.Message)
//	})
package feast

import (
	"context"
	"fmt"
	"math"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/qrjhamron/feast/pkg/nav/goal"
	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/registry"
	"github.com/qrjhamron/feast/pkg/state"
	"github.com/qrjhamron/feast/pkg/world"
)

// ─── Public event types ───────────────────────────────────────────────────────

// ChatEvent is emitted when a player-chat message is received.
type ChatEvent struct {
	// Sender is the display name of the message author (may be empty for system messages).
	Sender string
	// Message is the plain-text body of the chat message.
	Message string
}

// HealthEvent is emitted when the bot's health, food, or saturation changes.
type HealthEvent struct {
	// Health is the current health value (0–20).
	Health float32
	// Food is the current food level (0–20).
	Food int32
	// Saturation is the current food saturation.
	Saturation float32
}

// PositionEvent is emitted when the server synchronises the bot position.
type PositionEvent struct {
	X, Y, Z float64
	Yaw     float32
	Pitch   float32
}

// BlockUpdateEvent is emitted when one or more blocks change state.
type BlockUpdateEvent struct {
	X, Y, Z int32
	StateID int32
}

// EntityEvent carries a snapshot of one tracked entity and the kind of change.
type EntityEvent struct {
	// Entity is a snapshot of the entity at the time of the event.
	Entity *world.Entity
	// Kind is "spawn", "move", or "remove".
	Kind string
}

// Direction is a block face used for placement operations.
// It aliases protocol.Direction so callers need not import pkg/protocol.
type Direction = protocol.Direction

// Re-export named face constants so library users only need to import pkg/feast.
const (
	// FaceDown places against the underside of the block above.
	FaceDown = protocol.DirectionDown
	// FaceUp places on top of the support block (most common).
	FaceUp = protocol.DirectionUp
	// FaceNorth places against the north face.
	FaceNorth = protocol.DirectionNorth
	// FaceSouth places against the south face.
	FaceSouth = protocol.DirectionSouth
	// FaceWest places against the west face.
	FaceWest = protocol.DirectionWest
	// FaceEast places against the east face.
	FaceEast = protocol.DirectionEast
)

// BlockPos is an integer block coordinate.
// It aliases protocol.BlockPos so library users need not import pkg/protocol.
type BlockPos = protocol.BlockPos

// Vec3 is a floating-point world-space coordinate.
// It aliases world.Vec3 so library users need not import pkg/world.
type Vec3 = world.Vec3

// ─── Top-level constructors ───────────────────────────────────────────────────

// Connect creates a new client with the given options, connects to the server,
// and waits until it enters the Play state. It returns an error if the
// connection or login sequence fails.
//
// The context is used for the initial network dial. The running client
// lifecycle is managed separately via [Client.Disconnect].
func Connect(ctx context.Context, opts Options) (*Client, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c := NewClient(opts)
	var dialer net.Dialer
	c.dialFunc = func(network, address string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, address)
	}
	if err := c.Connect(); err != nil {
		return nil, err
	}
	return c, nil
}

// ─── State accessors ──────────────────────────────────────────────────────────

// Position returns the bot's current world-space coordinates.
func (c *Client) Position() world.Vec3 {
	st := c.PlayerState()
	return world.Vec3{X: st.X, Y: st.Y, Z: st.Z}
}

// Health returns the bot's current health (0–20).
func (c *Client) Health() float32 {
	return c.PlayerState().Health
}

// Food returns the bot's current food level (0–20).
func (c *Client) Food() int32 {
	return c.PlayerState().Food
}

// ─── World helpers ────────────────────────────────────────────────────────────

// FindNearestBlock searches the loaded world for the nearest block matching name.
func (c *Client) FindNearestBlock(name string, radius int) (world.BlockHit, bool) {
	return c.world.FindNearestBlock(c.Position(), name, radius)
}

// FindBlocks searches the loaded world for up to count nearest blocks matching name.
// Results are sorted by distance.
func (c *Client) FindBlocks(name string, count, radius int) []world.BlockHit {
	return c.world.FindBlocks(c.Position(), name, count, radius)
}

// FindPlaceTargetNear searches loaded blocks around origin for a replaceable
// target block with a solid neighboring support block.
//
// It returns the target block to place into, the face to pass to
// [Client.PlaceBlockSurvival], and ok=false if no loaded safe target is found.
// blockName is reserved for future block-specific placement rules; current
// placement safety is based on replaceability and support solidity.
func FindPlaceTargetNear(w *world.World, origin Vec3, blockName string, radius int) (target BlockPos, face Direction, ok bool) {
	if w == nil || radius < 0 {
		return BlockPos{}, FaceUp, false
	}
	ox := int(math.Floor(origin.X))
	oy := int(math.Floor(origin.Y))
	oz := int(math.Floor(origin.Z))
	bestDist := math.Inf(1)
	for dx := -radius; dx <= radius; dx++ {
		for dy := -radius; dy <= radius; dy++ {
			for dz := -radius; dz <= radius; dz++ {
				cand := BlockPos{X: int32(ox + dx), Y: int32(oy + dy), Z: int32(oz + dz)}
				if !w.IsBlockLoaded(world.BlockPos(cand)) || !w.IsReplaceable(world.BlockPos(cand)) {
					continue
				}
				for _, support := range []struct {
					face Direction
					pos  BlockPos
				}{
					{FaceUp, BlockPos{X: cand.X, Y: cand.Y - 1, Z: cand.Z}},
					{FaceDown, BlockPos{X: cand.X, Y: cand.Y + 1, Z: cand.Z}},
					{FaceNorth, BlockPos{X: cand.X, Y: cand.Y, Z: cand.Z + 1}},
					{FaceSouth, BlockPos{X: cand.X, Y: cand.Y, Z: cand.Z - 1}},
					{FaceWest, BlockPos{X: cand.X + 1, Y: cand.Y, Z: cand.Z}},
					{FaceEast, BlockPos{X: cand.X - 1, Y: cand.Y, Z: cand.Z}},
				} {
					if !w.IsBlockLoaded(world.BlockPos(support.pos)) || !w.IsSolid(world.BlockPos(support.pos)) {
						continue
					}
					dist := math.Sqrt(float64(dx*dx + dy*dy + dz*dz))
					if dist < bestDist {
						bestDist = dist
						target = cand
						face = support.face
						ok = true
					}
				}
			}
		}
	}
	return target, face, ok
}

// ─── Navigation ──────────────────────────────────────────────────────────────

// NavigateResult contains details about a completed navigation attempt.
type NavigateResult struct {
	// Reached is true if the goal was satisfied.
	Reached bool
	// FinalPosition is the bot's position after navigation.
	FinalPosition Vec3
	// DistanceTraveled is the total distance moved.
	DistanceTraveled float64
	// PacketsSent is the number of position packets sent.
	PacketsSent int
	// Duration is how long navigation took.
	Duration time.Duration
}

// NavigateTo wraps NavigateWithResult and discards the result.
func (c *Client) NavigateTo(ctx context.Context, g goal.Goal, opts ...MovementOptions) error {
	_, err := c.NavigateWithResult(ctx, g, opts...)
	return err
}

// NavigateWithResult navigates the bot to the given goal and blocks until the
// goal is satisfied, navigation fails, or ctx is cancelled.
//
// Use [goal.NewGoalBlock], [goal.NewGoalXZ], [goal.NewGoalProximity], or
// [goal.NewGoalNear] to create a goal. The zero-value [MovementOptions] uses
// the client's current movement profile.
func (c *Client) NavigateWithResult(ctx context.Context, g goal.Goal, opts ...MovementOptions) (NavigateResult, error) {
	startTime := time.Now()
	profile := c.MovementProfile()
	opt := MovementOptions{
		Profile: profile,
	}
	if len(opts) > 0 {
		opt = opts[0]
		if opt.Profile == "" {
			opt.Profile = profile
		}
	}
	c.stateMu.Lock()
	c.activeOptions = opt
	c.stateMu.Unlock()

	pos := c.Position()
	x := int(math.Floor(pos.X))
	z := int(math.Floor(pos.Z))

	// Resolve target coordinates from the goal's heuristic centre if possible.
	// Fall back to current position as a no-op sentinel.
	targetX, targetY, targetZ := x, int(math.Floor(pos.Y)), z
	if gb, ok := g.(*goal.GoalBlock); ok {
		targetX, targetY, targetZ = gb.X, gb.Y, gb.Z
	} else if gz, ok := g.(*goal.GoalXZ); ok {
		targetX, targetZ = gz.X, gz.Z
	} else if gp, ok := g.(*goal.GoalProximity); ok {
		targetX, targetY, targetZ = gp.X, gp.Y, gp.Z
	}

	done := make(chan error, 1)
	arrID, _ := c.bus.On("nav_arrived", func(e state.Event) {
		select {
		case done <- nil:
		default:
		}
	})
	failID, _ := c.bus.On("nav_failed", func(e state.Event) {
		reason := "navigation failed"
		if ev, ok := e.(state.NavFailedEvent); ok && ev.Reason != "" {
			reason = ev.Reason
		}
		select {
		case done <- fmt.Errorf("nav: %s", reason):
		default:
		}
	})
	defer c.bus.Off(arrID)
	defer c.bus.Off(failID)

	if err := c.navigateTo(targetX, targetY, targetZ); err != nil {
		return NavigateResult{}, err
	}

	select {
	case <-ctx.Done():
		c.StopNavigation()
		stats := c.LastMovementStats()
		return NavigateResult{
			Reached:          false,
			FinalPosition:    c.Position(),
			DistanceTraveled: stats.DistanceTraveled,
			PacketsSent:      stats.PacketsSent,
			Duration:         time.Since(startTime),
		}, ctx.Err()
	case err := <-done:
		stats := c.LastMovementStats()
		return NavigateResult{
			Reached:          err == nil,
			FinalPosition:    c.Position(),
			DistanceTraveled: stats.DistanceTraveled,
			PacketsSent:      stats.PacketsSent,
			Duration:         time.Since(startTime),
		}, err
	}
}

// navigateTo is the internal coordinate-based navigation entry point.
func (c *Client) navigateTo(x, y, z int) error {
	return c.NavigateTo2(x, y, z)
}

// ─── Block interaction ────────────────────────────────────────────────────────

// BreakOptions configures one block-breaking action.
//
// The zero value is safe: FeastGo breaks with the currently held item, applies
// survival timing, refuses unloaded chunks, refuses unsafe targets under the
// bot, and waits for a server block update.
type BreakOptions struct {
	// AutoTool enables automatic selection of the best tool from the inventory.
	AutoTool bool
	// Creative skips block-hardness delay for creative-mode or test harness use.
	Creative bool
}

// BreakResult contains details about a successful block break action.
//
// On failure, [Client.BreakBlockWithResult] returns the zero result and the
// original error, which can be checked with [errors.Is] for sentinel errors.
type BreakResult struct {
	// Position is the block that was broken.
	Position BlockPos
	// OldBlock is the block name before breaking.
	OldBlock string
	// NewBlock is the block name after breaking (typically "air").
	NewBlock string
	// ToolUsed is the name of the tool used, or "none".
	ToolUsed string
	// Duration is how long the break action took.
	Duration time.Duration
}

// BreakBlock wraps BreakBlockWithResult and discards the result.
func (c *Client) BreakBlock(ctx context.Context, pos BlockPos, opts ...BreakOptions) error {
	_, err := c.BreakBlockWithResult(ctx, pos, opts...)
	return err
}

// BreakBlockWithResult sends dig packets for pos and waits for the server to
// confirm the break with a block update.
func (c *Client) BreakBlockWithResult(ctx context.Context, pos BlockPos, opts ...BreakOptions) (BreakResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var autoTool bool
	var creative bool
	for _, opt := range opts {
		if opt.AutoTool {
			autoTool = true
		}
		if opt.Creative {
			creative = true
		}
	}

	if err := c.requireCoreActionReady(); err != nil {
		return BreakResult{}, err
	}
	if err := c.world.RequireBlockLoaded(world.BlockPos(pos)); err != nil {
		return BreakResult{}, err
	}

	playerSt := c.PlayerState()
	bx, by, bz := playerSt.X, playerSt.Y, playerSt.Z
	eyeX, eyeY, eyeZ := bx, by+1.62, bz
	tcX, tcY, tcZ := float64(pos.X)+0.5, float64(pos.Y)+0.5, float64(pos.Z)+0.5
	dx := tcX - eyeX
	dy := tcY - eyeY
	dz := tcZ - eyeZ
	distance := math.Sqrt(dx*dx + dy*dy + dz*dz)
	withinReach := distance <= 4.5

	underFeet := pos.X == int32(math.Floor(bx)) && pos.Z == int32(math.Floor(bz)) && pos.Y == int32(math.Floor(by))-1

	minX := bx - 0.3
	maxX := bx + 0.3
	minZ := bz - 0.3
	maxZ := bz + 0.3
	blockMinX := float64(pos.X)
	blockMaxX := float64(pos.X) + 1.0
	blockMinZ := float64(pos.Z)
	blockMaxZ := float64(pos.Z) + 1.0

	inFootprint := pos.Y == int32(math.Floor(by))-1 &&
		blockMinX <= maxX && blockMaxX >= minX &&
		blockMinZ <= maxZ && blockMaxZ >= minZ

	if underFeet || inFootprint {
		return BreakResult{}, ErrBreakTargetUnsafe
	}

	blockState, err := c.world.GetBlock(int(pos.X), int(pos.Y), int(pos.Z))
	if err != nil {
		return BreakResult{}, fmt.Errorf("failed to get block: %w", err)
	}
	if c.world.IsReplaceable(world.BlockPos(pos)) {
		return BreakResult{}, ErrBlockAir
	}
	nameNorm := strings.TrimPrefix(strings.ToLower(blockState.Name), "minecraft:")
	if nameNorm == "bedrock" || nameNorm == "barrier" {
		return BreakResult{}, fmt.Errorf("%w: %s", ErrBlockUnbreakable, blockState.Name)
	}

	blockName := blockState.Name
	c.debugActionf("break", "auto_tool=%v", autoTool)
	c.debugActionf("break", "target_block=%s", blockName)

	selectedToolName := "none"
	selectedSlot := -1

	c.inventoryMu.RLock()
	currentSlot := c.inventory.SelectedHotbarSlot
	c.inventoryMu.RUnlock()

	if autoTool {
		preferredKind := registry.PreferredToolForBlock(blockName)
		if preferredKind != registry.ToolNone {
			c.inventoryMu.RLock()
			for i := 0; i < 9; i++ {
				invSlot := 36 + i
				st, present := c.inventory.Slots[invSlot]
				if present && st.Present {
					kind := registry.ToolKindFromItem(st.Name)
					if kind == preferredKind {
						selectedToolName = st.Name
						selectedSlot = i
						break
					}
				}
			}
			c.inventoryMu.RUnlock()
		}

		if selectedSlot != -1 {
			if selectedSlot != currentSlot {
				if err := c.SelectHotbarSlot(ctx, selectedSlot); err != nil {
					return BreakResult{}, fmt.Errorf("failed to select tool slot: %w", err)
				}
			}
			c.debugActionf("break", "selected_tool=%s", selectedToolName)
			c.debugActionf("break", "selected_slot=%d", selectedSlot)
		} else {
			heldItem, hasHeld := c.HeldItem()
			if hasHeld {
				selectedToolName = heldItem.Name
			}
			c.debugActionf("break", "selected_tool=%s", selectedToolName)
			c.debugActionf("break", "selected_slot=%d", currentSlot)
		}
	} else {
		heldItem, hasHeld := c.HeldItem()
		if hasHeld {
			selectedToolName = heldItem.Name
		}
	}

	var delay time.Duration
	if !creative {
		delay = c.estimateBreakDelay(blockName, selectedToolName)
	}

	c.debugActionf("break-debug", "pos=%.2f,%.2f,%.2f", bx, by, bz)
	c.debugActionf("break-debug", "eye=%.2f,%.2f,%.2f", eyeX, eyeY, eyeZ)
	c.debugActionf("break-debug", "target_center=%.2f,%.2f,%.2f", tcX, tcY, tcZ)
	c.debugActionf("break-debug", "distance=%.2f", distance)
	c.debugActionf("break-debug", "within_reach=%v", withinReach)
	c.debugActionf("break-debug", "selected_tool=%s", selectedToolName)
	c.debugActionf("break-debug", "estimated_break_delay=%s", delay)

	if !withinReach {
		return BreakResult{}, fmt.Errorf("%w (distance %.2f)", ErrBreakOutOfReach, distance)
	}

	ch := make(chan struct{}, 1)
	blockHandlerID, _ := c.bus.On("block_update", func(e state.Event) {
		if ev, ok := e.(state.BlockUpdateEvent); ok {
			if int(ev.X) == int(pos.X) && int(ev.Y) == int(pos.Y) && int(ev.Z) == int(pos.Z) {
				select {
				case ch <- struct{}{}:
				default:
				}
			}
		}
	})
	defer c.bus.Off(blockHandlerID)

	sectionHandlerID, _ := c.bus.On("section_blocks_update", func(e state.Event) {
		if ev, ok := e.(state.SectionBlocksUpdateEvent); ok {
			for _, u := range ev.Updates {
				if int(u.X) == int(pos.X) && int(u.Y) == int(pos.Y) && int(u.Z) == int(pos.Z) {
					select {
					case ch <- struct{}{}:
					default:
					}
				}
			}
		}
	})
	defer c.bus.Off(sectionHandlerID)

	face := c.determineDigFace(world.Vec3{X: bx, Y: by, Z: bz}, pos)
	c.debugActionf("break-debug", "face=%d", face)
	seq := int32(atomic.AddInt32(&placementSequence, 1))
	c.debugActionf("break", "sequence_id=%d", seq)

	// Look-at-target step
	yaw, pitch := CalculateLookRotation(eyeX, eyeY, eyeZ, tcX, tcY, tcZ)
	lookPkt := &protocol.PlayServerboundSetPlayerPositionAndRotationPacket{
		X:        playerSt.X,
		Y:        playerSt.Y,
		Z:        playerSt.Z,
		Yaw:      yaw,
		Pitch:    pitch,
		OnGround: playerSt.OnGround,
	}
	c.debugActionf("look", "eye=%f,%f,%f", eyeX, eyeY, eyeZ)
	c.debugActionf("look", "target=%f,%f,%f", tcX, tcY, tcZ)
	c.debugActionf("look", "yaw=%f", yaw)
	c.debugActionf("look", "pitch=%f", pitch)
	if err := c.WritePacket(lookPkt); err != nil {
		return BreakResult{}, fmt.Errorf("failed to look at target: %w", err)
	}
	c.debugActionf("look", "sent=true")

	start := &protocol.PlayServerboundPlayerActionPacket{
		Status:   protocol.PlayerActionStartDigging,
		Position: pos,
		Face:     face,
		Sequence: seq,
	}
	if err := c.WritePacket(start); err != nil {
		c.debugActionf("break-debug", "start_sent=false")
		return BreakResult{}, fmt.Errorf("break block start: %w", err)
	}
	c.debugActionf("break-debug", "start_sent=true")

	if !creative && delay > 0 {
		select {
		case <-ctx.Done():
			return BreakResult{}, ctx.Err()
		case <-time.After(delay):
		}
	}

	finish := &protocol.PlayServerboundPlayerActionPacket{
		Status:   protocol.PlayerActionFinishDigging,
		Position: pos,
		Face:     face,
		Sequence: seq,
	}
	if err := c.WritePacket(finish); err != nil {
		c.debugActionf("break-debug", "finish_sent=false")
		return BreakResult{}, fmt.Errorf("break block finish: %w", err)
	}
	c.debugActionf("break-debug", "finish_sent=true")

	var finalStateName string = "unknown"

	startTime := time.Now()
	select {
	case <-ctx.Done():
		c.debugActionf("break-debug", "update_seen=false")
		c.debugActionf("break-debug", "final_state=unknown")
		return BreakResult{}, ctx.Err()
	case <-ch:
		newState, err := c.world.GetBlock(int(pos.X), int(pos.Y), int(pos.Z))
		if err != nil {
			c.debugActionf("break-debug", "update_seen=true")
			c.debugActionf("break-debug", "final_state=unknown")
			return BreakResult{}, err
		}
		finalStateName = newState.Name
		c.debugActionf("break-debug", "update_seen=true")
		c.debugActionf("break-debug", "final_state=%s", finalStateName)

		if !c.world.IsReplaceable(world.BlockPos(pos)) || newState.Name == blockName {
			c.debugActionf("break", "rollback detected: expected air/replaceable, got %s", newState.Name)
			return BreakResult{}, ErrBreakRolledBack
		}
		c.debugActionf("break", "result=PASS")
		return BreakResult{
			Position: pos,
			OldBlock: blockName,
			NewBlock: finalStateName,
			ToolUsed: selectedToolName,
			Duration: time.Since(startTime) + delay,
		}, nil
	case <-time.After(4 * time.Second):
		c.debugActionf("break-debug", "update_seen=false")
		c.debugActionf("break-debug", "final_state=unknown")
		return BreakResult{}, ErrBlockUpdateTimeout
	}
}

// CalculateLookRotation calculates the Minecraft yaw and pitch (in degrees) from eyePos to targetCenter.
func CalculateLookRotation(eyeX, eyeY, eyeZ, tcX, tcY, tcZ float64) (yaw float32, pitch float32) {
	dx := tcX - eyeX
	dy := tcY - eyeY
	dz := tcZ - eyeZ

	yawRad := -math.Atan2(dx, dz)
	yaw = float32(yawRad * 180 / math.Pi)

	horizontalDistance := math.Sqrt(dx*dx + dz*dz)
	pitchRad := -math.Atan2(dy, horizontalDistance)
	pitch = float32(pitchRad * 180 / math.Pi)

	return yaw, pitch
}

func (c *Client) determineDigFace(botPos world.Vec3, target BlockPos) byte {
	dx := float64(target.X) + 0.5 - botPos.X
	dy := float64(target.Y) + 0.5 - (botPos.Y + 1.62)
	dz := float64(target.Z) + 0.5 - botPos.Z

	if dy < -1.0 {
		return 1 // BlockFaceTop
	}
	if dy > 1.0 {
		return 0 // BlockFaceBottom
	}
	if math.Abs(dx) > math.Abs(dz) {
		if dx > 0 {
			return 4 // BlockFaceWest
		}
		return 5 // BlockFaceEast
	} else {
		if dz > 0 {
			return 2 // BlockFaceNorth
		}
		return 3 // BlockFaceSouth
	}
}

func (c *Client) estimateBreakDelay(blockName, toolName string) time.Duration {
	blockName = strings.TrimSpace(strings.ToLower(blockName))
	blockName = strings.TrimPrefix(blockName, "minecraft:")

	toolName = strings.TrimSpace(strings.ToLower(toolName))
	toolName = strings.TrimPrefix(toolName, "minecraft:")

	if blockName == "air" || blockName == "cave_air" || blockName == "void_air" {
		return 0
	}

	isSoft := strings.Contains(blockName, "dirt") || strings.Contains(blockName, "grass") ||
		strings.Contains(blockName, "sand") || strings.Contains(blockName, "gravel") ||
		strings.Contains(blockName, "clay") || strings.Contains(blockName, "snow")
	isStone := strings.Contains(blockName, "stone") || strings.Contains(blockName, "cobblestone") ||
		strings.Contains(blockName, "deepslate") || strings.Contains(blockName, "ore") ||
		strings.Contains(blockName, "obsidian") || strings.Contains(blockName, "andesite") ||
		strings.Contains(blockName, "diorite") || strings.Contains(blockName, "granite")

	isShovel := strings.Contains(toolName, "shovel")
	isPickaxe := strings.Contains(toolName, "pickaxe")

	if isSoft {
		if isShovel {
			return 200 * time.Millisecond
		}
		return 900 * time.Millisecond
	}

	if isStone {
		if isPickaxe {
			return 800 * time.Millisecond
		}
		return 7500 * time.Millisecond
	}

	return 500 * time.Millisecond
}

// PlaceBlockCreative places a block in creative mode at target using the given
// blockName (e.g. "stone"). It sets the creative-mode inventory slot, selects
// hotbar slot 0, then sends a UseItemOn packet.
func (c *Client) PlaceBlockCreative(ctx context.Context, target BlockPos, face Direction, blockName string) error {
	// Resolve item ID from block name.
	var itemID int32 = 1 // default: stone
	for id := int32(0); id < 1000; id++ {
		if name, ok := ItemNameFromID(id); ok && name == blockName {
			itemID = id
			break
		}
	}
	plan, err := c.PrepareCreativeSmokePlacement(target)
	if err != nil {
		return err
	}
	plan.HeldItemID = itemID
	plan.HeldItemName = blockName
	return c.ExecuteCreativeSmokePlacement(plan)
}

// PlaceBlockSurvival places a block from the hotbar in survival mode.
//
// target is the replaceable block to place into; face is the face of the
// neighboring support block the placement is against. The action refuses
// unloaded targets/supports, non-replaceable targets, non-solid support,
// out-of-reach targets, player/entity overlap, context cancellation, timeout,
// and server rollback.
func (c *Client) PlaceBlockSurvival(ctx context.Context, target BlockPos, face Direction) error {
	return c.PlaceBlockSurvivalInternal(ctx, target, face)
}

// ─── Chat ─────────────────────────────────────────────────────────────────────

// Chat sends a chat message. It is an alias for [Client.SendChat].
func (c *Client) Chat(message string) error {
	return c.SendChat(message)
}

// ─── Typed event hooks ────────────────────────────────────────────────────────

// OnReady registers fn to be called once the client enters the Play state and
// has received at least one position sync. The callback fires at most once per
// connection.
func (c *Client) OnReady(fn func()) func() {
	var fired bool
	id, _ := c.bus.On("position", func(e state.Event) {
		if fired {
			return
		}
		if _, ok := e.(state.PositionEvent); ok {
			fired = true
			fn()
		}
	})
	return func() { c.bus.Off(id) }
}

// OnChat registers fn to be called for every player-chat message.
func (c *Client) OnChat(fn func(ChatEvent)) func() {
	id, _ := c.bus.On("chat", func(e state.Event) {
		if ev, ok := e.(state.ChatEvent); ok {
			fn(ChatEvent{Sender: ev.Sender, Message: ev.Message})
		}
	})
	return func() { c.bus.Off(id) }
}

// OnSystemChat registers fn to be called for system/server messages.
// System messages use the same ChatEvent type with Sender set to "".
func (c *Client) OnSystemChat(fn func(ChatEvent)) func() {
	id, _ := c.bus.On("system_chat", func(e state.Event) {
		if ev, ok := e.(state.ChatEvent); ok {
			fn(ChatEvent{Sender: ev.Sender, Message: ev.Message})
		}
	})
	return func() { c.bus.Off(id) }
}

// OnHealth registers fn for health/food/saturation change events.
func (c *Client) OnHealth(fn func(HealthEvent)) func() {
	id, _ := c.bus.On("health", func(e state.Event) {
		if ev, ok := e.(state.HealthEvent); ok {
			fn(HealthEvent{Health: ev.Health, Food: ev.Food, Saturation: ev.Saturation})
		}
	})
	return func() { c.bus.Off(id) }
}

// OnPosition registers fn for server position-sync events.
func (c *Client) OnPosition(fn func(PositionEvent)) func() {
	id, _ := c.bus.On("position", func(e state.Event) {
		if ev, ok := e.(state.PositionEvent); ok {
			fn(PositionEvent{X: ev.X, Y: ev.Y, Z: ev.Z, Yaw: ev.Yaw, Pitch: ev.Pitch})
		}
	})
	return func() { c.bus.Off(id) }
}

// OnPositionSync registers fn for server position-sync events.
//
// It is a discoverability-friendly alias for [Client.OnPosition].
func (c *Client) OnPositionSync(fn func(PositionEvent)) func() {
	return c.OnPosition(fn)
}

// OnBlockUpdate registers fn for block-state-change events.
func (c *Client) OnBlockUpdate(fn func(BlockUpdateEvent)) func() {
	id, _ := c.bus.On("block_update", func(e state.Event) {
		if ev, ok := e.(state.BlockUpdateEvent); ok {
			fn(BlockUpdateEvent{X: ev.X, Y: ev.Y, Z: ev.Z, StateID: ev.StateID})
		}
	})
	return func() { c.bus.Off(id) }
}

// OnEntitySpawn registers fn for entity spawn events.
func (c *Client) OnEntitySpawn(fn func(EntityEvent)) func() {
	id, _ := c.bus.On("entity_spawn", func(e state.Event) {
		if ev, ok := e.(state.EntitySpawnEvent); ok {
			ent := &world.Entity{
				ID:   ev.EntityID,
				UUID: ev.UUID,
				Type: world.EntityTypeFromID(ev.Type),
				X:    ev.X,
				Y:    ev.Y,
				Z:    ev.Z,
			}
			fn(EntityEvent{Entity: ent, Kind: "spawn"})
		}
	})
	return func() { c.bus.Off(id) }
}

// OnEntityMove registers fn for entity position-delta events.
func (c *Client) OnEntityMove(fn func(EntityEvent)) func() {
	id, _ := c.bus.On("entity_move_delta", func(e state.Event) {
		if ev, ok := e.(state.EntityMoveDeltaEvent); ok {
			if ent, exists := c.entities.Get(ev.EntityID); exists {
				fn(EntityEvent{Entity: ent, Kind: "move"})
			}
		}
	})
	return func() { c.bus.Off(id) }
}

// OnEntityRemove registers fn for entity removal events.
func (c *Client) OnEntityRemove(fn func(EntityEvent)) func() {
	id, _ := c.bus.On("entity_remove", func(e state.Event) {
		if ev, ok := e.(state.EntityRemoveEvent); ok {
			fn(EntityEvent{Entity: &world.Entity{ID: ev.EntityID}, Kind: "remove"})
		}
	})
	return func() { c.bus.Off(id) }
}

// OnRespawn registers fn for respawn events.
func (c *Client) OnRespawn(fn func(state.RespawnEvent)) func() {
	id, _ := c.bus.On("respawn", func(e state.Event) {
		if ev, ok := e.(state.RespawnEvent); ok {
			fn(ev)
		}
	})
	return func() { c.bus.Off(id) }
}

// OnError registers fn for non-fatal runtime errors.
func (c *Client) OnError(fn func(error)) func() {
	id, _ := c.bus.On("error", func(e state.Event) {
		if ev, ok := e.(state.ErrorEvent); ok && ev.Error != nil {
			fn(ev.Error)
		}
	})
	return func() { c.bus.Off(id) }
}

// OnDisconnect registers fn for disconnect events (both clean and unclean).
// The error is nil for clean disconnects.
func (c *Client) OnDisconnect(fn func(error)) func() {
	id, _ := c.bus.On("disconnect", func(e state.Event) {
		if ev, ok := e.(state.DisconnectEvent); ok {
			if ev.Clean {
				fn(nil)
			} else {
				fn(fmt.Errorf("disconnected: %s", ev.Reason))
			}
		}
	})
	return func() { c.bus.Off(id) }
}

// WaitUntilReady blocks until the bot receives its first server position sync,
// or until the context is cancelled.
//
// The internal readiness handler is always unsubscribed before returning, so
// repeated calls do not leak event handlers.
func (c *Client) WaitUntilReady(ctx context.Context) error {
	ready := make(chan struct{}, 1)
	unsub := c.OnReady(func() {
		select {
		case ready <- struct{}{}:
		default:
		}
	})
	defer unsub()
	// Already synced?
	c.stateMu.RLock()
	synced := c.positionSynced
	c.stateMu.RUnlock()
	if synced {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ready:
		return nil
	}
}
