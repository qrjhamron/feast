package protocol

import (
	"encoding/binary"
	"io"
)

// Writer writes Minecraft protocol primitives to an underlying writer.
type Writer struct {
	w io.Writer
}

// NewWriter returns a new protocol Writer.
func NewWriter(w io.Writer) *Writer { return &Writer{w: w} }

// WriteVarInt writes a VarInt.
func (w *Writer) WriteVarInt(v int32) error {
	_, err := WriteVarInt(w.w, v)
	return err
}

// WriteVarLong writes a VarLong.
func (w *Writer) WriteVarLong(v int64) error {
	u := uint64(v)
	for {
		if (u & ^uint64(0x7f)) == 0 {
			_, err := w.w.Write([]byte{byte(u)})
			return err
		}
		if _, err := w.w.Write([]byte{byte((u & 0x7f) | 0x80)}); err != nil {
			return err
		}
		u >>= 7
	}
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
	b := byte(0)
	if v {
		b = 1
	}
	_, err := w.w.Write([]byte{b})
	return err
}

// WriteByte writes a single byte.
func (w *Writer) WriteByte(v byte) error {
	_, err := w.w.Write([]byte{v})
	return err
}

// WriteShort writes a big-endian int16.
func (w *Writer) WriteShort(v int16) error {
	return binary.Write(w.w, binary.BigEndian, v)
}

// WriteInt writes a big-endian int32.
func (w *Writer) WriteInt(v int32) error {
	return binary.Write(w.w, binary.BigEndian, v)
}

// WriteLong writes a big-endian int64.
func (w *Writer) WriteLong(v int64) error {
	return binary.Write(w.w, binary.BigEndian, v)
}

// WriteFloat writes a big-endian float32.
func (w *Writer) WriteFloat(v float32) error {
	return binary.Write(w.w, binary.BigEndian, v)
}

// WriteDouble writes a big-endian float64.
func (w *Writer) WriteDouble(v float64) error {
	return binary.Write(w.w, binary.BigEndian, v)
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
