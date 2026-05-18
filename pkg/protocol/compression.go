package protocol

import (
	"bytes"
	"compress/zlib"
	"io"
)

// Compress compresses the data using zlib.
func Compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	_, err := w.Write(data)
	if err != nil {
		return nil, err
	}
	err = w.Close()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Decompress decompresses the data using zlib.
func Decompress(data []byte, size int) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	out := make([]byte, size)
	_, err = io.ReadFull(r, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}
