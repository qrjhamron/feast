package protocol

import (
	"bytes"
	"errors"
	"io"
	"math"
	"testing"
)

// TestReaderWriterFixedWidthRoundTrip locks in byte-exact behavior of the
// allocation-free fixed-width codecs (they previously used encoding/binary).
func TestReaderWriterFixedWidthRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)

	if err := w.WriteByte(0xAB); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteBoolean(true); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteShort(-12345); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteInt(-2000000000); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteLong(0x0102030405060708); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteFloat(math.Pi); err != nil {
		t.Fatal(err)
	}
	if err := w.WriteDouble(-math.E); err != nil {
		t.Fatal(err)
	}

	// Byte layout must match big-endian wire format exactly.
	want := []byte{
		0xAB,       // byte
		0x01,       // bool
		0xCF, 0xC7, // short -12345
		0x88, 0xCA, 0x6C, 0x00, // int -2000000000
		0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, // long 0x0102030405060708
	}
	got := buf.Bytes()
	if !bytes.Equal(got[:len(want)], want) {
		t.Fatalf("fixed-width wire bytes mismatch:\n got=%x\nwant(prefix)=%x", got[:len(want)], want)
	}

	r := NewReader(bytes.NewReader(got))
	if v, _ := r.ReadByte(); v != 0xAB {
		t.Fatalf("ReadByte=%#x", v)
	}
	if v, _ := r.ReadBoolean(); v != true {
		t.Fatalf("ReadBoolean=%v", v)
	}
	if v, _ := r.ReadShort(); v != -12345 {
		t.Fatalf("ReadShort=%d", v)
	}
	if v, _ := r.ReadInt(); v != -2000000000 {
		t.Fatalf("ReadInt=%d", v)
	}
	if v, _ := r.ReadLong(); v != 0x0102030405060708 {
		t.Fatalf("ReadLong=%d", v)
	}
	if v, _ := r.ReadFloat(); v != float32(math.Pi) {
		t.Fatalf("ReadFloat=%v", v)
	}
	if v, _ := r.ReadDouble(); v != -math.E {
		t.Fatalf("ReadDouble=%v", v)
	}
}

func TestReaderReadVarIntMatchesPackage(t *testing.T) {
	values := []int32{0, 1, 127, 128, 255, 25565, 2097151, 2147483647, -1, -2147483648}
	for _, v := range values {
		var buf bytes.Buffer
		if _, err := WriteVarInt(&buf, v); err != nil {
			t.Fatalf("WriteVarInt(%d): %v", v, err)
		}
		got, err := NewReader(bytes.NewReader(buf.Bytes())).ReadVarInt()
		if err != nil {
			t.Fatalf("Reader.ReadVarInt(%d): %v", v, err)
		}
		if got != v {
			t.Fatalf("Reader.ReadVarInt round-trip: got %d want %d", got, v)
		}
	}
}

func TestReaderReadVarIntRejectsOversized(t *testing.T) {
	// Six continuation bytes: exceeds the 5-byte VarInt limit.
	data := []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x01}
	_, err := NewReader(bytes.NewReader(data)).ReadVarInt()
	if !errors.Is(err, ErrVarIntTooBig) {
		t.Fatalf("expected ErrVarIntTooBig, got %v", err)
	}
}

func TestReaderReadVarIntTruncated(t *testing.T) {
	// Continuation bit set but stream ends.
	data := []byte{0x80}
	_, err := NewReader(bytes.NewReader(data)).ReadVarInt()
	if err == nil {
		t.Fatal("expected error for truncated varint")
	}
	if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("expected EOF-class error, got %v", err)
	}
}

func TestReaderReadVarLong(t *testing.T) {
	cases := []int64{0, 1, 127, 128, -1, 9223372036854775807, -9223372036854775808}
	for _, v := range cases {
		var buf bytes.Buffer
		if err := NewWriter(&buf).WriteVarLong(v); err != nil {
			t.Fatalf("WriteVarLong(%d): %v", v, err)
		}
		got, err := NewReader(bytes.NewReader(buf.Bytes())).ReadVarLong()
		if err != nil {
			t.Fatalf("ReadVarLong(%d): %v", v, err)
		}
		if got != v {
			t.Fatalf("VarLong round-trip: got %d want %d", got, v)
		}
	}
}

func TestReaderReadVarLongRejectsOversized(t *testing.T) {
	// Eleven continuation bytes: exceeds the 10-byte VarLong limit.
	data := []byte{0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x80, 0x01}
	_, err := NewReader(bytes.NewReader(data)).ReadVarLong()
	if err == nil {
		t.Fatal("expected error for oversized varlong")
	}
}

func TestReaderFixedWidthTruncated(t *testing.T) {
	// Long needs 8 bytes; provide 3.
	_, err := NewReader(bytes.NewReader([]byte{1, 2, 3})).ReadLong()
	if err == nil {
		t.Fatal("expected error reading truncated long")
	}
}
