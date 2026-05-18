package protocol

import (
	"bytes"
	"testing"
)

func TestCompression(t *testing.T) {
	original := []byte("hello world, this is a test of compression logic for minecraft protocol")

	compressed, err := Compress(original)
	if err != nil {
		t.Fatalf("Compress error: %v", err)
	}

	decompressed, err := Decompress(compressed, len(original))
	if err != nil {
		t.Fatalf("Decompress error: %v", err)
	}

	if !bytes.Equal(original, decompressed) {
		t.Errorf("Decompressed data mismatch. Got %s, expected %s", string(decompressed), string(original))
	}
}
