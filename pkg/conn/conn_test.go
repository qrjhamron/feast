package conn

import (
	"bytes"
	"net"
	"testing"

	"github.com/user/feastgo/pkg/protocol"
)

type testPacket struct {
	id   int32
	data []byte
}

func (p *testPacket) PacketID() int32 { return p.id }
func (p *testPacket) Marshal(w *protocol.Writer) error {
	return w.WriteByteArray(p.data)
}
func (p *testPacket) Unmarshal(r *protocol.Reader) error {
	b, err := r.ReadByteArray()
	p.data = b
	return err
}

func TestWriteReadPacketNoCompression(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	client := New(c1)
	server := New(c2)

	want := &testPacket{id: 0x02, data: []byte("hello")}
	errCh := make(chan error, 1)
	go func() {
		errCh <- client.WritePacket(want)
	}()

	got, err := server.ReadPacket()
	if err != nil {
		t.Fatalf("read packet: %v", err)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("write packet: %v", err)
	}

	if got.ID != want.id {
		t.Fatalf("packet id mismatch: got %d want %d", got.ID, want.id)
	}
	r := protocol.NewReader(bytes.NewReader(got.Data))
	decoded, err := r.ReadByteArray()
	if err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if !bytes.Equal(decoded, want.data) {
		t.Fatalf("payload mismatch: got %x want %x", decoded, want.data)
	}
}

func TestCompressionThresholdBoundary(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	client := New(c1)
	server := New(c2)
	client.SetCompression(8)
	server.SetCompression(8)

	cases := []*testPacket{
		{id: 0x03, data: []byte("123")},
		{id: 0x04, data: bytes.Repeat([]byte("a"), 64)},
	}

	for _, tc := range cases {
		errCh := make(chan error, 1)
		go func(pkt *testPacket) { errCh <- client.WritePacket(pkt) }(tc)

		got, err := server.ReadPacket()
		if err != nil {
			t.Fatalf("read packet: %v", err)
		}
		if err := <-errCh; err != nil {
			t.Fatalf("write packet: %v", err)
		}
		if got.ID != tc.id {
			t.Fatalf("packet id mismatch: got %d want %d", got.ID, tc.id)
		}
		r := protocol.NewReader(bytes.NewReader(got.Data))
		decoded, err := r.ReadByteArray()
		if err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if !bytes.Equal(decoded, tc.data) {
			t.Fatalf("payload mismatch: got %x want %x", decoded, tc.data)
		}
	}
}
