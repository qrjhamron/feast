package protocol

import (
	"bytes"
	"testing"
)

func TestRawPacketFramingCompression(t *testing.T) {
	threshold := 10
	p := &RawPacket{ID: 0x00, Data: []byte("this is a long packet that should be compressed")}

	var buf bytes.Buffer
	if err := WriteRawPacket(&buf, p, threshold); err != nil {
		t.Fatalf("WriteRawPacket error: %v", err)
	}

	readPacket, err := ReadRawPacket(&buf, threshold)
	if err != nil {
		t.Fatalf("ReadRawPacket error: %v", err)
	}

	if readPacket.ID != p.ID {
		t.Fatalf("packet ID mismatch: got %d want %d", readPacket.ID, p.ID)
	}
	if !bytes.Equal(readPacket.Data, p.Data) {
		t.Fatalf("packet data mismatch: got %q want %q", string(readPacket.Data), string(p.Data))
	}
}
