package state

import (
	"bytes"
	"encoding/binary"
	"fmt"

	"github.com/qrjhamron/feast/pkg/protocol"
	"github.com/qrjhamron/feast/pkg/protocol/consts"
)

// Dispatcher decodes raw packets into typed events.
type Dispatcher struct {
	bus *EventBus
}

// NewDispatcher creates a new dispatcher with an event bus.
func NewDispatcher(bus *EventBus) *Dispatcher {
	return &Dispatcher{bus: bus}
}

// Dispatch decodes a packet based on state and emits typed events.
func (d *Dispatcher) Dispatch(state State, p *protocol.RawPacket) error {
	// The raw PacketEvent requires copying the packet payload to keep it valid
	// after the read buffer is reused. Only pay that cost when something is
	// actually subscribed to raw packets (the production client subscribes to
	// typed events only).
	if d.bus.has("packet") || d.bus.has("*") {
		d.bus.Emit(PacketEvent{ID: p.ID, Data: append([]byte(nil), p.Data...)})
	}

	switch state {
	case StateLogin:
		return d.dispatchLogin(p)
	case StateConfiguration:
		return d.dispatchConfiguration(p)
	case StatePlay:
		return d.dispatchPlay(p)
	default:
		return nil
	}
}

func (d *Dispatcher) dispatchLogin(p *protocol.RawPacket) error {
	r := protocol.NewReader(bytes.NewReader(p.Data))
	switch p.ID {
	case consts.LoginClientboundDisconnect:
		pkt := &protocol.LoginClientboundDisconnectPacket{}
		if err := pkt.Unmarshal(r); err != nil {
			return err
		}
		d.bus.Emit(KickEvent{Reason: pkt.Reason})
		d.bus.Emit(DisconnectEvent{Reason: pkt.Reason})
	case consts.LoginClientboundLoginSuccess:
		pkt := &protocol.LoginClientboundLoginSuccessPacket{}
		if err := pkt.Unmarshal(r); err != nil {
			return err
		}
		d.bus.Emit(LoginEvent{UUID: uuidToString(pkt.UUID), RawUUID: pkt.UUID, Username: pkt.Username})
	case consts.LoginClientboundEncryptionRequest:
		return fmt.Errorf("online-mode encryption is intentionally unsupported")
	case consts.LoginClientboundSetCompression,
		consts.LoginClientboundLoginPluginRequest:
		return nil
	default:
		return nil
	}
	return nil
}

func (d *Dispatcher) dispatchConfiguration(p *protocol.RawPacket) error {
	r := protocol.NewReader(bytes.NewReader(p.Data))
	switch p.ID {
	case consts.ConfigurationClientboundDisconnect:
		pkt := &protocol.ConfigClientboundDisconnectPacket{}
		if err := pkt.Unmarshal(r); err != nil {
			return err
		}
		d.bus.Emit(KickEvent{Reason: pkt.Reason})
		d.bus.Emit(DisconnectEvent{Reason: pkt.Reason})
	case consts.ConfigurationClientboundFinishConfiguration,
		consts.ConfigurationClientboundClientboundPluginMessage,
		consts.ConfigurationClientboundClientboundKeepAlive,
		consts.ConfigurationClientboundPing,
		consts.ConfigurationClientboundRegistryData,
		consts.ConfigurationClientboundRemoveResourcePack,
		consts.ConfigurationClientboundAddResourcePack,
		consts.ConfigurationClientboundFeatureFlags,
		consts.ConfigurationClientboundUpdateTags:
		return nil
	default:
		return nil
	}
	return nil
}

