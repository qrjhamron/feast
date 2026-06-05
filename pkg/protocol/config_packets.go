package protocol

import "github.com/user/feastgo/pkg/protocol/consts"

// ConfigClientboundPluginMessagePacket is Clientbound Plugin Message (configuration).
type ConfigClientboundPluginMessagePacket struct {
	Channel string
	Data    []byte
}

func (p *ConfigClientboundPluginMessagePacket) PacketID() int32 {
	return consts.ConfigurationClientboundClientboundPluginMessage
}
func (p *ConfigClientboundPluginMessagePacket) Marshal(w *Writer) error {
	if err := w.WriteString(p.Channel); err != nil {
		return err
	}
	_, err := w.w.Write(p.Data)
	return err
}
func (p *ConfigClientboundPluginMessagePacket) Unmarshal(r *Reader) error {
	ch, err := r.ReadString()
	if err != nil {
		return err
	}
	p.Channel = ch
	d, err := r.ReadRemainingBytes()
	if err != nil {
		return err
	}
	p.Data = d
	return nil
}

// ConfigClientboundDisconnectPacket is Disconnect (configuration).
type ConfigClientboundDisconnectPacket struct{ Reason string }

func (p *ConfigClientboundDisconnectPacket) PacketID() int32 {
	return consts.ConfigurationClientboundDisconnect
}
func (p *ConfigClientboundDisconnectPacket) Marshal(w *Writer) error { return w.WriteString(p.Reason) }
func (p *ConfigClientboundDisconnectPacket) Unmarshal(r *Reader) error {
	v, err := r.ReadString()
	p.Reason = v
	return err
}

// ConfigClientboundFinishPacket is Finish Configuration.
type ConfigClientboundFinishPacket struct{}

func (p *ConfigClientboundFinishPacket) PacketID() int32 {
	return consts.ConfigurationClientboundFinishConfiguration
}
func (p *ConfigClientboundFinishPacket) Marshal(_ *Writer) error   { return nil }
func (p *ConfigClientboundFinishPacket) Unmarshal(_ *Reader) error { return nil }

// ConfigClientboundKeepAlivePacket is Clientbound Keep Alive (configuration).
type ConfigClientboundKeepAlivePacket struct{ KeepAliveID int64 }

func (p *ConfigClientboundKeepAlivePacket) PacketID() int32 {
	return consts.ConfigurationClientboundClientboundKeepAlive
}
func (p *ConfigClientboundKeepAlivePacket) Marshal(w *Writer) error {
	return w.WriteLong(p.KeepAliveID)
}
func (p *ConfigClientboundKeepAlivePacket) Unmarshal(r *Reader) error {
	v, err := r.ReadLong()
	p.KeepAliveID = v
	return err
}

// ConfigClientboundPingPacket is Ping (configuration).
type ConfigClientboundPingPacket struct{ ID int32 }

func (p *ConfigClientboundPingPacket) PacketID() int32         { return consts.ConfigurationClientboundPing }
func (p *ConfigClientboundPingPacket) Marshal(w *Writer) error { return w.WriteInt(p.ID) }
func (p *ConfigClientboundPingPacket) Unmarshal(r *Reader) error {
	v, err := r.ReadInt()
	p.ID = v
	return err
}

// ConfigClientboundRegistryDataPacket is Registry Data.
type ConfigClientboundRegistryDataPacket struct {
	RegistryID string
	Data       []byte
}

func (p *ConfigClientboundRegistryDataPacket) PacketID() int32 {
	return consts.ConfigurationClientboundRegistryData
}
func (p *ConfigClientboundRegistryDataPacket) Marshal(w *Writer) error {
	if err := w.WriteString(p.RegistryID); err != nil {
		return err
	}
	_, err := w.w.Write(p.Data)
	return err
}
func (p *ConfigClientboundRegistryDataPacket) Unmarshal(r *Reader) error {
	id, err := r.ReadString()
	if err != nil {
		return err
	}
	p.RegistryID = id
	d, err := r.ReadRemainingBytes()
	if err != nil {
		return err
	}
	p.Data = d
	return nil
}

// ConfigClientboundRemoveResourcePackPacket is Remove Resource Pack (configuration).
type ConfigClientboundRemoveResourcePackPacket struct {
	HasUUID bool
	UUID    [16]byte
}

