package protocol

import (
	"bytes"
	"io"

	"github.com/user/feastgo/pkg/protocol/consts"
)

// PlayClientboundLoginPacket is Login (play) (0x29).
type PlayClientboundLoginPacket struct {
	EntityID int32
	TailData []byte
}

func (p *PlayClientboundLoginPacket) PacketID() int32 { return consts.PlayClientboundLogin }
func (p *PlayClientboundLoginPacket) Marshal(w *Writer) error {
	if err := w.WriteInt(p.EntityID); err != nil {
		return err
	}
	_, err := w.w.Write(p.TailData)
	return err
}
func (p *PlayClientboundLoginPacket) Unmarshal(r *Reader) error {
	id, err := r.ReadInt()
	if err != nil {
		return err
	}
	p.EntityID = id
	tail, err := r.ReadRemainingBytes()
	if err != nil {
		return err
	}
	p.TailData = tail
	return nil
}

// PlayClientboundSynchronizePlayerPositionPacket is Synchronize Player Position (0x3E).
type PlayClientboundSynchronizePlayerPositionPacket struct {
	X          float64
	Y          float64
	Z          float64
	Yaw        float32
	Pitch      float32
	Flags      byte
	TeleportID int32
	TailData   []byte
}

func (p *PlayClientboundSynchronizePlayerPositionPacket) PacketID() int32 {
	return consts.PlayClientboundSynchronizePlayerPosition
}
func (p *PlayClientboundSynchronizePlayerPositionPacket) Marshal(w *Writer) error {
	if err := w.WriteDouble(p.X); err != nil {
		return err
	}
	if err := w.WriteDouble(p.Y); err != nil {
		return err
	}
	if err := w.WriteDouble(p.Z); err != nil {
		return err
	}
	if err := w.WriteFloat(p.Yaw); err != nil {
		return err
	}
	if err := w.WriteFloat(p.Pitch); err != nil {
		return err
	}
	if err := w.WriteByte(p.Flags); err != nil {
		return err
	}
	if err := w.WriteVarInt(p.TeleportID); err != nil {
		return err
	}
	_, err := w.w.Write(p.TailData)
	return err
}
func (p *PlayClientboundSynchronizePlayerPositionPacket) Unmarshal(r *Reader) error {
	var err error
	if p.X, err = r.ReadDouble(); err != nil {
		return err
	}
	if p.Y, err = r.ReadDouble(); err != nil {
		return err
	}
	if p.Z, err = r.ReadDouble(); err != nil {
		return err
	}
	if p.Yaw, err = r.ReadFloat(); err != nil {
		return err
	}
	if p.Pitch, err = r.ReadFloat(); err != nil {
		return err
	}
	if p.Flags, err = r.ReadByte(); err != nil {
		return err
	}
	if p.TeleportID, err = r.ReadVarInt(); err != nil {
		return err
	}
	p.TailData, err = r.ReadRemainingBytes()
	return err
}

// PlayClientboundSystemChatMessagePacket is System Chat Message (0x69).
type PlayClientboundSystemChatMessagePacket struct {
	Message string
	Overlay bool
}

func (p *PlayClientboundSystemChatMessagePacket) PacketID() int32 {
	return consts.PlayClientboundSystemChatMessage
}
func (p *PlayClientboundSystemChatMessagePacket) Marshal(w *Writer) error {
	if err := w.WriteString(p.Message); err != nil {
		return err
	}
	return w.WriteBoolean(p.Overlay)
}
func (p *PlayClientboundSystemChatMessagePacket) Unmarshal(r *Reader) error {
	msg, err := r.ReadString()
	if err != nil {
		return err
	}
	ov, err := r.ReadBoolean()
	if err != nil {
		return err
	}
	p.Message = msg
	p.Overlay = ov
	return nil
}

// PlayClientboundPlayerChatMessagePacket is Player Chat Message (0x37).
type PlayClientboundPlayerChatMessagePacket struct {
	RawData []byte
}

func (p *PlayClientboundPlayerChatMessagePacket) PacketID() int32 {
	return consts.PlayClientboundPlayerChatMessage
}
func (p *PlayClientboundPlayerChatMessagePacket) Marshal(w *Writer) error {
	_, err := w.w.Write(p.RawData)
	return err
}
func (p *PlayClientboundPlayerChatMessagePacket) Unmarshal(r *Reader) error {
	data, err := r.ReadRemainingBytes()
	if err != nil {
		return err
	}
	p.RawData = data
	return nil
}

