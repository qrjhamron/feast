package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

const (
	MaxStringLen    int32 = 32767
	MaxByteArrayLen int32 = 1 << 20
	MaxBitSetWords  int32 = 4096
)

// Reader reads Minecraft protocol primitives from an underlying reader.
//
// A Reader is not safe for concurrent use. Each packet is decoded by a single
// goroutine with its own Reader, so the scratch buffer below is reused across
// calls to avoid a heap allocation per fixed-width read.
type Reader struct {
	r   io.Reader
	buf [8]byte
}

// NewReader returns a new protocol Reader.
func NewReader(r io.Reader) *Reader { return &Reader{r: r} }

// ReadVarInt reads a VarInt value (max 5 bytes).
func (r *Reader) ReadVarInt() (int32, error) {
	var value uint32
	for size := 0; ; size++ {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		value |= uint32(b&SegmentBits) << uint32(7*size)
		if size >= MaxVarIntLen-1 && b&ContinueBit != 0 {
			return 0, ErrVarIntTooBig
		}
		if b&ContinueBit == 0 {
			return int32(value), nil
		}
	}
}

// ReadVarLong reads a VarLong value (max 10 bytes).
func (r *Reader) ReadVarLong() (int64, error) {
	var value uint64
	for i := 0; i < 10; i++ {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		value |= uint64(b&0x7f) << (7 * i)
		if (b & 0x80) == 0 {
			return int64(value), nil
		}
	}
	return 0, fmt.Errorf("protocol: varlong too long (>10 bytes)")
}

// ReadString reads a length-prefixed UTF-8 string.
func (r *Reader) ReadString() (string, error) {
	l, err := r.ReadVarInt()
	if err != nil {
		return "", err
	}
	if l < 0 {
		return "", fmt.Errorf("negative string length")
	}
	if l > MaxStringLen {
		return "", fmt.Errorf("string length exceeds max: %d", l)
	}
	buf := make([]byte, l)
	if _, err := io.ReadFull(r.r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

// ReadUUID reads a UUID as 16 raw bytes.
func (r *Reader) ReadUUID() ([16]byte, error) {
	var u [16]byte
	_, err := io.ReadFull(r.r, u[:])
	return u, err
}

// ReadBoolean reads a bool.
func (r *Reader) ReadBoolean() (bool, error) {
	b, err := r.ReadByte()
	if err != nil {
		return false, err
	}
	return b != 0, nil
}

// ReadByte reads a signed byte.
func (r *Reader) ReadByte() (byte, error) {
	if _, err := io.ReadFull(r.r, r.buf[:1]); err != nil {
		return 0, err
	}
	return r.buf[0], nil
}

// ReadShort reads a big-endian int16.
func (r *Reader) ReadShort() (int16, error) {
	if _, err := io.ReadFull(r.r, r.buf[:2]); err != nil {
		return 0, err
	}
	return int16(binary.BigEndian.Uint16(r.buf[:2])), nil
}

// ReadInt reads a big-endian int32.
func (r *Reader) ReadInt() (int32, error) {
	if _, err := io.ReadFull(r.r, r.buf[:4]); err != nil {
		return 0, err
	}
	return int32(binary.BigEndian.Uint32(r.buf[:4])), nil
}

// ReadLong reads a big-endian int64.
func (r *Reader) ReadLong() (int64, error) {
	if _, err := io.ReadFull(r.r, r.buf[:8]); err != nil {
		return 0, err
	}
	return int64(binary.BigEndian.Uint64(r.buf[:8])), nil
}

// ReadFloat reads a big-endian float32.
func (r *Reader) ReadFloat() (float32, error) {
	if _, err := io.ReadFull(r.r, r.buf[:4]); err != nil {
		return 0, err
	}
	return math.Float32frombits(binary.BigEndian.Uint32(r.buf[:4])), nil
}

// ReadDouble reads a big-endian float64.
func (r *Reader) ReadDouble() (float64, error) {
	if _, err := io.ReadFull(r.r, r.buf[:8]); err != nil {
		return 0, err
	}
	return math.Float64frombits(binary.BigEndian.Uint64(r.buf[:8])), nil
}

// ReadByteArray reads a VarInt-length-prefixed byte array.
func (r *Reader) ReadByteArray() ([]byte, error) {
	l, err := r.ReadVarInt()
	if err != nil {
		return nil, err
	}
	if l < 0 {
		return nil, fmt.Errorf("negative byte array length")
	}
	if l > MaxByteArrayLen {
		return nil, fmt.Errorf("byte array length exceeds max: %d", l)
	}
	buf := make([]byte, l)
	_, err = io.ReadFull(r.r, buf)
	return buf, err
}

// ReadBitSet reads a bitset encoded as VarInt length + int64 words.
func (r *Reader) ReadBitSet() ([]int64, error) {
	l, err := r.ReadVarInt()
	if err != nil {
		return nil, err
	}
	if l < 0 {
		return nil, fmt.Errorf("negative bitset length")
	}
	if l > MaxBitSetWords {
		return nil, fmt.Errorf("bitset word length exceeds max: %d", l)
	}
	out := make([]int64, l)
	for i := range out {
		v, err := r.ReadLong()
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

// ReadNBT reads raw NBT bytes until the end of the packet field.
func (r *Reader) ReadNBT() ([]byte, error) {
	return r.ReadRemainingBytes()
}

// ReadRemainingBytes reads the rest of the underlying stream.
func (r *Reader) ReadRemainingBytes() ([]byte, error) {
	return io.ReadAll(r.r)
}