func (p *ConfigClientboundRemoveResourcePackPacket) PacketID() int32 {
	return consts.ConfigurationClientboundRemoveResourcePack
}
func (p *ConfigClientboundRemoveResourcePackPacket) Marshal(w *Writer) error {
	if err := w.WriteBoolean(p.HasUUID); err != nil {
		return err
	}
	if p.HasUUID {
		return w.WriteUUID(p.UUID)
	}
	return nil
}
func (p *ConfigClientboundRemoveResourcePackPacket) Unmarshal(r *Reader) error {
	h, err := r.ReadBoolean()
	if err != nil {
		return err
	}
	p.HasUUID = h
	if h {
		u, err := r.ReadUUID()
		if err != nil {
			return err
		}
		p.UUID = u
	}
	return nil
}

// ConfigClientboundAddResourcePackPacket is Add Resource Pack (configuration).
type ConfigClientboundAddResourcePackPacket struct {
	UUID             [16]byte
	URL              string
	Hash             string
	Forced           bool
	HasPromptMessage bool
	PromptMessage    string
}

func (p *ConfigClientboundAddResourcePackPacket) PacketID() int32 {
	return consts.ConfigurationClientboundAddResourcePack
}
func (p *ConfigClientboundAddResourcePackPacket) Marshal(w *Writer) error {
	if err := w.WriteUUID(p.UUID); err != nil {
		return err
	}
	if err := w.WriteString(p.URL); err != nil {
		return err
	}
	if err := w.WriteString(p.Hash); err != nil {
		return err
	}
	if err := w.WriteBoolean(p.Forced); err != nil {
		return err
	}
	if err := w.WriteBoolean(p.HasPromptMessage); err != nil {
		return err
	}
	if p.HasPromptMessage {
		return w.WriteString(p.PromptMessage)
	}
	return nil
}
func (p *ConfigClientboundAddResourcePackPacket) Unmarshal(r *Reader) error {
	u, err := r.ReadUUID()
	if err != nil {
		return err
	}
	p.UUID = u
	url, err := r.ReadString()
	if err != nil {
		return err
	}
	p.URL = url
	h, err := r.ReadString()
	if err != nil {
		return err
	}
	p.Hash = h
	f, err := r.ReadBoolean()
	if err != nil {
		return err
	}
	p.Forced = f
	hp, err := r.ReadBoolean()
	if err != nil {
		return err
	}
	p.HasPromptMessage = hp
	if hp {
		s, err := r.ReadString()
		if err != nil {
			return err
		}
		p.PromptMessage = s
	}
	return nil
}

// ConfigClientboundFeatureFlagsPacket is Feature Flags.
type ConfigClientboundFeatureFlagsPacket struct{ Flags []string }

func (p *ConfigClientboundFeatureFlagsPacket) PacketID() int32 {
	return consts.ConfigurationClientboundFeatureFlags
}
func (p *ConfigClientboundFeatureFlagsPacket) Marshal(w *Writer) error {
	if err := w.WriteVarInt(int32(len(p.Flags))); err != nil {
		return err
	}
	for _, f := range p.Flags {
		if err := w.WriteString(f); err != nil {
			return err
		}
	}
	return nil
}
func (p *ConfigClientboundFeatureFlagsPacket) Unmarshal(r *Reader) error {
	n, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	p.Flags = make([]string, n)
	for i := int32(0); i < n; i++ {
		s, err := r.ReadString()
		if err != nil {
			return err
		}
		p.Flags[i] = s
	}
	return nil
}

// ConfigClientboundUpdateTagsPacket is Update Tags (configuration).
type ConfigClientboundUpdateTagsPacket struct{ Data []byte }

func (p *ConfigClientboundUpdateTagsPacket) PacketID() int32 {
	return consts.ConfigurationClientboundUpdateTags
}
func (p *ConfigClientboundUpdateTagsPacket) Marshal(w *Writer) error {
	_, err := w.w.Write(p.Data)
	return err
}
func (p *ConfigClientboundUpdateTagsPacket) Unmarshal(r *Reader) error {
	d, err := r.ReadRemainingBytes()
	p.Data = d
	return err
}

// ConfigServerboundClientInformationPacket is Client Information (configuration).
type ConfigServerboundClientInformationPacket struct {
	Locale              string
	ViewDistance        byte
	ChatMode            int32
	ChatColors          bool
	DisplayedSkinParts  byte
	MainHand            int32
	EnableTextFiltering bool
	AllowServerListings bool
}