// PlayClientboundKeepAlivePacket is Clientbound Keep Alive (play) (0x24).
type PlayClientboundKeepAlivePacket struct {
	KeepAliveID int64
}

func (p *PlayClientboundKeepAlivePacket) PacketID() int32 {
	return consts.PlayClientboundClientboundKeepAlive
}
func (p *PlayClientboundKeepAlivePacket) Marshal(w *Writer) error { return w.WriteLong(p.KeepAliveID) }
func (p *PlayClientboundKeepAlivePacket) Unmarshal(r *Reader) error {
	id, err := r.ReadLong()
	if err != nil {
		return err
	}
	p.KeepAliveID = id
	return nil
}

// PlayClientboundDisconnectPacket is Disconnect (play) (0x1B).
type PlayClientboundDisconnectPacket struct {
	Reason string
}

func (p *PlayClientboundDisconnectPacket) PacketID() int32         { return consts.PlayClientboundDisconnect }
func (p *PlayClientboundDisconnectPacket) Marshal(w *Writer) error { return w.WriteString(p.Reason) }
func (p *PlayClientboundDisconnectPacket) Unmarshal(r *Reader) error {
	reason, err := r.ReadString()
	if err != nil {
		return err
	}
	p.Reason = reason
	return nil
}

// PlayClientboundChunkDataAndUpdateLightPacket is Chunk Data and Update Light (0x25).
type PlayClientboundChunkDataAndUpdateLightPacket struct {
	ChunkX   int32
	ChunkZ   int32
	TailData []byte
}

func (p *PlayClientboundChunkDataAndUpdateLightPacket) PacketID() int32 {
	return consts.PlayClientboundChunkDataAndUpdateLight
}
func (p *PlayClientboundChunkDataAndUpdateLightPacket) Marshal(w *Writer) error {
	if err := w.WriteInt(p.ChunkX); err != nil {
		return err
	}
	if err := w.WriteInt(p.ChunkZ); err != nil {
		return err
	}
	_, err := w.w.Write(p.TailData)
	return err
}
func (p *PlayClientboundChunkDataAndUpdateLightPacket) Unmarshal(r *Reader) error {
	x, err := r.ReadInt()
	if err != nil {
		return err
	}
	z, err := r.ReadInt()
	if err != nil {
		return err
	}
	tail, err := r.ReadRemainingBytes()
	if err != nil {
		return err
	}
	p.ChunkX = x
	p.ChunkZ = z
	p.TailData = tail
	return nil
}

// PlayClientboundSetHealthPacket is Set Health (0x5B).
type PlayClientboundSetHealthPacket struct {
	Health         float32
	Food           int32
	Saturation     float32
	AdditionalData []byte
}

func (p *PlayClientboundSetHealthPacket) PacketID() int32 { return consts.PlayClientboundSetHealth }
func (p *PlayClientboundSetHealthPacket) Marshal(w *Writer) error {
	if err := w.WriteFloat(p.Health); err != nil {
		return err
	}
	if err := w.WriteVarInt(p.Food); err != nil {
		return err
	}
	if err := w.WriteFloat(p.Saturation); err != nil {
		return err
	}
	_, err := w.w.Write(p.AdditionalData)
	return err
}
func (p *PlayClientboundSetHealthPacket) Unmarshal(r *Reader) error {
	h, err := r.ReadFloat()
	if err != nil {
		return err
	}
	food, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	sat, err := r.ReadFloat()
	if err != nil {
		return err
	}
	tail, err := r.ReadRemainingBytes()
	if err != nil {
		return err
	}
	p.Health = h
	p.Food = food
	p.Saturation = sat
	p.AdditionalData = tail
	return nil
}

// PlayClientboundSetContainerContentPacket is Set Container Content (0x13).
type PlayClientboundSetContainerContentPacket struct {
	WindowID    int32
	StateID     int32
	SlotCount   int32
	RawSlotData []byte
}

