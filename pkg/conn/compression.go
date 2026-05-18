package conn

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"

	"github.com/user/feastgo/pkg/protocol"
)

// Compress encodes packet data with Minecraft post-compression format.
//
// Input data should be uncompressed packet bytes (packet ID + packet body).
// If threshold is negative, data is returned unchanged.
func Compress(data []byte, threshold int) ([]byte, error) {
	if threshold < 0 {
		return data, nil
	}

	var out bytes.Buffer
	if len(data) < threshold {
		if _, err := protocol.WriteVarInt(&out, 0); err != nil {
			return nil, err
		}
		if _, err := out.Write(data); err != nil {
			return nil, err
		}
		return out.Bytes(), nil
	}

	if _, err := protocol.WriteVarInt(&out, int32(len(data))); err != nil {
		return nil, err
	}

	zw := zlib.NewWriter(&out)
	if _, err := zw.Write(data); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}

	return out.Bytes(), nil
}

// Decompress decodes Minecraft post-compression packet bytes.
//
// Input data should contain Data Length + payload from a compressed connection.
func Decompress(data []byte) ([]byte, error) {
	r := bytes.NewReader(data)
	dataLength, _, err := protocol.ReadVarInt(r)
	if err != nil {
		return nil, err
	}
	if dataLength < 0 {
		return nil, fmt.Errorf("negative data length")
	}

	if dataLength == 0 {
		return io.ReadAll(r)
	}

	zr, err := zlib.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	out, err := io.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	if len(out) != int(dataLength) {
		return nil, fmt.Errorf("decompressed length mismatch: got %d want %d", len(out), dataLength)
	}
	return out, nil
}
