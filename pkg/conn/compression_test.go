package conn

import (
	"bytes"
	"testing"
)

func TestCompressionThresholdBehavior(t *testing.T) {
	// 1. Compression disabled (threshold < 0)
	payload := []byte("hello world")
	out, err := Compress(payload, -1)
	if err != nil {
		t.Fatalf("disabled compress: %v", err)
	}
	if !bytes.Equal(out, payload) {
		t.Fatalf("expected unchanged data when disabled")
	}

	// 2. Below threshold
	outBelow, err := Compress(payload, 100) // threshold 100
	if err != nil {
		t.Fatalf("below threshold compress: %v", err)
	}
	// format: Data Length (VarInt, 0) + payload
	if outBelow[0] != 0 {
		t.Fatalf("expected data length 0 for uncompressed, got %d", outBelow[0])
	}
	if !bytes.Equal(outBelow[1:], payload) {
		t.Fatalf("payload mismatch below threshold")
	}
	decBelow, err := Decompress(outBelow)
	if err != nil || !bytes.Equal(decBelow, payload) {
		t.Fatalf("below threshold decompress: %v", err)
	}

	// 3. Equal to threshold
	outEqual, err := Compress(payload, len(payload))
	if err != nil {
		t.Fatalf("equal threshold compress: %v", err)
	}
	if outEqual[0] == 0 {
		t.Fatalf("expected data length > 0 for compressed equal threshold")
	}
	decEqual, err := Decompress(outEqual)
	if err != nil || !bytes.Equal(decEqual, payload) {
		t.Fatalf("equal threshold decompress: %v", err)
	}

	// 4. Above threshold
	outAbove, err := Compress(payload, 5) // threshold 5
	if err != nil {
		t.Fatalf("above threshold compress: %v", err)
	}
	if outAbove[0] == 0 {
		t.Fatalf("expected compressed data for above threshold")
	}
	decAbove, err := Decompress(outAbove)
	if err != nil || !bytes.Equal(decAbove, payload) {
		t.Fatalf("above threshold decompress: %v", err)
	}

	// 5. Invalid zlib data
	invalidData := append([]byte{11}, []byte("invalid zlib data")...) // VarInt length 11, followed by garbage
	_, err = Decompress(invalidData)
	if err == nil {
		t.Fatalf("expected error decompressing invalid zlib data")
	}
}