func (p *PlayClientboundSetContainerContentPacket) PacketID() int32 {
	return consts.PlayClientboundSetContainerContent
}
func (p *PlayClientboundSetContainerContentPacket) Marshal(w *Writer) error {
	if err := w.WriteVarInt(p.WindowID); err != nil {
		return err
	}
	if err := w.WriteVarInt(p.StateID); err != nil {
		return err
	}
	if err := w.WriteVarInt(p.SlotCount); err != nil {
		return err
	}
	_, err := w.w.Write(p.RawSlotData)
	return err
}
func (p *PlayClientboundSetContainerContentPacket) Unmarshal(r *Reader) error {
	windowID, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	stateID, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	slotCount, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	slots, err := r.ReadRemainingBytes()
	if err != nil {
		return err
	}
	p.WindowID = windowID
	p.StateID = stateID
	p.SlotCount = slotCount
	p.RawSlotData = slots
	return nil
}

// PlayServerboundConfirmTeleportationPacket is Confirm Teleportation (0x00).
type PlayServerboundConfirmTeleportationPacket struct {
	TeleportID int32
}

func (p *PlayServerboundConfirmTeleportationPacket) PacketID() int32 {
	return consts.PlayServerboundConfirmTeleportation
}
func (p *PlayServerboundConfirmTeleportationPacket) Marshal(w *Writer) error {
	return w.WriteVarInt(p.TeleportID)
}
func (p *PlayServerboundConfirmTeleportationPacket) Unmarshal(r *Reader) error {
	id, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	p.TeleportID = id
	return nil
}

// PlayServerboundPlayerSessionPacket is Player Session (0x06).
type PlayServerboundPlayerSessionPacket struct {
	SessionID [16]byte
	ExpiresAt int64
	PublicKey []byte
	Signature []byte
}

func (p *PlayServerboundPlayerSessionPacket) PacketID() int32 {
	return consts.PlayServerboundPlayerSession
}
func (p *PlayServerboundPlayerSessionPacket) Marshal(w *Writer) error {
	if _, err := w.w.Write(p.SessionID[:]); err != nil {
		return err
	}
	if err := w.WriteLong(p.ExpiresAt); err != nil {
		return err
	}
	if err := w.WriteVarInt(int32(len(p.PublicKey))); err != nil {
		return err
	}
	if _, err := w.w.Write(p.PublicKey); err != nil {
		return err
	}
	if err := w.WriteVarInt(int32(len(p.Signature))); err != nil {
		return err
	}
	_, err := w.w.Write(p.Signature)
	return err
}
func (p *PlayServerboundPlayerSessionPacket) Unmarshal(r *Reader) error {
	if _, err := io.ReadFull(r.r, p.SessionID[:]); err != nil {
		return err
	}
	exp, err := r.ReadLong()
	if err != nil {
		return err
	}
	p.ExpiresAt = exp
	pubLen, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	p.PublicKey = make([]byte, pubLen)
	if _, err := io.ReadFull(r.r, p.PublicKey); err != nil {
		return err
	}
	sigLen, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	p.Signature = make([]byte, sigLen)
	if _, err := io.ReadFull(r.r, p.Signature); err != nil {
		return err
	}
	return nil
}

// PlayServerboundChatMessagePacket is Chat Message (0x05).
type PlayServerboundChatMessagePacket struct {
	Message      string
	Timestamp    int64
	Salt         int64
	MessageCount int32
}

func (p *PlayServerboundChatMessagePacket) PacketID() int32 { return consts.PlayServerboundChatMessage }
func (p *PlayServerboundChatMessagePacket) Marshal(w *Writer) error {
	if err := w.WriteString(p.Message); err != nil {
		return err
	}
	if err := w.WriteLong(p.Timestamp); err != nil {
		return err
	}
	if err := w.WriteLong(p.Salt); err != nil {
		return err
	}
	if err := w.WriteBoolean(false); err != nil {
		return err
	}
	if err := w.WriteVarInt(p.MessageCount); err != nil {
		return err
	}
	// Acknowledged is Fixed BitSet(20) => 20 bits => 3 raw bytes, not length-prefixed.
	_, err := w.w.Write([]byte{0x00, 0x00, 0x00})
	return err
}
func (p *PlayServerboundChatMessagePacket) Unmarshal(r *Reader) error {
	msg, err := r.ReadString()
	if err != nil {
		return err
	}
	ts, err := r.ReadLong()
	if err != nil {
		return err
	}
	salt, err := r.ReadLong()
	if err != nil {
		return err
	}
	hasSig, err := r.ReadBoolean()
	if err != nil {
		return err
	}
	if hasSig {
		sig := make([]byte, 256)
		if _, err := io.ReadFull(r.r, sig); err != nil {
			return err
		}
	}
	count, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	ack := make([]byte, 3)
	if _, err := io.ReadFull(r.r, ack); err != nil {
		return err
	}
	p.Message = msg
	p.Timestamp = ts
	p.Salt = salt
	p.MessageCount = count
	return nil
}

