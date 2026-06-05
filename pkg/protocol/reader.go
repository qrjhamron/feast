package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
)

const (
	MaxStringLen    int32 = 32767
	MaxByteArrayLen int32 = 1 << 20
	MaxBitSetWords  int32 = 4096
)

// Reader reads Minecraft protocol primitives from an underlying reader.
type Reader struct {
	r io.Reader
}

// NewReader returns a new protocol Reader.
func NewReader(r io.Reader) *Reader { return &Reader{r: r} }

// ReadVarInt reads a VarInt value.
func (r *Reader) ReadVarInt() (int32, error) {
	v, _, err := ReadVarInt(r.r)
	return v, err
}

// ReadVarLong reads a VarLong value.
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
	return 0, fmt.Errorf("varlong too big")
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
	var b [1]byte
	_, err := io.ReadFull(r.r, b[:])
	return b[0], err
}

// ReadShort reads a big-endian int16.
func (r *Reader) ReadShort() (int16, error) {
	var v int16
	err := binary.Read(r.r, binary.BigEndian, &v)
	return v, err
}

// ReadInt reads a big-endian int32.
func (r *Reader) ReadInt() (int32, error) {
	var v int32
	err := binary.Read(r.r, binary.BigEndian, &v)
	return v, err
}

// ReadLong reads a big-endian int64.
func (r *Reader) ReadLong() (int64, error) {
	var v int64
	err := binary.Read(r.r, binary.BigEndian, &v)
	return v, err
}

// ReadFloat reads a big-endian float32.
func (r *Reader) ReadFloat() (float32, error) {
	var v float32
	err := binary.Read(r.r, binary.BigEndian, &v)
	return v, err
}

// ReadDouble reads a big-endian float64.
func (r *Reader) ReadDouble() (float64, error) {
	var v float64
	err := binary.Read(r.r, binary.BigEndian, &v)
	return v, err
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
