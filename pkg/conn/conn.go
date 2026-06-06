package conn

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"time"

	"github.com/qrjhamron/feast/pkg/protocol"
)

// Conn is a Minecraft protocol transport connection.
type Conn struct {
	raw                  net.Conn
	reader               io.Reader
	writer               io.Writer
	br                   *bufio.Reader
	bw                   *bufio.Writer
	pr                   *protocol.Reader
	compressionThreshold int
	closed               atomic.Bool
}

// New creates a new protocol connection wrapper.
func New(c net.Conn) *Conn {
	mc := &Conn{raw: c, reader: c, writer: c, compressionThreshold: -1}
	mc.rebuildBuffers()
	return mc
}

// SetCompression sets packet compression threshold. Use negative value to disable.
func (c *Conn) SetCompression(threshold int) {
	c.compressionThreshold = threshold
}

// ReadPacket reads one framed packet from the stream.
//
// After [Conn.Close] it returns a wrapped net.ErrClosed (recognizable via
// errors.Is(err, net.ErrClosed)) without touching the underlying socket.
func (c *Conn) ReadPacket() (*protocol.RawPacket, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("conn: read after close: %w", net.ErrClosed)
	}
	frameLen, err := c.pr.ReadVarInt()
	if err != nil {
		return nil, err
	}
	if frameLen < 0 || frameLen > protocol.MaxRawPacketLen {
		return nil, fmt.Errorf("invalid packet frame length: %d", frameLen)
	}

	frame := make([]byte, frameLen)
	if _, err := io.ReadFull(c.br, frame); err != nil {
		return nil, err
	}

	packetBytes := frame
	if c.compressionThreshold >= 0 {
		dec, err := Decompress(frame)
		if err != nil {
			return nil, err
		}
		packetBytes = dec
	}

	r := protocol.NewReader(bytes.NewReader(packetBytes))
	id, err := r.ReadVarInt()
	if err != nil {
		return nil, err
	}
	data, err := r.ReadRemainingBytes()
	if err != nil {
		return nil, err
	}
	return &protocol.RawPacket{ID: id, Data: data}, nil
}

// WritePacket writes one typed packet to the stream.
//
// After [Conn.Close] it returns a wrapped net.ErrClosed (recognizable via
// errors.Is(err, net.ErrClosed)) without touching the underlying socket.
func (c *Conn) WritePacket(p protocol.Packet) error {
	if c.closed.Load() {
		return fmt.Errorf("conn: write after close: %w", net.ErrClosed)
	}
	var payload bytes.Buffer
	pw := protocol.NewWriter(&payload)
	if err := pw.WriteVarInt(p.PacketID()); err != nil {
		return err
	}
	if err := p.Marshal(pw); err != nil {
		return err
	}

	packetBytes := payload.Bytes()
	framePayload := packetBytes
	if c.compressionThreshold >= 0 {
		compressed, err := Compress(packetBytes, c.compressionThreshold)
		if err != nil {
			return err
		}
		framePayload = compressed
	}

	if err := c.prWriteVarInt(int32(len(framePayload))); err != nil {
		return err
	}
	if _, err := c.bw.Write(framePayload); err != nil {
		return err
	}
	return c.bw.Flush()
}

// Close closes the underlying network connection. It is idempotent: the first
// call closes the socket, and subsequent calls are no-ops that return nil.
func (c *Conn) Close() error {
	if c.closed.Swap(true) {
		return nil
	}
	return c.raw.Close()
}

// SetReadDeadline sets the read deadline on the underlying network connection.
func (c *Conn) SetReadDeadline(t time.Time) error {
	return c.raw.SetReadDeadline(t)
}

// SetWriteDeadline sets the write deadline on the underlying network connection.
func (c *Conn) SetWriteDeadline(t time.Time) error {
	return c.raw.SetWriteDeadline(t)
}

// SetDeadline sets the read and write deadlines on the underlying network connection.
func (c *Conn) SetDeadline(t time.Time) error {
	return c.raw.SetDeadline(t)
}

func (c *Conn) rebuildBuffers() {
	c.br = bufio.NewReader(c.reader)
	c.bw = bufio.NewWriter(c.writer)
	c.pr = protocol.NewReader(c.br)
}

func (c *Conn) prWriteVarInt(v int32) error {
	pw := protocol.NewWriter(c.bw)
	return pw.WriteVarInt(v)
}