// PlayServerboundSetPlayerPositionAndRotationPacket is Set Player Position and Rotation (0x18).
type PlayServerboundSetPlayerPositionAndRotationPacket struct {
	X        float64
	Y        float64
	Z        float64
	Yaw      float32
	Pitch    float32
	OnGround bool
}

func (p *PlayServerboundSetPlayerPositionAndRotationPacket) PacketID() int32 {
	return consts.PlayServerboundSetPlayerPositionAndRotation
}
func (p *PlayServerboundSetPlayerPositionAndRotationPacket) Marshal(w *Writer) error {
	if err := w.WriteDouble(p.X); err != nil {
		return err
	}
	if err := w.WriteDouble(p.Y); err != nil {
		return err
	}
	if err := w.WriteDouble(p.Z); err != nil {
		return err
	}
	if err := w.WriteFloat(p.Yaw); err != nil {
		return err
	}
	if err := w.WriteFloat(p.Pitch); err != nil {
		return err
	}
	return w.WriteBoolean(p.OnGround)
}
func (p *PlayServerboundSetPlayerPositionAndRotationPacket) Unmarshal(r *Reader) error {
	var err error
	if p.X, err = r.ReadDouble(); err != nil {
		return err
	}
	if p.Y, err = r.ReadDouble(); err != nil {
		return err
	}
	if p.Z, err = r.ReadDouble(); err != nil {
		return err
	}
	if p.Yaw, err = r.ReadFloat(); err != nil {
		return err
	}
	if p.Pitch, err = r.ReadFloat(); err != nil {
		return err
	}
	p.OnGround, err = r.ReadBoolean()
	return err
}

// PlayServerboundPlayerActionPacket is Player Action (0x21).
type PlayServerboundPlayerActionPacket struct {
	RawData []byte
}

func (p *PlayServerboundPlayerActionPacket) PacketID() int32 {
	return consts.PlayServerboundPlayerAction
}
func (p *PlayServerboundPlayerActionPacket) Marshal(w *Writer) error {
	_, err := w.w.Write(p.RawData)
	return err
}
func (p *PlayServerboundPlayerActionPacket) Unmarshal(r *Reader) error {
	data, err := r.ReadRemainingBytes()
	if err != nil {
		return err
	}
	p.RawData = data
	return nil
}

// PlayServerboundUseItemOnPacket is Use Item On (0x35).
type PlayServerboundUseItemOnPacket struct {
	RawData []byte
}

func (p *PlayServerboundUseItemOnPacket) PacketID() int32 { return consts.PlayServerboundUseItemOn }
func (p *PlayServerboundUseItemOnPacket) Marshal(w *Writer) error {
	_, err := w.w.Write(p.RawData)
	return err
}
func (p *PlayServerboundUseItemOnPacket) Unmarshal(r *Reader) error {
	data, err := r.ReadRemainingBytes()
	if err != nil {
		return err
	}
	p.RawData = data
	return nil
}

// PlayServerboundKeepAlivePacket is Serverbound Keep Alive (play) (0x15).
type PlayServerboundKeepAlivePacket struct {
	KeepAliveID int64
}

func (p *PlayServerboundKeepAlivePacket) PacketID() int32 {
	return consts.PlayServerboundServerboundKeepAlive
}
func (p *PlayServerboundKeepAlivePacket) Marshal(w *Writer) error { return w.WriteLong(p.KeepAliveID) }
func (p *PlayServerboundKeepAlivePacket) Unmarshal(r *Reader) error {
	id, err := r.ReadLong()
	if err != nil {
		return err
	}
	p.KeepAliveID = id
	return nil
}

func unmarshalPacketBody(p Packet, data []byte) error {
	return p.Unmarshal(NewReader(bytes.NewReader(data)))
}
