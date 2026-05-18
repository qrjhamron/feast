package protocol

import (
	"bytes"
	"io"
)

// RawPacket represents a framed packet with ID and raw body bytes.
//
// This is a transport-level type used by connection framing.
type RawPacket struct {
	ID   int32
	Data []byte
}

// ReadRawPacket reads a framed packet from an io.Reader, handling optional compression.
func ReadRawPacket(r io.Reader, compressionThreshold int) (*RawPacket, error) {
	packetLength, _, err := ReadVarInt(r)
	if err != nil {
		return nil, err
	}

	packetData := make([]byte, packetLength)
	if _, err := io.ReadFull(r, packetData); err != nil {
		return nil, err
	}

	dataReader := bytes.NewReader(packetData)
	if compressionThreshold >= 0 {
		dataLength, dataLengthSize, err := ReadVarInt(dataReader)
		if err != nil {
			return nil, err
		}
		if dataLength > 0 {
			compressedData := packetData[dataLengthSize:]
			decompressed, err := Decompress(compressedData, int(dataLength))
			if err != nil {
				return nil, err
			}
			dataReader = bytes.NewReader(decompressed)
		}
	}

	id, _, err := ReadVarInt(dataReader)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(dataReader)
	if err != nil {
		return nil, err
	}
	return &RawPacket{ID: id, Data: data}, nil
}

// WriteRawPacket writes a framed packet to an io.Writer, handling optional compression.
func WriteRawPacket(w io.Writer, p *RawPacket, compressionThreshold int) error {
	var packetData bytes.Buffer
	if _, err := WriteVarInt(&packetData, p.ID); err != nil {
		return err
	}
	if _, err := packetData.Write(p.Data); err != nil {
		return err
	}

	fullData := packetData.Bytes()
	if compressionThreshold >= 0 {
		var finalBuffer bytes.Buffer
		if len(fullData) >= compressionThreshold {
			if _, err := WriteVarInt(&finalBuffer, int32(len(fullData))); err != nil {
				return err
			}
			compressed, err := Compress(fullData)
			if err != nil {
				return err
			}
			if _, err := finalBuffer.Write(compressed); err != nil {
				return err
			}
		} else {
			if _, err := WriteVarInt(&finalBuffer, 0); err != nil {
				return err
			}
			if _, err := finalBuffer.Write(fullData); err != nil {
				return err
			}
		}
		if _, err := WriteVarInt(w, int32(finalBuffer.Len())); err != nil {
			return err
		}
		_, err := w.Write(finalBuffer.Bytes())
		return err
	}

	if _, err := WriteVarInt(w, int32(len(fullData))); err != nil {
		return err
	}
	_, err := w.Write(fullData)
	return err
}
