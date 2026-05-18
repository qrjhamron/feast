package state

import (
	"bytes"
	"fmt"

	"github.com/user/feastgo/pkg/protocol"
	"github.com/user/feastgo/pkg/protocol/consts"
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
	d.bus.Emit(PacketEvent{ID: p.ID, Data: append([]byte(nil), p.Data...)})

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
	case consts.LoginClientboundEncryptionRequest,
		consts.LoginClientboundSetCompression,
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
		d.bus.Emit(ChatEvent{Sender: "system", Message: pkt.Message})
	case consts.PlayClientboundPlayerChatMessage:
		pkt := &protocol.PlayClientboundPlayerChatMessagePacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		d.bus.Emit(ChatEvent{Sender: "player", Message: string(pkt.RawData)})
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
		return nil
	case consts.PlayClientboundSetHealth:
		pkt := &protocol.PlayClientboundSetHealthPacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
		d.bus.Emit(HealthEvent{Health: pkt.Health, Food: pkt.Food, Saturation: pkt.Saturation})
		return nil
	case consts.PlayClientboundSetContainerContent:
		pkt := &protocol.PlayClientboundSetContainerContentPacket{}
		if err := unmarshalRaw(pkt, p); err != nil {
			return err
		}
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
