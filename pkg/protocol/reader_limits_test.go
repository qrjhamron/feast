package protocol

import (
	"bytes"
	"testing"
)

func TestReader_ReadString_Cap(t *testing.T) {
	var buf bytes.Buffer
	if _, err := WriteVarInt(&buf, MaxStringLen+1); err != nil {
		t.Fatalf("write varint: %v", err)
	}
	r := NewReader(bytes.NewReader(buf.Bytes()))
	if _, err := r.ReadString(); err == nil {
		t.Fatalf("expected error for oversized string")
	}
}

func TestReader_ReadByteArray_Cap(t *testing.T) {
	var buf bytes.Buffer
	if _, err := WriteVarInt(&buf, MaxByteArrayLen+1); err != nil {
		t.Fatalf("write varint: %v", err)
	}
	r := NewReader(bytes.NewReader(buf.Bytes()))
	if _, err := r.ReadByteArray(); err == nil {
		t.Fatalf("expected error for oversized byte array")
	}
}

func TestReader_ReadBitSet_Cap(t *testing.T) {
	var buf bytes.Buffer
	if _, err := WriteVarInt(&buf, MaxBitSetWords+1); err != nil {
		t.Fatalf("write varint: %v", err)
	}
	r := NewReader(bytes.NewReader(buf.Bytes()))
	if _, err := r.ReadBitSet(); err == nil {
		t.Fatalf("expected error for oversized bitset")
	}
}
