package protocol

import (
	"bytes"
	"testing"
)

// fixedReader is a cheap repeating reader that never allocates, so benchmarks
// measure only codec overhead.
type fixedReader struct {
	data []byte
	pos  int
}

func (f *fixedReader) Read(p []byte) (int, error) {
	n := 0
	for n < len(p) {
		if f.pos >= len(f.data) {
			f.pos = 0
		}
		p[n] = f.data[f.pos]
		f.pos++
		n++
	}
	return n, nil
}

func BenchmarkReadLong(b *testing.B) {
	r := NewReader(&fixedReader{data: []byte{1, 2, 3, 4, 5, 6, 7, 8}})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.ReadLong(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadDouble(b *testing.B) {
	r := NewReader(&fixedReader{data: []byte{1, 2, 3, 4, 5, 6, 7, 8}})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.ReadDouble(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReadShort(b *testing.B) {
	r := NewReader(&fixedReader{data: []byte{1, 2}})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := r.ReadShort(); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWriteLong(b *testing.B) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := w.WriteLong(int64(i)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWriteDouble(b *testing.B) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := w.WriteDouble(float64(i)); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPositionPacketRoundTrip exercises a hot serverbound packet that the
// movement/heartbeat loop sends continuously (3 doubles + 2 floats + bool).
func BenchmarkSetPlayerPositionAndRotationMarshal(b *testing.B) {
	pkt := &PlayServerboundSetPlayerPositionAndRotationPacket{
		X: 1.5, Y: 64.0, Z: -2.5, Yaw: 90, Pitch: 12, OnGround: true,
	}
	var buf bytes.Buffer
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		if err := pkt.Marshal(NewWriter(&buf)); err != nil {
			b.Fatal(err)
		}
	}
}
