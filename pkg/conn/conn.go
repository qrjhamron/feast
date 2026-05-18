package conn

import (
	"bufio"
	"bytes"
	"crypto/cipher"
	"io"
	"net"

	"github.com/user/feastgo/pkg/protocol"
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
	encryptStream        cipher.Stream
	decryptStream        cipher.Stream
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

// SetEncryption enables AES/CFB8 transport encryption on both directions.
func (c *Conn) SetEncryption(key []byte) error {
	enc, dec, err := NewCFB8(key)
	if err != nil {
		return err
	}
	c.encryptStream = enc
	c.decryptStream = dec
	c.reader = &cipherReader{r: c.raw, s: c.decryptStream}
	c.writer = &cipherWriter{w: c.raw, s: c.encryptStream}
	c.rebuildBuffers()
	return nil
}

// ReadPacket reads one framed packet from the stream.
func (c *Conn) ReadPacket() (*protocol.RawPacket, error) {
	frameLen, err := c.pr.ReadVarInt()
	if err != nil {
		return nil, err
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
func (c *Conn) WritePacket(p protocol.Packet) error {
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

// Close closes the underlying network connection.
func (c *Conn) Close() error {
	return c.raw.Close()
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

type cipherReader struct {
	r io.Reader
	s cipher.Stream
}

func (r *cipherReader) Read(p []byte) (int, error) {
	n, err := r.r.Read(p)
	if n > 0 {
		r.s.XORKeyStream(p[:n], p[:n])
	}
	return n, err
}

type cipherWriter struct {
	w io.Writer
	s cipher.Stream
}

func (w *cipherWriter) Write(p []byte) (int, error) {
	buf := make([]byte, len(p))
	copy(buf, p)
	w.s.XORKeyStream(buf, buf)
	return w.w.Write(buf)
}