func (d *Dispatcher) dispatchPlay(p *protocol.RawPacket) error {
	switch p.ID {
	case consts.PlayClientboundLogin:
		pkt := &protocol.PlayClientboundLoginPacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		d.bus.Emit(PlayLoginEvent{EntityID: pkt.EntityID})
		d.bus.Emit(SpawnEvent{EntityID: pkt.EntityID, X: 0, Y: 0, Z: 0})
		return nil
	case consts.PlayClientboundSynchronizePlayerPosition:
		pkt := &protocol.PlayClientboundSynchronizePlayerPositionPacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		d.bus.Emit(PositionEvent{
			X: pkt.X, Y: pkt.Y, Z: pkt.Z,
			Yaw: pkt.Yaw, Pitch: pkt.Pitch,
			Flags: pkt.Flags, TeleportID: pkt.TeleportID,
		})
		d.bus.Emit(SpawnEvent{X: pkt.X, Y: pkt.Y, Z: pkt.Z})
	case consts.PlayClientboundSystemChatMessage:
		pkt := &protocol.PlayClientboundSystemChatMessagePacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		d.bus.Emit(ChatEvent{Sender: "system", Message: pkt.DisplayText()})
	case consts.PlayClientboundPlayerChatMessage:
		pkt := &protocol.PlayClientboundPlayerChatMessagePacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		d.bus.Emit(ChatEvent{Sender: "player", Message: pkt.DisplayMessage()})
	case consts.PlayClientboundClientboundKeepAlive:
		pkt := &protocol.PlayClientboundKeepAlivePacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		d.bus.Emit(KeepAliveEvent{ID: pkt.KeepAliveID})
		return nil
	case consts.PlayClientboundDisconnect:
		pkt := &protocol.PlayClientboundDisconnectPacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		d.bus.Emit(KickEvent{Reason: pkt.Reason})
		d.bus.Emit(DisconnectEvent{Reason: pkt.Reason})
	case consts.PlayClientboundChunkDataAndUpdateLight:
		pkt := &protocol.PlayClientboundChunkDataAndUpdateLightPacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		d.bus.Emit(ChunkLoadEvent{ChunkX: pkt.ChunkX, ChunkZ: pkt.ChunkZ})
		return nil
	case consts.PlayClientboundUnloadChunk:
		// Unload Chunk (0x1F) carries Chunk Z then Chunk X as two big-endian
		// ints. Since 1.20.2 the vanilla client reads them as a single
		// big-endian long (a packed ChunkPos) with Z in the high 32 bits and X
		// in the low 32 bits. Decode the 8-byte body directly to avoid pulling a
		// protocol packet type in for two integers. See the dispatcher unload
		// test, which locks this byte order.
		if len(p.Data) < 8 {
			return fmt.Errorf("unload chunk: short payload (%d bytes)", len(p.Data))
		}
		v := binary.BigEndian.Uint64(p.Data[:8])
		d.bus.Emit(ChunkUnloadEvent{
			ChunkX: int32(uint32(v)),
			ChunkZ: int32(uint32(v >> 32)),
		})
		return nil
	case consts.PlayClientboundBlockUpdate:
		pkt := &protocol.PlayClientboundBlockUpdatePacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		d.bus.Emit(BlockUpdateEvent{
			X:       pkt.Position.X,
			Y:       pkt.Position.Y,
			Z:       pkt.Position.Z,
			StateID: pkt.StateID,
		})
		return nil
	case consts.PlayClientboundUpdateSectionBlocks:
		pkt := &protocol.PlayClientboundUpdateSectionBlocksPacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		updates := make([]SectionBlockUpdate, 0, len(pkt.Changes))
		baseX := pkt.SectionPos.X * 16
		baseY := pkt.SectionPos.Y * 16
		baseZ := pkt.SectionPos.Z * 16
		for _, ch := range pkt.Changes {
			x := int32((ch.LocalPos >> 8) & 0xF)
			z := int32((ch.LocalPos >> 4) & 0xF)
			y := int32(ch.LocalPos & 0xF)
			updates = append(updates, SectionBlockUpdate{
				X:       baseX + x,
				Y:       baseY + y,
				Z:       baseZ + z,
				StateID: ch.StateID,
			})
		}
		d.bus.Emit(SectionBlocksUpdateEvent{Updates: updates})
		return nil
	case consts.PlayClientboundUpdateEntityPositionAndRotation:
		pkt := &protocol.PlayClientboundUpdateEntityPositionAndRotationPacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		d.bus.Emit(EntityMoveDeltaEvent{EntityID: pkt.EntityID, DX: pkt.DX, DY: pkt.DY, DZ: pkt.DZ})
		d.bus.Emit(EntityRotateEvent{EntityID: pkt.EntityID, Yaw: pkt.Yaw, Pitch: pkt.Pitch, OnGround: pkt.OnGround})
		return nil
	case consts.PlayClientboundSpawnEntity:
		pkt := &protocol.PlayClientboundSpawnEntityPacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		d.bus.Emit(EntitySpawnEvent{EntityID: pkt.EntityID, X: pkt.X, Y: pkt.Y, Z: pkt.Z, UUID: pkt.UUID, Type: pkt.Type})
		return nil
	case consts.PlayClientboundRemoveEntities:
		pkt := &protocol.PlayClientboundRemoveEntitiesPacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		for _, id := range pkt.EntityIDs {
			d.bus.Emit(EntityRemoveEvent{EntityID: id})
		}
		return nil
	case consts.PlayClientboundUpdateEntityPosition:
		pkt := &protocol.PlayClientboundUpdateEntityPositionPacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		d.bus.Emit(EntityMoveDeltaEvent{EntityID: pkt.EntityID, DX: pkt.DX, DY: pkt.DY, DZ: pkt.DZ})
		return nil

	case consts.PlayClientboundUpdateEntityRotation:
		pkt := &protocol.PlayClientboundUpdateEntityRotationPacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		d.bus.Emit(EntityRotateEvent{EntityID: pkt.EntityID, Yaw: pkt.Yaw, Pitch: pkt.Pitch, OnGround: pkt.OnGround})
		return nil
	case consts.PlayClientboundSetEntityVelocity:
		pkt := &protocol.PlayClientboundSetEntityVelocityPacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		d.bus.Emit(EntityVelocityEvent{EntityID: pkt.EntityID, VelocityX: pkt.VelocityX, VelocityY: pkt.VelocityY, VelocityZ: pkt.VelocityZ})
		return nil
	case consts.PlayClientboundTeleportEntity:
		pkt := &protocol.PlayClientboundTeleportEntityPacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		d.bus.Emit(EntityTeleportEvent{EntityID: pkt.EntityID, X: pkt.X, Y: pkt.Y, Z: pkt.Z, Yaw: pkt.Yaw, Pitch: pkt.Pitch, OnGround: pkt.OnGround})
		return nil
	case consts.PlayClientboundUpdateTime:
		pkt := &protocol.PlayClientboundUpdateTimePacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		d.bus.Emit(TimeUpdateEvent{WorldAge: pkt.WorldAge, TimeOfDay: pkt.TimeOfDay})
		return nil
	case consts.PlayClientboundPlayerInfoUpdate:
		pkt := &protocol.PlayClientboundPlayerInfoUpdatePacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		d.bus.Emit(PlayerInfoUpdateEvent{Actions: pkt.Actions, Players: pkt.Players})
		return nil
	case consts.PlayClientboundSetHealth:
		pkt := &protocol.PlayClientboundSetHealthPacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		d.bus.Emit(HealthEvent{Health: pkt.Health, Food: pkt.Food, Saturation: pkt.Saturation})
		return nil
	case consts.PlayClientboundSetHeldItem:
		pkt := &protocol.PlayClientboundSetHeldItemPacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		d.bus.Emit(HeldItemEvent{Slot: int(pkt.Slot)})
		return nil
	case consts.PlayClientboundSetContainerSlot:
		pkt := &protocol.PlayClientboundSetContainerSlotPacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		d.bus.Emit(InventorySlotEvent{WindowID: pkt.WindowID, StateID: pkt.StateID, Slot: pkt.Slot, Item: pkt.Item})
		return nil
	case consts.PlayClientboundSetContainerContent:
		pkt := &protocol.PlayClientboundSetContainerContentPacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		d.bus.Emit(ContainerContentEvent{WindowID: pkt.WindowID, StateID: pkt.StateID, Slots: pkt.Slots})
		return nil
	case consts.PlayClientboundSetEntityMetadata:
		pkt := &protocol.PlayClientboundSetEntityMetadataPacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		d.bus.Emit(EntityMetadataUpdateEvent{EntityID: pkt.EntityID, Metadata: pkt.Metadata})
		return nil
	case consts.PlayClientboundOpenScreen:
		pkt := &protocol.PlayClientboundOpenScreenPacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		d.bus.Emit(OpenScreenEvent{WindowID: pkt.WindowID, WindowType: pkt.WindowType, Title: pkt.Title})
		return nil
	case consts.PlayClientboundCloseContainer:
		pkt := &protocol.PlayClientboundCloseContainerPacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		d.bus.Emit(CloseContainerEvent{WindowID: pkt.WindowID})
		return nil
	}
	return nil
}

func unmarshalRaw(pkt protocol.Packet, raw *protocol.RawPacket) error {
	return pkt.Unmarshal(protocol.NewReader(bytes.NewReader(raw.Data)))
}

func uuidToString(u [16]byte) string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uint32(u[0])<<24|uint32(u[1])<<16|uint32(u[2])<<8|uint32(u[3]),
		uint16(u[4])<<8|uint16(u[5]),
		uint16(u[6])<<8|uint16(u[7]),
		uint16(u[8])<<8|uint16(u[9]),
		uint64(u[10])<<40|uint64(u[11])<<32|uint64(u[12])<<24|uint64(u[13])<<16|uint64(u[14])<<8|uint64(u[15]))
}
