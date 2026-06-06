package protocol

import (
	"bytes"
	"testing"

	"github.com/qrjhamron/feast/pkg/protocol/consts"
)

// --- Bundle Delimiter (0x00) -------------------------------------------------

func TestBundleDelimiterPacketDecodesZeroPayload(t *testing.T) {
	var pkt PlayClientboundBundleDelimiterPacket
	if pkt.PacketID() != consts.PlayClientboundBundleDelimiter {
		t.Fatalf("packet id = 0x%02X, want 0x00", pkt.PacketID())
	}

	var buf bytes.Buffer
	if err := pkt.Marshal(NewWriter(&buf)); err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("marshal wrote %d bytes, want 0 (bundle delimiter is zero-payload)", buf.Len())
	}

	if err := pkt.Unmarshal(NewReader(bytes.NewReader(nil))); err != nil {
		t.Fatalf("unmarshal empty body: %v", err)
	}
	if err := pkt.Unmarshal(NewReader(bytes.NewReader([]byte{}))); err != nil {
		t.Fatalf("unmarshal explicit empty body: %v", err)
	}
}

func TestBundleDelimiterRejectsUnexpectedPayload(t *testing.T) {
	var pkt PlayClientboundBundleDelimiterPacket
	if err := pkt.Unmarshal(NewReader(bytes.NewReader([]byte{0x01}))); err == nil {
		t.Fatal("expected error decoding bundle delimiter with a 1-byte payload")
	}
	if err := pkt.Unmarshal(NewReader(bytes.NewReader([]byte{0x00, 0x00, 0x00}))); err == nil {
		t.Fatal("expected error decoding bundle delimiter with a 3-byte payload")
	}
}

// --- Acknowledge Block Change (0x05) -----------------------------------------

func TestAcknowledgeBlockChangePacketEncode(t *testing.T) {
	pkt := &PlayClientboundAcknowledgeBlockChangePacket{SequenceID: 300}
	if pkt.PacketID() != consts.PlayClientboundAcknowledgeBlockChange {
		t.Fatalf("packet id = 0x%02X, want 0x05", pkt.PacketID())
	}
	var buf bytes.Buffer
	if err := pkt.Marshal(NewWriter(&buf)); err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// 300 as a VarInt is 0xAC 0x02.
	want := []byte{0xAC, 0x02}
	if !bytes.Equal(buf.Bytes(), want) {
		t.Fatalf("marshal = % X, want % X", buf.Bytes(), want)
	}
}

func TestAcknowledgeBlockChangePacketDecodeIfClientboundTriggerExists(t *testing.T) {
	for _, seq := range []int32{0, 1, 127, 128, 255, 300, 16384, 2147483647} {
		var buf bytes.Buffer
		if err := (&PlayClientboundAcknowledgeBlockChangePacket{SequenceID: seq}).Marshal(NewWriter(&buf)); err != nil {
			t.Fatalf("marshal seq=%d: %v", seq, err)
		}
		var got PlayClientboundAcknowledgeBlockChangePacket
		if err := got.Unmarshal(NewReader(bytes.NewReader(buf.Bytes()))); err != nil {
			t.Fatalf("unmarshal seq=%d: %v", seq, err)
		}
		if got.SequenceID != seq {
			t.Fatalf("round-trip seq = %d, want %d", got.SequenceID, seq)
		}
	}
}

// --- Respawn (0x45) ----------------------------------------------------------

