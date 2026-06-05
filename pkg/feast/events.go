package feast

import (
	"log"
	"sync/atomic"
	"time"

	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/protocol/consts"
	"github.com/qrjhamron/feast/pkg/state"
	"github.com/qrjhamron/feast/pkg/world"
)

func (c *Client) registerStateHandlers() {
	c.bus.On("login", func(e state.Event) {
		ev, ok := e.(state.LoginEvent)
		if !ok {
			return
		}
		c.stateMu.Lock()
		c.player.UUID = ev.RawUUID
		c.stateMu.Unlock()
	})
	c.bus.On("play_login", func(e state.Event) {
		ev, ok := e.(state.PlayLoginEvent)
		if !ok {
			return
		}
		c.stateMu.Lock()
		c.player.EntityID = ev.EntityID
		c.stateMu.Unlock()
		if c.world != nil {
			c.world.SetBotEntityID(ev.EntityID)
		}
	})
	c.bus.On("position", func(e state.Event) {
		ev, ok := e.(state.PositionEvent)
		if !ok {
			return
		}
		c.stateMu.RLock()
		cur := c.player
		c.stateMu.RUnlock()
		x, y, z := ev.X, ev.Y, ev.Z
		yaw, pitch := ev.Yaw, ev.Pitch
		// Synchronize Player Position flags indicate relative components.
		if ev.Flags&0x01 != 0 {
			x = cur.X + ev.X
		}
		if ev.Flags&0x02 != 0 {
			y = cur.Y + ev.Y
		}
		if ev.Flags&0x04 != 0 {
			z = cur.Z + ev.Z
		}
		if ev.Flags&0x08 != 0 {
			yaw = cur.Yaw + ev.Yaw
		}
		if ev.Flags&0x10 != 0 {
			pitch = cur.Pitch + ev.Pitch
		}
		c.stateMu.Lock()
		firstSync := !c.positionSynced
		c.player.X = x
		c.player.Y = y
		c.player.Z = z
		c.player.Yaw = yaw
		c.player.Pitch = pitch
		c.positionSynced = true
		c.stateMu.Unlock()
		if firstSync {
			log.Printf("[pos] synced x=%.3f y=%.3f z=%.3f", x, y, z)
		}
		c.statsMu.Lock()
		c.stats.LastPositionSyncAt = time.Now()
		c.statsMu.Unlock()
		atomic.AddUint64(&c.positionSyncSeq, 1)
		c.teleportMu.Lock()
		go func(teleportID int32) {
			defer c.teleportMu.Unlock()
			_ = c.writePacket(&protocol.PlayServerboundConfirmTeleportationPacket{TeleportID: teleportID})
		}(ev.TeleportID)
		c.log("position sync received", -1, nil)
	})
	c.bus.On("health", func(e state.Event) {
		ev, ok := e.(state.HealthEvent)
		if !ok {
			return
		}
		c.stateMu.Lock()
		c.player.Health = ev.Health
		c.player.Food = ev.Food
		c.player.Saturation = ev.Saturation
		c.stateMu.Unlock()
	})
	c.bus.On("held_item", func(e state.Event) {
		ev, ok := e.(state.HeldItemEvent)
		if !ok {
			return
		}
		c.trackSelectedHotbarSlot(ev.Slot)
	})
	c.bus.On("inventory_slot", func(e state.Event) {
		ev, ok := e.(state.InventorySlotEvent)
		if !ok {
			return
		}
		c.trackInventorySlot(ev.Slot, ev.Item)
	})
	c.bus.On("container_content", func(e state.Event) {
		ev, ok := e.(state.ContainerContentEvent)
		if !ok {
			return
		}
		c.trackContainerContent(ev.WindowID, ev.Slots)
	})
	c.bus.On("keep_alive", func(e state.Event) {
		if _, ok := e.(state.KeepAliveEvent); !ok {
			return
		}
		c.statsMu.Lock()
		c.stats.LastKeepAliveAt = time.Now()
		c.statsMu.Unlock()
		c.log("keepalive received", consts.PlayClientboundClientboundKeepAlive, nil)
	})
	c.bus.On("kick", func(e state.Event) {
		ev, ok := e.(state.KickEvent)
		if !ok {
			return
		}
		c.log("kick received: "+ev.Reason, -1, nil)
	})
	c.bus.On("block_update", func(e state.Event) {
		ev, ok := e.(state.BlockUpdateEvent)
		if !ok {
			return
		}
		if c.applyWorldBlockUpdate(int(ev.X), int(ev.Y), int(ev.Z), ev.StateID) && c.hpaUpdater != nil {
			atomic.AddInt32(&c.hpaInvalidations, 1)
			c.hpaUpdater.NotifyUpdate([3]int{int(ev.X), int(ev.Y), int(ev.Z)})
		}
	})
	c.bus.On("section_blocks_update", func(e state.Event) {
		ev, ok := e.(state.SectionBlocksUpdateEvent)
		if !ok {
			return
		}
		for _, update := range ev.Updates {
			if c.applyWorldBlockUpdate(int(update.X), int(update.Y), int(update.Z), update.StateID) && c.hpaUpdater != nil {
				atomic.AddInt32(&c.hpaInvalidations, 1)
				c.hpaUpdater.NotifyUpdate([3]int{int(update.X), int(update.Y), int(update.Z)})
			}
		}
	})
	c.bus.On("entity_spawn", func(e state.Event) {
		ev, ok := e.(state.EntitySpawnEvent)
		if !ok {
			return
		}
		c.entities.Upsert(&world.Entity{
			ID:       ev.EntityID,
			UUID:     ev.UUID,
			Type:     world.EntityTypeFromID(ev.Type),
			X:        ev.X,
			Y:        ev.Y,
			Z:        ev.Z,
			Metadata: make(map[byte]protocol.EntityMetadataEntry),
		})
	})
	c.bus.On("entity_metadata", func(e state.Event) {
		ev, ok := e.(state.EntityMetadataUpdateEvent)
		if !ok {
			return
		}
		ent, exists := c.entities.Get(ev.EntityID)
		if !exists {
			return
		}
		if ent.Metadata == nil {
			ent.Metadata = make(map[byte]protocol.EntityMetadataEntry)
		}
		for _, m := range ev.Metadata {
			ent.Metadata[m.Index] = m
			if m.Index == 6 {
				if val, ok := m.Value.(int32); ok {
					switch val {
					case 0:
						ent.Pose = protocol.PoseStanding
					case 1:
						ent.Pose = protocol.PoseSleeping
					case 2:
						ent.Pose = protocol.PoseSneaking
					case 3:
						ent.Pose = protocol.PoseSwimming
					case 4:
						ent.Pose = protocol.PoseFallFlying
					case 5:
						ent.Pose = protocol.PoseCrawling
					default:
						ent.Pose = protocol.PoseStanding
					}
				}
			}
		}
		c.entities.Upsert(ent)
	})
	c.bus.On("entity_remove", func(e state.Event) {
		ev, ok := e.(state.EntityRemoveEvent)
		if !ok {
			return
		}
		c.entities.Remove(ev.EntityID)
	})
	c.bus.On("entity_move_delta", func(e state.Event) {
		ev, ok := e.(state.EntityMoveDeltaEvent)
		if !ok {
			return
		}
		c.entities.UpdateDelta(ev.EntityID, ev.DX, ev.DY, ev.DZ)
	})
	c.bus.On("entity_rotate", func(e state.Event) {
		ev, ok := e.(state.EntityRotateEvent)
		if !ok {
			return
		}
		// Yaw and Pitch are represented in angles 0-255 in the packet.
		// Angle to degrees = (Angle * 360) / 256
		yawDeg := (float32(ev.Yaw) * 360.0) / 256.0
		pitchDeg := (float32(ev.Pitch) * 360.0) / 256.0
		c.entities.UpdateRotation(ev.EntityID, yawDeg, pitchDeg)
	})
	c.bus.On("entity_teleport", func(e state.Event) {
		ev, ok := e.(state.EntityTeleportEvent)
		if !ok {
			return
		}
		c.entities.UpdatePosition(ev.EntityID, ev.X, ev.Y, ev.Z)
		yawDeg := (float32(ev.Yaw) * 360.0) / 256.0
		pitchDeg := (float32(ev.Pitch) * 360.0) / 256.0
		c.entities.UpdateRotation(ev.EntityID, yawDeg, pitchDeg)
	})
	c.bus.On("entity_velocity", func(e state.Event) {
		ev, ok := e.(state.EntityVelocityEvent)
		if !ok {
			return
		}
		c.entities.UpdateVelocity(ev.EntityID, ev.VelocityX, ev.VelocityY, ev.VelocityZ)
	})
}

func (c *Client) applyWorldBlockUpdate(x, y, z int, stateID int32) bool {
	if c.world == nil || stateID < 0 || stateID > 0xFFFF {
		return false
	}
	return c.world.SetBlock(x, y, z, uint16(stateID))
}
