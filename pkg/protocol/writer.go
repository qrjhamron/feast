package protocol

import (
	"encoding/binary"
	"io"
	"math"
)

// Writer writes Minecraft protocol primitives to an underlying writer.
//
// A Writer is not safe for concurrent use. The scratch buffer is reused across
// calls to avoid a heap allocation per fixed-width write.
type Writer struct {
	w   io.Writer
	buf [10]byte
}

// NewWriter returns a new protocol Writer.
func NewWriter(w io.Writer) *Writer { return &Writer{w: w} }

// WriteVarInt writes a VarInt (max 5 bytes) in a single underlying write.
func (w *Writer) WriteVarInt(v int32) error {
	u := uint32(v)
	n := 0
	for {
		if u&^uint32(SegmentBits) == 0 {
			w.buf[n] = byte(u)
			n++
			break
		}
		w.buf[n] = byte((u & SegmentBits) | ContinueBit)
		n++
		u >>= 7
	}
	_, err := w.w.Write(w.buf[:n])
	return err
}

// WriteVarLong writes a VarLong (max 10 bytes) in a single underlying write.
func (w *Writer) WriteVarLong(v int64) error {
	u := uint64(v)
	n := 0
	for {
		if u&^uint64(0x7f) == 0 {
			w.buf[n] = byte(u)
			n++
			break
		}
		w.buf[n] = byte((u & 0x7f) | 0x80)
		n++
		u >>= 7
	}
	_, err := w.w.Write(w.buf[:n])
	return err
}

// WriteString writes a length-prefixed UTF-8 string.
func (w *Writer) WriteString(s string) error {
	b := []byte(s)
	if err := w.WriteVarInt(int32(len(b))); err != nil {
		return err
	}
	_, err := w.w.Write(b)
	return err
}

// WriteUUID writes a UUID as 16 raw bytes.
func (w *Writer) WriteUUID(u [16]byte) error {
	_, err := w.w.Write(u[:])
	return err
}

// WriteBoolean writes a bool.
func (w *Writer) WriteBoolean(v bool) error {
	w.buf[0] = 0
	if v {
		w.buf[0] = 1
	}
	_, err := w.w.Write(w.buf[:1])
	return err
}

// WriteByte writes a single byte.
func (w *Writer) WriteByte(v byte) error {
	w.buf[0] = v
	_, err := w.w.Write(w.buf[:1])
	return err
}

// WriteShort writes a big-endian int16.
func (w *Writer) WriteShort(v int16) error {
	binary.BigEndian.PutUint16(w.buf[:2], uint16(v))
	_, err := w.w.Write(w.buf[:2])
	return err
}

// WriteInt writes a big-endian int32.
func (w *Writer) WriteInt(v int32) error {
	binary.BigEndian.PutUint32(w.buf[:4], uint32(v))
	_, err := w.w.Write(w.buf[:4])
	return err
}

// WriteLong writes a big-endian int64.
func (w *Writer) WriteLong(v int64) error {
	binary.BigEndian.PutUint64(w.buf[:8], uint64(v))
	_, err := w.w.Write(w.buf[:8])
	return err
}

// WriteFloat writes a big-endian float32.
func (w *Writer) WriteFloat(v float32) error {
	binary.BigEndian.PutUint32(w.buf[:4], math.Float32bits(v))
	_, err := w.w.Write(w.buf[:4])
	return err
}

// WriteDouble writes a big-endian float64.
func (w *Writer) WriteDouble(v float64) error {
	binary.BigEndian.PutUint64(w.buf[:8], math.Float64bits(v))
	_, err := w.w.Write(w.buf[:8])
	return err
}

// WriteByteArray writes a VarInt-length-prefixed byte array.
func (w *Writer) WriteByteArray(b []byte) error {
	if err := w.WriteVarInt(int32(len(b))); err != nil {
		return err
	}
	_, err := w.w.Write(b)
	return err
}

// WriteBitSet writes a bitset as VarInt length + int64 words.
func (w *Writer) WriteBitSet(words []int64) error {
	if err := w.WriteVarInt(int32(len(words))); err != nil {
		return err
	}
	for _, v := range words {
		if err := w.WriteLong(v); err != nil {
			return err
		}
	}
	return nil
}

// WriteNBT writes raw NBT bytes as provided.
func (w *Writer) WriteNBT(raw []byte) error {
	_, err := w.w.Write(raw)
	return err
}
