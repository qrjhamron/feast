package protocol

import (
	"encoding/binary"
	"io"
)

// WriteString writes a length-prefixed UTF-8 string.
func WriteString(w io.Writer, s string) error {
	b := []byte(s)
	_, err := WriteVarInt(w, int32(len(b)))
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

// ReadString reads a length-prefixed UTF-8 string.
func ReadString(r io.Reader) (string, error) {
	length, _, err := ReadVarInt(r)
	if err != nil {
		return "", err
	}
	b := make([]byte, length)
	_, err = io.ReadFull(r, b)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// WriteUint16 writes a big-endian uint16.
func WriteUint16(w io.Writer, v uint16) error {
	return binary.Write(w, binary.BigEndian, v)
}

// ReadUint16 reads a big-endian uint16.
func ReadUint16(r io.Reader) (uint16, error) {
	var v uint16
	err := binary.Read(r, binary.BigEndian, &v)
	return v, err
}

// WriteDouble writes a big-endian float64.
func WriteDouble(w io.Writer, v float64) error {
	return binary.Write(w, binary.BigEndian, v)
}

// ReadDouble reads a big-endian float64.
func ReadDouble(r io.Reader) (float64, error) {
	var v float64
	err := binary.Read(r, binary.BigEndian, &v)
	return v, err
}

// WriteFloat writes a big-endian float32.
func WriteFloat(w io.Writer, v float32) error {
	return binary.Write(w, binary.BigEndian, v)
}

// ReadFloat reads a big-endian float32.
func ReadFloat(r io.Reader) (float32, error) {
	var v float32
	err := binary.Read(r, binary.BigEndian, &v)
	return v, err
}

// WriteBoolean writes a boolean (byte).
func WriteBoolean(w io.Writer, v bool) error {
	var b byte
	if v {
		b = 0x01
	}
	_, err := w.Write([]byte{b})
	return err
}

// ReadBoolean reads a boolean (byte).
func ReadBoolean(r io.Reader) (bool, error) {
	var b [1]byte
	_, err := r.Read(b[:])
	return b[0] != 0, err
}

// WriteInt64 writes a big-endian int64.
func WriteInt64(w io.Writer, v int64) error {
	return binary.Write(w, binary.BigEndian, v)
}

// ReadInt64 reads a big-endian int64.
func ReadInt64(r io.Reader) (int64, error) {
	var v int64
	err := binary.Read(r, binary.BigEndian, &v)
	return v, err
}
