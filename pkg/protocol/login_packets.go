package protocol

import "github.com/qrjhamron/feast/pkg/protocol/consts"

// LoginProperty represents one profile property entry in Login Success.
type LoginProperty struct {
	Name      string
	Value     string
	IsSigned  bool
	Signature string
}

// LoginClientboundDisconnectPacket is Disconnect (login).
type LoginClientboundDisconnectPacket struct{ Reason string }

func (p *LoginClientboundDisconnectPacket) PacketID() int32         { return consts.LoginClientboundDisconnect }
func (p *LoginClientboundDisconnectPacket) Marshal(w *Writer) error { return w.WriteString(p.Reason) }
func (p *LoginClientboundDisconnectPacket) Unmarshal(r *Reader) error {
	v, err := r.ReadString()
	p.Reason = v
	return err
}

// LoginClientboundEncryptionRequestPacket is Encryption Request.
type LoginClientboundEncryptionRequestPacket struct {
	ServerID    string
	PublicKey   []byte
	VerifyToken []byte
}

func (p *LoginClientboundEncryptionRequestPacket) PacketID() int32 {
	return consts.LoginClientboundEncryptionRequest
}
func (p *LoginClientboundEncryptionRequestPacket) Marshal(w *Writer) error {
	if err := w.WriteString(p.ServerID); err != nil {
		return err
	}
	if err := w.WriteByteArray(p.PublicKey); err != nil {
		return err
	}
	return w.WriteByteArray(p.VerifyToken)
}
func (p *LoginClientboundEncryptionRequestPacket) Unmarshal(r *Reader) error {
	v, err := r.ReadString()
	if err != nil {
		return err
	}
	p.ServerID = v
	pk, err := r.ReadByteArray()
	if err != nil {
		return err
	}
	p.PublicKey = pk
	vt, err := r.ReadByteArray()
	if err != nil {
		return err
	}
	p.VerifyToken = vt
	return nil
}

// LoginClientboundLoginSuccessPacket is Login Success.
type LoginClientboundLoginSuccessPacket struct {
	UUID       [16]byte
	Username   string
	Properties []LoginProperty
}

func (p *LoginClientboundLoginSuccessPacket) PacketID() int32 {
	return consts.LoginClientboundLoginSuccess
}
func (p *LoginClientboundLoginSuccessPacket) Marshal(w *Writer) error {
	if err := w.WriteUUID(p.UUID); err != nil {
		return err
	}
	if err := w.WriteString(p.Username); err != nil {
		return err
	}
	if err := w.WriteVarInt(int32(len(p.Properties))); err != nil {
		return err
	}
	for _, prop := range p.Properties {
		if err := w.WriteString(prop.Name); err != nil {
			return err
		}
		if err := w.WriteString(prop.Value); err != nil {
			return err
		}
		if err := w.WriteBoolean(prop.IsSigned); err != nil {
			return err
		}
		if prop.IsSigned {
			if err := w.WriteString(prop.Signature); err != nil {
				return err
			}
		}
	}
	return nil
}
func (p *LoginClientboundLoginSuccessPacket) Unmarshal(r *Reader) error {
	u, err := r.ReadUUID()
	if err != nil {
		return err
	}
	p.UUID = u
	n, err := r.ReadString()
	if err != nil {
		return err
	}
	p.Username = n
	count, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	p.Properties = make([]LoginProperty, count)
	for i := int32(0); i < count; i++ {
		name, err := r.ReadString()
		if err != nil {
			return err
		}
		value, err := r.ReadString()
		if err != nil {
			return err
		}
		signed, err := r.ReadBoolean()
		if err != nil {
			return err
		}
		prop := LoginProperty{Name: name, Value: value, IsSigned: signed}
		if signed {
			sig, err := r.ReadString()
			if err != nil {
				return err
			}
			prop.Signature = sig
		}
		p.Properties[i] = prop
	}
	return nil
}

// LoginClientboundSetCompressionPacket is Set Compression.
type LoginClientboundSetCompressionPacket struct{ Threshold int32 }

func (p *LoginClientboundSetCompressionPacket) PacketID() int32 {
	return consts.LoginClientboundSetCompression
}
func (p *LoginClientboundSetCompressionPacket) Marshal(w *Writer) error {
	return w.WriteVarInt(p.Threshold)
}
func (p *LoginClientboundSetCompressionPacket) Unmarshal(r *Reader) error {
	v, err := r.ReadVarInt()
	p.Threshold = v
	return err
}

