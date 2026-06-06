package conn

import (
	"errors"
	"net"
	"testing"

	"github.com/qrjhamron/feast/pkg/protocol"
)

func TestConnCloseIdempotent(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c2.Close()

	c := New(c1)
	if err := c.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	// Subsequent closes must be safe no-ops returning nil, not
	// "use of closed network connection".
	if err := c.Close(); err != nil {
		t.Fatalf("second close should be a no-op, got: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("third close should be a no-op, got: %v", err)
	}
}

func TestWriteAfterClose(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c2.Close()

	c := New(c1)
	if err := c.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	err := c.WritePacket(&testPacket{id: 0x01, data: []byte("x")})
	if err == nil {
		t.Fatal("expected error writing after close")
	}
	if !errors.Is(err, net.ErrClosed) {
		t.Fatalf("write-after-close error not recognizable as net.ErrClosed: %v", err)
	}
}

func TestReadAfterClose(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c2.Close()

	c := New(c1)
	if err := c.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	var _ *protocol.RawPacket
	_, err := c.ReadPacket()
	if err == nil {
		t.Fatal("expected error reading after close")
	}
	if !errors.Is(err, net.ErrClosed) {
		t.Fatalf("read-after-close error not recognizable as net.ErrClosed: %v", err)
	}
}
