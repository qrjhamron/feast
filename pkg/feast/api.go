// Package feast is the primary entry point for the FeastGo Minecraft bot library.
//
// FeastGo targets Minecraft Java Edition 1.20.4 (protocol 765) in offline mode only.
// It does not support online-mode auth, encryption, or multi-version protocols.
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

type Vec3 = world.Vec3

// ─── Top-level constructors ───────────────────────────────────────────────────

// Connect creates a new client with the given options, connects to the server,
// and waits until it enters the Play state. It returns an error if the
// connection or login sequence fails.
//
// The context is used only for the initial connection attempt; the running
// client lifecycle is managed separately via [Client.Disconnect].
func Connect(ctx context.Context, opts Options) (*Client, error) {
	c := NewClient(opts)
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

// FindNearestBlock searches loaded chunks for the nearest block with the given
// registry name within radius blocks of the bot's current position.
// Returns the hit and true when found.
func (c *Client) FindNearestBlock(name string, radius int) (world.BlockHit, bool) {
	pos := c.Position()
	return c.world.FindNearestBlock(pos, name, radius)
}

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

// NavigateTo navigates the bot to the given goal and blocks until the goal is
// satisfied, navigation fails, or the context is cancelled.
//
// Use [goal.Block], [goal.XZ], [goal.Proximity], or [goal.NearEntity] to create a goal.
func (c *Client) NavigateTo(ctx context.Context, g goal.Goal, opts ...MovementOptions) error {
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
		return err
	}

	select {
	case <-ctx.Done():
		c.StopNavigation()
		return ctx.Err()
	case err := <-done:
		return err
	}
}

// navigateTo is the internal coordinate-based navigation entry point.
func (c *Client) navigateTo(x, y, z int) error {
	return c.NavigateTo2(x, y, z)
}

// ─── Block interaction ────────────────────────────────────────────────────────

type BreakOptions struct {
	AutoTool bool
}