// TestRespawnPacketDecodeProtocol765 decodes a hand-built wire image to lock the
// Protocol 765 field order independently of Marshal (so a symmetric bug in both
// directions cannot hide). Short ASCII identifiers keep the byte layout obvious.
func TestRespawnPacketDecodeProtocol765(t *testing.T) {
	image := []byte{
		0x01, 'a', // Dimension Type: String "a"
		0x01, 'b', // Dimension Name: String "b"
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x2A, // Hashed Seed: Long 42
		0x01, // Game Mode: 1 (creative)
		0xFF, // Previous Game Mode: -1 (none)
		0x00, // Is Debug: false
		0x01, // Is Flat: true
		0x00, // Has Death Location: false
		0x00, // Portal Cooldown: VarInt 0
		0x02, // Data Kept: 0x02 (keep metadata)
	}

	var pkt PlayClientboundRespawnPacket
	if err := pkt.Unmarshal(NewReader(bytes.NewReader(image))); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if pkt.DimensionType != "a" || pkt.DimensionName != "b" {
		t.Fatalf("dimension type/name = %q/%q, want a/b", pkt.DimensionType, pkt.DimensionName)
	}
	if pkt.HashedSeed != 42 {
		t.Fatalf("hashed seed = %d, want 42", pkt.HashedSeed)
	}
	if pkt.GameMode != 1 || pkt.PreviousGameMode != 0xFF {
		t.Fatalf("game mode/prev = %d/%d, want 1/255", pkt.GameMode, pkt.PreviousGameMode)
	}
	if pkt.IsDebug || !pkt.IsFlat {
		t.Fatalf("isDebug/isFlat = %v/%v, want false/true", pkt.IsDebug, pkt.IsFlat)
	}
	if pkt.HasDeathLocation {
		t.Fatalf("hasDeathLocation = true, want false")
	}
	if pkt.PortalCooldown != 0 {
		t.Fatalf("portal cooldown = %d, want 0", pkt.PortalCooldown)
	}
	if pkt.DataKept != 0x02 || !pkt.CopyMetadata() {
		t.Fatalf("dataKept = 0x%02X copyMetadata=%v, want 0x02/true", pkt.DataKept, pkt.CopyMetadata())
	}
}

func TestRespawnPacketRoundTripWithDeathLocation(t *testing.T) {
	cases := []PlayClientboundRespawnPacket{
		{
			DimensionType: "minecraft:overworld", DimensionName: "minecraft:overworld",
			HashedSeed: -123456789, GameMode: 0, PreviousGameMode: 0xFF,
			IsDebug: false, IsFlat: false, HasDeathLocation: false,
			PortalCooldown: 0, DataKept: 0x00,
		},
		{
			DimensionType: "minecraft:the_nether", DimensionName: "minecraft:the_nether",
			HashedSeed: 987654321, GameMode: 1, PreviousGameMode: 0,
			IsDebug: true, IsFlat: true, HasDeathLocation: true,
			DeathDimension: "minecraft:overworld",
			DeathLocation:  BlockPos{X: -120, Y: 70, Z: 2048},
			PortalCooldown: 40, DataKept: 0x03,
		},
	}
	for i, in := range cases {
		var buf bytes.Buffer
		if err := in.Marshal(NewWriter(&buf)); err != nil {
			t.Fatalf("case %d marshal: %v", i, err)
		}
		if in.PacketID() != consts.PlayClientboundRespawn {
			t.Fatalf("case %d packet id = 0x%02X, want 0x45", i, in.PacketID())
		}
		var got PlayClientboundRespawnPacket
		if err := got.Unmarshal(NewReader(bytes.NewReader(buf.Bytes()))); err != nil {
			t.Fatalf("case %d unmarshal: %v", i, err)
		}
		if got != in {
			t.Fatalf("case %d round-trip mismatch:\n got %+v\nwant %+v", i, got, in)
		}
	}
}

// --- Benchmarks --------------------------------------------------------------

func BenchmarkDecodeBundleDelimiter(b *testing.B) {
	// Reuse the reader so the benchmark measures the decode itself, not the
	// per-iteration reader construction. The Unmarshal path is allocation-free.
	src := bytes.NewReader(nil)
	r := NewReader(src)
	var pkt PlayClientboundBundleDelimiterPacket
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		src.Reset(nil)
		_ = pkt.Unmarshal(r)
	}
}

func BenchmarkEncodeAcknowledgeBlockChange(b *testing.B) {
	pkt := &PlayClientboundAcknowledgeBlockChangePacket{SequenceID: 123456}
	var buf bytes.Buffer
	w := NewWriter(&buf)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		_ = pkt.Marshal(w)
	}
}

func BenchmarkDecodeRespawn(b *testing.B) {
	var srcBuf bytes.Buffer
	_ = (&PlayClientboundRespawnPacket{
		DimensionType: "minecraft:overworld", DimensionName: "minecraft:overworld",
		HashedSeed: 1, GameMode: 0, PreviousGameMode: 0xFF, PortalCooldown: 0, DataKept: 0,
	}).Marshal(NewWriter(&srcBuf))
	data := srcBuf.Bytes()
	// Reuse the reader; the remaining allocations are the two decoded Identifier
	// strings (inherent), not temporary buffers retained by the decoder.
	src := bytes.NewReader(data)
	r := NewReader(src)
	var pkt PlayClientboundRespawnPacket
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		src.Reset(data)
		_ = pkt.Unmarshal(r)
	}
}
