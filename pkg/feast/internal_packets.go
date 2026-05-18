package feast

import (
	"github.com/user/feastgo/pkg/protocol"
)

// handshakePacket is Handshake (serverbound, handshaking).
type handshakePacket struct {
	ProtocolVersion int32
	ServerAddress   string
	ServerPort      uint16
	NextState       int32
}

func (p *handshakePacket) PacketID() int32 { return 0x00 }
func (p *handshakePacket) Marshal(w *protocol.Writer) error {
	if err := w.WriteVarInt(p.ProtocolVersion); err != nil {
		return err
	}
	if err := w.WriteString(p.ServerAddress); err != nil {
		return err
	}
	if err := w.WriteShort(int16(p.ServerPort)); err != nil {
		return err
	}
	return w.WriteVarInt(p.NextState)
}
func (p *handshakePacket) Unmarshal(_ *protocol.Reader) error { return nil }