// BreakBlock sends start+finish digging packets for the block at pos.
// If AutoTool option is set, it selects the best matching tool and waits for block update.
func (c *Client) BreakBlock(ctx context.Context, pos BlockPos, opts ...BreakOptions) error {
	if ctx == nil {
		ctx = context.Background()
	}
	var autoTool bool
	for _, opt := range opts {
		if opt.AutoTool {
			autoTool = true
		}
	}

	if err := c.requireCoreActionReady(); err != nil {
		return err
	}
	if err := c.world.RequireBlockLoaded(world.BlockPos(pos)); err != nil {
		return err
	}
	bx, by, bz, _, _ := c.GetPosition()
	if c.blockCenterDistance(pos) > 6.0 {
		return fmt.Errorf("%w (distance %.2f)", ErrBreakOutOfReach, c.blockCenterDistance(pos))
	}

	if pos.X == int32(math.Floor(bx)) && pos.Z == int32(math.Floor(bz)) && pos.Y == int32(math.Floor(by))-1 {
		return ErrBreakTargetUnsafe
	}

	blockState, err := c.world.GetBlock(int(pos.X), int(pos.Y), int(pos.Z))
	if err != nil {
		return fmt.Errorf("failed to get block: %w", err)
	}
	if c.world.IsReplaceable(world.BlockPos(pos)) {
		return ErrBlockAir
	}
	if blockState.Name == "bedrock" || blockState.Name == "barrier" {
		return fmt.Errorf("%w: %s", ErrBlockUnbreakable, blockState.Name)
	}

	blockName := blockState.Name
	fmt.Printf("[break] auto_tool=%v\n", autoTool)
	fmt.Printf("[break] target_block=%s\n", blockName)

	if autoTool {
		preferredKind := registry.PreferredToolForBlock(blockName)
		selectedToolName := "none"
		selectedSlot := -1

		c.inventoryMu.RLock()
		currentSlot := c.inventory.SelectedHotbarSlot
		if preferredKind != registry.ToolNone {
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
		}
		c.inventoryMu.RUnlock()

		if selectedSlot != -1 {
			if selectedSlot != currentSlot {
				if err := c.SelectHotbarSlot(ctx, selectedSlot); err != nil {
					return fmt.Errorf("failed to select tool slot: %w", err)
				}
			}
			fmt.Printf("[break] selected_tool=%s\n", selectedToolName)
			fmt.Printf("[break] selected_slot=%d\n", selectedSlot)
		} else {
			heldItem, hasHeld := c.HeldItem()
			if hasHeld {
				selectedToolName = heldItem.Name
			}
			fmt.Printf("[break] selected_tool=%s\n", selectedToolName)
			fmt.Printf("[break] selected_slot=%d\n", currentSlot)
		}
	}

	// Subscribe to block update event before writing packets to prevent race conditions
	ch := make(chan struct{}, 1)
	handlerID, _ := c.bus.On("block_update", func(e state.Event) {
		if ev, ok := e.(state.BlockUpdateEvent); ok {
			if int(ev.X) == int(pos.X) && int(ev.Y) == int(pos.Y) && int(ev.Z) == int(pos.Z) {
				select {
				case ch <- struct{}{}:
				default:
				}
			}
		}
	})
	defer c.bus.Off(handlerID)

	start := &protocol.PlayServerboundPlayerActionPacket{
		Status:   protocol.PlayerActionStartDigging,
		Position: pos,
		Face:     protocol.BlockFaceTop,
		Sequence: 1,
	}
	if err := c.WritePacket(start); err != nil {
		return fmt.Errorf("break block start: %w", err)
	}
	finish := &protocol.PlayServerboundPlayerActionPacket{
		Status:   protocol.PlayerActionFinishDigging,
		Position: pos,
		Face:     protocol.BlockFaceTop,
		Sequence: 2,
	}
	if err := c.WritePacket(finish); err != nil {
		return fmt.Errorf("break block finish: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ch:
		newState, err := c.world.GetBlock(int(pos.X), int(pos.Y), int(pos.Z))
		if err != nil {
			return err
		}
		if !c.world.IsReplaceable(world.BlockPos(pos)) || newState.Name == blockName {
			return ErrBreakRolledBack
		}
		fmt.Printf("[break] result=PASS\n")
		return nil
	case <-time.After(3 * time.Second):
		return ErrBlockUpdateTimeout
	}
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
// target is the air block to place into; face is which face of the support block
// the placement is against.
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
func (c *Client) OnReady(fn func()) {
	var fired bool
	c.bus.On("position", func(e state.Event) {
		if fired {
			return
		}
		if _, ok := e.(state.PositionEvent); ok {
			fired = true
			fn()
		}
	})
}

// OnChat registers fn to be called for every player-chat message.
func (c *Client) OnChat(fn func(ChatEvent)) {
	c.bus.On("chat", func(e state.Event) {
		if ev, ok := e.(state.ChatEvent); ok {
			fn(ChatEvent{Sender: ev.Sender, Message: ev.Message})
		}
	})
}

// OnSystemChat registers fn to be called for system/server messages.
// System messages use the same ChatEvent type with Sender set to "".
func (c *Client) OnSystemChat(fn func(ChatEvent)) {
	c.bus.On("system_chat", func(e state.Event) {
		if ev, ok := e.(state.ChatEvent); ok {
			fn(ChatEvent{Sender: ev.Sender, Message: ev.Message})
		}
	})
}

// OnHealth registers fn for health/food/saturation change events.
func (c *Client) OnHealth(fn func(HealthEvent)) {
	c.bus.On("health", func(e state.Event) {
		if ev, ok := e.(state.HealthEvent); ok {
			fn(HealthEvent{Health: ev.Health, Food: ev.Food, Saturation: ev.Saturation})
		}
	})
}

// OnPosition registers fn for server position-sync events.
func (c *Client) OnPosition(fn func(PositionEvent)) {
	c.bus.On("position", func(e state.Event) {
		if ev, ok := e.(state.PositionEvent); ok {
			fn(PositionEvent{X: ev.X, Y: ev.Y, Z: ev.Z, Yaw: ev.Yaw, Pitch: ev.Pitch})
		}
	})
}

// OnBlockUpdate registers fn for block-state-change events.
func (c *Client) OnBlockUpdate(fn func(BlockUpdateEvent)) {
	c.bus.On("block_update", func(e state.Event) {
		if ev, ok := e.(state.BlockUpdateEvent); ok {
			fn(BlockUpdateEvent{X: ev.X, Y: ev.Y, Z: ev.Z, StateID: ev.StateID})
		}
	})
}

// OnEntitySpawn registers fn for entity spawn events.
func (c *Client) OnEntitySpawn(fn func(EntityEvent)) {
	c.bus.On("entity_spawn", func(e state.Event) {
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
}

// OnEntityMove registers fn for entity position-delta events.
func (c *Client) OnEntityMove(fn func(EntityEvent)) {
	c.bus.On("entity_move_delta", func(e state.Event) {
		if ev, ok := e.(state.EntityMoveDeltaEvent); ok {
			if ent, exists := c.entities.Get(ev.EntityID); exists {
				fn(EntityEvent{Entity: ent, Kind: "move"})
			}
		}
	})
}

// OnEntityRemove registers fn for entity removal events.
func (c *Client) OnEntityRemove(fn func(EntityEvent)) {
	c.bus.On("entity_remove", func(e state.Event) {
		if ev, ok := e.(state.EntityRemoveEvent); ok {
			fn(EntityEvent{Entity: &world.Entity{ID: ev.EntityID}, Kind: "remove"})
		}
	})
}

// OnError registers fn for non-fatal runtime errors.
func (c *Client) OnError(fn func(error)) {
	c.bus.On("error", func(e state.Event) {
		if ev, ok := e.(state.ErrorEvent); ok && ev.Error != nil {
			fn(ev.Error)
		}
	})
}

// OnDisconnect registers fn for disconnect events (both clean and unclean).
// The error is nil for clean disconnects.
func (c *Client) OnDisconnect(fn func(error)) {
	c.bus.On("disconnect", func(e state.Event) {
		if ev, ok := e.(state.DisconnectEvent); ok {
			if ev.Clean {
				fn(nil)
			} else {
				fn(fmt.Errorf("disconnected: %s", ev.Reason))
			}
		}
	})
}

// WaitUntilReady blocks until the bot receives its first server position sync,
// or until the context is cancelled.
func (c *Client) WaitUntilReady(ctx context.Context) error {
	ready := make(chan struct{}, 1)
	c.OnReady(func() {
		select {
		case ready <- struct{}{}:
		default:
		}
	})
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
