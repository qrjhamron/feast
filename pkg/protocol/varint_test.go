package protocol

import (
	"bytes"
	"testing"
)

func TestVarInt(t *testing.T) {
	tests := []struct {
		value    int32
		expected []byte
	}{
		{0, []byte{0x00}},
		{1, []byte{0x01}},
		{127, []byte{0x7f}},
		{128, []byte{0x80, 0x01}},
		{255, []byte{0xff, 0x01}},
		{25565, []byte{0xdd, 0xc7, 0x01}},
		{2097151, []byte{0xff, 0xff, 0x7f}},
		{2147483647, []byte{0xff, 0xff, 0xff, 0xff, 0x07}},
		{-1, []byte{0xff, 0xff, 0xff, 0xff, 0x0f}},
		{-2147483648, []byte{0x80, 0x80, 0x80, 0x80, 0x08}},
	}

	for _, tc := range tests {
		// Test Writing
		var buf bytes.Buffer
		n, err := WriteVarInt(&buf, tc.value)
		if err != nil {
			t.Errorf("WriteVarInt(%d) error: %v", tc.value, err)
		}
		if !bytes.Equal(buf.Bytes(), tc.expected) {
			t.Errorf("WriteVarInt(%d) = %v, expected %v", tc.value, buf.Bytes(), tc.expected)
		}
		if n != len(tc.expected) {
			t.Errorf("WriteVarInt(%d) n = %d, expected %d", tc.value, n, len(tc.expected))
		}

		// Test Reading
		reader := bytes.NewReader(tc.expected)
		val, size, err := ReadVarInt(reader)
		if err != nil {
			t.Errorf("ReadVarInt(%v) error: %v", tc.expected, err)
		}
		if val != tc.value {
			t.Errorf("ReadVarInt(%v) = %d, expected %d", tc.expected, val, tc.value)
		}
		if size != len(tc.expected) {
			t.Errorf("ReadVarInt(%v) size = %d, expected %d", tc.expected, size, len(tc.expected))
		}

		// Test Size
		sz := VarIntSize(tc.value)
		if sz != len(tc.expected) {
			t.Errorf("VarIntSize(%d) = %d, expected %d", tc.value, sz, len(tc.expected))
		}
	}
}