func (p *ConfigServerboundClientInformationPacket) PacketID() int32 {
	return consts.ConfigurationServerboundClientInformation
}
func (p *ConfigServerboundClientInformationPacket) Marshal(w *Writer) error {
	if err := w.WriteString(p.Locale); err != nil {
		return err
	}
	if err := w.WriteByte(p.ViewDistance); err != nil {
		return err
	}
	if err := w.WriteVarInt(p.ChatMode); err != nil {
		return err
	}
	if err := w.WriteBoolean(p.ChatColors); err != nil {
		return err
	}
	if err := w.WriteByte(p.DisplayedSkinParts); err != nil {
		return err
	}
	if err := w.WriteVarInt(p.MainHand); err != nil {
		return err
	}
	if err := w.WriteBoolean(p.EnableTextFiltering); err != nil {
		return err
	}
	return w.WriteBoolean(p.AllowServerListings)
}
func (p *ConfigServerboundClientInformationPacket) Unmarshal(r *Reader) error {
	var err error
	p.Locale, err = r.ReadString()
	if err != nil {
		return err
	}
	p.ViewDistance, err = r.ReadByte()
	if err != nil {
		return err
	}
	p.ChatMode, err = r.ReadVarInt()
	if err != nil {
		return err
	}
	p.ChatColors, err = r.ReadBoolean()
	if err != nil {
		return err
	}
	p.DisplayedSkinParts, err = r.ReadByte()
	if err != nil {
		return err
	}
	p.MainHand, err = r.ReadVarInt()
	if err != nil {
		return err
	}
	p.EnableTextFiltering, err = r.ReadBoolean()
	if err != nil {
		return err
	}
	p.AllowServerListings, err = r.ReadBoolean()
	return err
}

// ConfigServerboundPluginMessagePacket is Serverbound Plugin Message (configuration).
type ConfigServerboundPluginMessagePacket struct {
	Channel string
	Data    []byte
}

func (p *ConfigServerboundPluginMessagePacket) PacketID() int32 {
	return consts.ConfigurationServerboundServerboundPluginMessage
}
func (p *ConfigServerboundPluginMessagePacket) Marshal(w *Writer) error {
	if err := w.WriteString(p.Channel); err != nil {
		return err
	}
	_, err := w.w.Write(p.Data)
	return err
}
func (p *ConfigServerboundPluginMessagePacket) Unmarshal(r *Reader) error {
	ch, err := r.ReadString()
	if err != nil {
		return err
	}
	p.Channel = ch
	d, err := r.ReadRemainingBytes()
	p.Data = d
	return err
}

// ConfigServerboundAcknowledgeFinishPacket is Acknowledge Finish Configuration.
type ConfigServerboundAcknowledgeFinishPacket struct{}

func (p *ConfigServerboundAcknowledgeFinishPacket) PacketID() int32 {
	return consts.ConfigurationServerboundAcknowledgeFinishConfiguration
}
func (p *ConfigServerboundAcknowledgeFinishPacket) Marshal(_ *Writer) error   { return nil }
func (p *ConfigServerboundAcknowledgeFinishPacket) Unmarshal(_ *Reader) error { return nil }

// ConfigServerboundKeepAlivePacket is Serverbound Keep Alive (configuration).
type ConfigServerboundKeepAlivePacket struct{ KeepAliveID int64 }

func (p *ConfigServerboundKeepAlivePacket) PacketID() int32 {
	return consts.ConfigurationServerboundServerboundKeepAlive
}
func (p *ConfigServerboundKeepAlivePacket) Marshal(w *Writer) error {
	return w.WriteLong(p.KeepAliveID)
}
func (p *ConfigServerboundKeepAlivePacket) Unmarshal(r *Reader) error {
	v, err := r.ReadLong()
	p.KeepAliveID = v
	return err
}

// ConfigServerboundPongPacket is Pong (configuration).
type ConfigServerboundPongPacket struct{ ID int32 }

func (p *ConfigServerboundPongPacket) PacketID() int32         { return consts.ConfigurationServerboundPong }
func (p *ConfigServerboundPongPacket) Marshal(w *Writer) error { return w.WriteInt(p.ID) }
func (p *ConfigServerboundPongPacket) Unmarshal(r *Reader) error {
	v, err := r.ReadInt()
	p.ID = v
	return err
}

// ConfigServerboundResourcePackResponsePacket is Resource Pack Response (configuration).
type ConfigServerboundResourcePackResponsePacket struct {
	UUID   [16]byte
	Result int32
}

func (p *ConfigServerboundResourcePackResponsePacket) PacketID() int32 {
	return consts.ConfigurationServerboundResourcePackResponse
}
func (p *ConfigServerboundResourcePackResponsePacket) Marshal(w *Writer) error {
	if err := w.WriteUUID(p.UUID); err != nil {
		return err
	}
	return w.WriteVarInt(p.Result)
}
func (p *ConfigServerboundResourcePackResponsePacket) Unmarshal(r *Reader) error {
	u, err := r.ReadUUID()
	if err != nil {
		return err
	}
	p.UUID = u
	res, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	p.Result = res
	return nil
}