// LoginClientboundPluginRequestPacket is Login Plugin Request.
type LoginClientboundPluginRequestPacket struct {
	MessageID int32
	Channel   string
	Data      []byte
}

func (p *LoginClientboundPluginRequestPacket) PacketID() int32 {
	return consts.LoginClientboundLoginPluginRequest
}
func (p *LoginClientboundPluginRequestPacket) Marshal(w *Writer) error {
	if err := w.WriteVarInt(p.MessageID); err != nil {
		return err
	}
	if err := w.WriteString(p.Channel); err != nil {
		return err
	}
	_, err := w.w.Write(p.Data)
	return err
}
func (p *LoginClientboundPluginRequestPacket) Unmarshal(r *Reader) error {
	id, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	p.MessageID = id
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

// LoginServerboundStartPacket is Login Start.
type LoginServerboundStartPacket struct {
	Username string
	UUID     [16]byte
}

func (p *LoginServerboundStartPacket) PacketID() int32 { return consts.LoginServerboundLoginStart }
func (p *LoginServerboundStartPacket) Marshal(w *Writer) error {
	if err := w.WriteString(p.Username); err != nil {
		return err
	}
	if err := w.WriteUUID(p.UUID); err != nil {
		return err
	}
	return nil
}
func (p *LoginServerboundStartPacket) Unmarshal(r *Reader) error {
	n, err := r.ReadString()
	if err != nil {
		return err
	}
	p.Username = n
	u, err := r.ReadUUID()
	if err != nil {
		return err
	}
	p.UUID = u
	return nil
}

// LoginServerboundEncryptionResponsePacket is Encryption Response.
type LoginServerboundEncryptionResponsePacket struct {
	SharedSecret []byte
	VerifyToken  []byte
}

func (p *LoginServerboundEncryptionResponsePacket) PacketID() int32 {
	return consts.LoginServerboundEncryptionResponse
}
func (p *LoginServerboundEncryptionResponsePacket) Marshal(w *Writer) error {
	if err := w.WriteByteArray(p.SharedSecret); err != nil {
		return err
	}
	return w.WriteByteArray(p.VerifyToken)
}
func (p *LoginServerboundEncryptionResponsePacket) Unmarshal(r *Reader) error {
	s, err := r.ReadByteArray()
	if err != nil {
		return err
	}
	p.SharedSecret = s
	v, err := r.ReadByteArray()
	if err != nil {
		return err
	}
	p.VerifyToken = v
	return nil
}

// LoginServerboundPluginResponsePacket is Login Plugin Response.
type LoginServerboundPluginResponsePacket struct {
	MessageID  int32
	Successful bool
	Data       []byte
}

func (p *LoginServerboundPluginResponsePacket) PacketID() int32 {
	return consts.LoginServerboundLoginPluginResponse
}
func (p *LoginServerboundPluginResponsePacket) Marshal(w *Writer) error {
	if err := w.WriteVarInt(p.MessageID); err != nil {
		return err
	}
	if err := w.WriteBoolean(p.Successful); err != nil {
		return err
	}
	if p.Successful {
		_, err := w.w.Write(p.Data)
		return err
	}
	return nil
}
func (p *LoginServerboundPluginResponsePacket) Unmarshal(r *Reader) error {
	id, err := r.ReadVarInt()
	if err != nil {
		return err
	}
	p.MessageID = id
	s, err := r.ReadBoolean()
	if err != nil {
		return err
	}
	p.Successful = s
	if s {
		d, err := r.ReadRemainingBytes()
		if err != nil {
			return err
		}
		p.Data = d
	}
	return nil
}

// LoginServerboundAcknowledgedPacket is Login Acknowledged.
type LoginServerboundAcknowledgedPacket struct{}

func (p *LoginServerboundAcknowledgedPacket) PacketID() int32 {
	return consts.LoginServerboundLoginAcknowledged
}
func (p *LoginServerboundAcknowledgedPacket) Marshal(_ *Writer) error   { return nil }
func (p *LoginServerboundAcknowledgedPacket) Unmarshal(_ *Reader) error { return nil }
