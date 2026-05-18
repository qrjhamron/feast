package protocol

import (
	"errors"
	"io"
)

const (
	MaxVarIntLen = 5
	SegmentBits  = 0x7F
	ContinueBit  = 0x80
)

var ErrVarIntTooBig = errors.New("varint is too big")

// ReadVarInt reads a VarInt from an io.Reader.
func ReadVarInt(r io.Reader) (int32, int, error) {
	var value uint32
	var size int
	var b [1]byte

	for {
		n, err := r.Read(b[:])
		if err != nil {
			return 0, size, err
		}
		if n == 0 {
			return 0, size, io.ErrUnexpectedEOF
		}

		currentByte := b[0]
		value |= uint32(currentByte&SegmentBits) << uint32(7*size)
		size++

		if size > MaxVarIntLen {
			return 0, size, ErrVarIntTooBig
		}

		if (currentByte & ContinueBit) == 0 {
			break
		}
	}

	return int32(value), size, nil
}

// WriteVarInt writes a VarInt to an io.Writer.
func WriteVarInt(w io.Writer, value int32) (int, error) {
	uValue := uint32(value)
	var size int
	var b [1]byte

	for {
		if (uValue & ^uint32(SegmentBits)) == 0 {
			b[0] = byte(uValue)
			n, err := w.Write(b[:])
			size += n
			return size, err
		}

		b[0] = byte((uValue & SegmentBits) | ContinueBit)
		n, err := w.Write(b[:])
		if err != nil {
			return size + n, err
		}
		size += n
		uValue >>= 7
	}
}

// VarIntSize returns the number of bytes required to encode a VarInt.
func VarIntSize(value int32) int {
	uValue := uint32(value)
	size := 0
	for {
		size++
		if (uValue & ^uint32(SegmentBits)) == 0 {
			break
		}
		uValue >>= 7
	}
	return size
}
