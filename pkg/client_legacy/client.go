//go:build legacy
// +build legacy

package client

import (
	"fmt"
	"net"

	"github.com/user/feastgo/pkg/protocol"
)

type State int

const (
	StateHandshaking State = iota
	StateStatus
	StateLogin
	StateConfiguration
	StatePlay
)

type Client struct {
	Conn                 net.Conn
	State                State
	CompressionThreshold int
	UUID                 []byte

	// Position tracking
	X, Y, Z    float64
	Yaw, Pitch float32
	OnGround   bool
}

func Dial(address string) (*Client, error) {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return nil, err
	}
	return &Client{
		Conn:                 conn,
		State:                StateHandshaking,
		CompressionThreshold: -1, // -1 means disabled
	}, nil
}

func (c *Client) Close() error {
	return c.Conn.Close()
}

func (c *Client) SendPacket(p *protocol.Packet) error {
	return protocol.WritePacket(c.Conn, p, c.CompressionThreshold)
}

func (c *Client) ReadPacket() (*protocol.Packet, error) {
	p, err := protocol.ReadPacket(c.Conn, c.CompressionThreshold)
	if err == nil {
		fmt.Printf("<<< Received Packet ID 0x%02X (State %d, Length %d)\n", p.ID, c.State, len(p.Data))
	}
	return p, err
}

func (c *Client) Handshake(protocolVersion int32, host string, port uint16, nextState int32) error {
	var buf []byte
	w := &packetWriter{data: &buf}
	protocol.WriteVarInt(w, protocolVersion)
	protocol.WriteString(w, host)
	protocol.WriteUint16(w, port)
	protocol.WriteVarInt(w, nextState)

	p := &protocol.Packet{
		ID:   0x00,
		Data: buf,
	}
	return c.SendPacket(p)
}

// helper for building packet data
type packetWriter struct {
	data *[]byte
}

func (w *packetWriter) Write(p []byte) (n int, err error) {
	*w.data = append(*w.data, p...)
	return len(p), nil
}
