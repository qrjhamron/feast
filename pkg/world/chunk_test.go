package world

import (
	"encoding/binary"
	"github.com/qrjhamron/feast/pkg/protocol"
	"reflect"
	"strings"
	"testing"
)

func TestParseChunkFromTailData_SingleValuedBlockState(t *testing.T) {
	resetY := withWorldY(DefaultMinY, DefaultMaxY)
	defer resetY()

	sections := make([]byte, 0)
	sections = append(sections, encodeSectionSingle(0, 5)...) // section 0 filled with state 5
	for i := 1; i < SectionCount(); i++ {
		sections = append(sections, encodeSectionSingle(0, 0)...)
	}

	tail := buildTailData(sections)
	chunk, err := ParseChunkFromTailData(3, -2, tail)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if chunk.X != 3 || chunk.Z != -2 {
		t.Fatalf("unexpected coords: got (%d,%d)", chunk.X, chunk.Z)
	}
	for i := 0; i < SectionVolume; i++ {
		if chunk.Sections[0].Blocks[i] != 5 {
			t.Fatalf("block %d: got %d want 5", i, chunk.Sections[0].Blocks[i])
		}
	}
	if !IsSolidBlockState(5) {
		t.Fatalf("state 5 should be solid")
	}
	if IsSolidBlockState(0) {
		t.Fatalf("air should be non-solid")
	}
}

func TestParseChunkFromTailData_IndirectPaletteBlockState(t *testing.T) {
	resetY := withWorldY(DefaultMinY, DefaultMaxY)
	defer resetY()

	pattern := make([]int32, SectionVolume)
	for i := 0; i < SectionVolume; i++ {
		if i%2 == 0 {
			pattern[i] = 0
		} else {
			pattern[i] = 1
		}
	}
	longs := packEntries(pattern, 4)

	sections := make([]byte, 0)
	sections = append(sections, encodeSectionIndirect(2048, 4, []int32{0, 9}, longs)...)
	for i := 1; i < SectionCount(); i++ {
		sections = append(sections, encodeSectionSingle(0, 0)...)
	}

	tail := buildTailData(sections)
	chunk, err := ParseChunkFromTailData(0, 0, tail)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	for i := 0; i < 16; i++ {
		want := int32(0)
		if i%2 == 1 {
			want = 9
		}
		if got := int32(chunk.Sections[0].Blocks[i]); got != want {
			t.Fatalf("block %d: got %d want %d", i, got, want)
		}
	}
}

func TestBlockStateRegistryPassabilityFromEmbeddedData(t *testing.T) {
	shortGrassID, ok := FindAnyStateIDByName("short_grass")
	if !ok {
		t.Fatal("missing short_grass state")
	}
	torchID, ok := FindAnyStateIDByName("torch")
	if !ok {
		t.Fatal("missing torch state")
	}
	stoneID, ok := FindAnyStateIDByName("stone")
	if !ok {
		t.Fatal("missing stone state")
	}

	grass := blockStateFromID(shortGrassID)
	if grass.Name != "short_grass" || grass.Solid {
		t.Fatalf("short_grass mismatch: %+v", grass)
	}
	torch := blockStateFromID(torchID)
	if torch.Name != "torch" || torch.Solid {
		t.Fatalf("torch mismatch: %+v", torch)
	}
	stone := blockStateFromID(stoneID)
	if stone.Name != "stone" || !stone.Solid {
		t.Fatalf("stone mismatch: %+v", stone)
	}
}

func TestParseChunkFromTailData_ConfigurableSectionCount(t *testing.T) {
	resetY := withWorldY(0, 15) // exactly one 16-block section
	defer resetY()

	sections := make([]byte, 0)
	sections = append(sections, encodeSectionSingle(0, 0)...)
	if SectionCount() != 1 {
		t.Fatalf("SectionCount()=%d want 1", SectionCount())
	}

	tail := buildTailData(sections)
	chunk, err := ParseChunkFromTailData(0, 0, tail)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := len(chunk.Sections); got != 1 {
		t.Fatalf("chunk.Sections len=%d want 1", got)
	}
}

func TestParseChunkFromTailData_ConsumesLightAndBlockEntities(t *testing.T) {
	resetY := withWorldY(DefaultMinY, DefaultMaxY)
	defer resetY()

	sections := make([]byte, 0, 1024)
	for i := 0; i < SectionCount(); i++ {
		sections = append(sections, encodeSectionSingle(0, 0)...)
	}

	tail := make([]byte, 0, 4096)
	tail = append(tail, encodeHeightmapsNBT()...)
	tail = append(tail, writeVarInt(int32(len(sections)))...)
	tail = append(tail, sections...)

	// Block entities: 1 entry at local (2,3) with Y=64.
	tail = append(tail, writeVarInt(1)...)
	tail = append(tail, byte((2<<4)|3)) // packedXZ
	tail = binary.BigEndian.AppendUint16(tail, uint16(64))
	tail = append(tail, writeVarInt(7)...) // type id
	tail = append(tail, 0)                 // NBT TAG_End

	// Light tail.
	tail = append(tail, writeVarInt(1)...) // sky light mask bitset len
	tail = binary.BigEndian.AppendUint64(tail, 1)
	tail = append(tail, writeVarInt(1)...) // block light mask bitset len
	tail = binary.BigEndian.AppendUint64(tail, 2)
	tail = append(tail, writeVarInt(0)...) // empty sky light mask
	tail = append(tail, writeVarInt(0)...) // empty block light mask

	tail = append(tail, writeVarInt(1)...) // sky light arrays count
	tail = append(tail, writeVarInt(2048)...)
	tail = append(tail, make([]byte, 2048)...)

	tail = append(tail, writeVarInt(1)...) // block light arrays count
	tail = append(tail, writeVarInt(2048)...)
	tail = append(tail, make([]byte, 2048)...)

	chunk, err := ParseChunkFromTailData(1, -2, tail)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(chunk.BlockEntities) != 1 {
		t.Fatalf("block entities len=%d want 1", len(chunk.BlockEntities))
	}
	be := chunk.BlockEntities[0]
	if be.X != 1*16+2 || be.Z != -2*16+3 || be.Y != 64 || be.TypeID != 7 {
		t.Fatalf("block entity mismatch: %+v", be)
	}
	if len(chunk.Light.SkyLightArrays) != 1 || len(chunk.Light.BlockLightArrays) != 1 {
		t.Fatalf("light arrays lens mismatch: sky=%d block=%d", len(chunk.Light.SkyLightArrays), len(chunk.Light.BlockLightArrays))
	}
	if len(chunk.Light.SkyLightMask) != 1 || chunk.Light.SkyLightMask[0] != 1 {
		t.Fatalf("sky light mask mismatch: %#v", chunk.Light.SkyLightMask)
	}
	if len(chunk.Light.BlockLightMask) != 1 || chunk.Light.BlockLightMask[0] != 2 {
		t.Fatalf("block light mask mismatch: %#v", chunk.Light.BlockLightMask)
	}
}

func TestParseChunkFromTailData_MaxIndirectPalette256Entries(t *testing.T) {
	resetY := withWorldY(DefaultMinY, DefaultMaxY)
	defer resetY()

	palette := make([]int32, 256)
	for i := 0; i < 256; i++ {
		palette[i] = int32(i)
	}
	pattern := make([]int32, SectionVolume)
	for i := range pattern {
		pattern[i] = int32(i % 256)
	}
	longs := packEntriesNoSpan(pattern, 8)

	sections := make([]byte, 0)
	sections = append(sections, encodeSectionIndirect(2048, 8, palette, longs)...)
	for i := 1; i < SectionCount(); i++ {
		sections = append(sections, encodeSectionSingle(0, 0)...)
	}

	chunk, err := ParseChunkFromTailData(0, 0, buildTailData(sections))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	for i := 0; i < 256; i++ {
		got := chunk.Sections[0].Blocks[i]
		if got != uint16(i) {
			t.Fatalf("block %d: got %d want %d", i, got, i)
		}
	}
}

func TestParseChunkFromTailData_DirectPaletteBits15AndClamp(t *testing.T) {
	resetY := withWorldY(DefaultMinY, DefaultMaxY)
	defer resetY()

	pattern := make([]int32, SectionVolume)
	for i := range pattern {
		pattern[i] = 1
	}
	pattern[0] = 26643
	pattern[1] = 30000 // invalid vanilla state, should clamp to air
	longs := packEntriesNoSpan(pattern, 15)

	section := make([]byte, 0)
	section = binary.BigEndian.AppendUint16(section, uint16(2048))
	section = append(section, 15)
	section = append(section, writeVarInt(int32(len(longs)))...)
	for _, v := range longs {
		section = binary.BigEndian.AppendUint64(section, v)
	}
	section = append(section, 0) // biome bpe=0
	section = append(section, writeVarInt(0)...)
	section = append(section, writeVarInt(0)...)

	sections := make([]byte, 0)
	sections = append(sections, section...)
	for i := 1; i < SectionCount(); i++ {
		sections = append(sections, encodeSectionSingle(0, 0)...)
	}

	chunk, err := ParseChunkFromTailData(0, 0, buildTailData(sections))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := chunk.Sections[0].Blocks[0]; got != 26643 {
		t.Fatalf("block 0: got %d want 26643", got)
	}
	if got := chunk.Sections[0].Blocks[1]; got != 0 {
		t.Fatalf("block 1: got %d want 0 (clamped air)", got)
	}
}

func TestParseChunkFromTailData_EmptyAndSingleUniqueSection(t *testing.T) {
	resetY := withWorldY(DefaultMinY, DefaultMaxY)
	defer resetY()

	sections := make([]byte, 0)
	sections = append(sections, encodeSectionSingle(0, 0)...)
	for i := 1; i < SectionCount(); i++ {
		sections = append(sections, encodeSectionSingle(0, 42)...)
	}

	chunk, err := ParseChunkFromTailData(0, 0, buildTailData(sections))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := chunk.Sections[0].Blocks[0]; got != 0 {
		t.Fatalf("empty section first block got %d want 0", got)
	}
	if got := chunk.Sections[1].Blocks[0]; got != 42 {
		t.Fatalf("single unique section block got %d want 42", got)
	}
}

func TestParseChunkFromTailData_HeightmapMotionBlockingCompact9Bit(t *testing.T) {
	resetY := withWorldY(DefaultMinY, DefaultMaxY)
	defer resetY()

	values := make([]int32, 256)
	for i := range values {
		values[i] = int32((i * 3) % 384) // keep within [0, maxY-minY+1]
	}

	sections := make([]byte, 0)
	for i := 0; i < SectionCount(); i++ {
		sections = append(sections, encodeSectionSingle(0, 0)...)
	}

	tail := buildTailDataWithHeightmaps(encodeHeightmapsNBTWithMotionBlocking(values), sections)
	chunk, err := ParseChunkFromTailData(0, 0, tail)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	for i := 0; i < 256; i++ {
		want := int16(DefaultMinY - 1 + int(values[i]))
		if got := chunk.SurfaceY[i]; got != want {
			t.Fatalf("surface[%d]=%d want %d", i, got, want)
		}
	}
}

func TestParseChunkFromTailDataWithConfig_NetherPreset(t *testing.T) {
	sections := make([]byte, 0)
	for i := 0; i < NetherConfig.SectionCount; i++ {
		sections = append(sections, encodeSectionSingle(0, 0)...)
	}
	chunk, err := ParseChunkFromTailDataWithConfig(0, 0, buildTailData(sections), NetherConfig)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if len(chunk.Sections) != NetherConfig.SectionCount {
		t.Fatalf("sections len=%d want %d", len(chunk.Sections), NetherConfig.SectionCount)
	}
	if chunk.Config.MinY != NetherConfig.MinY || chunk.Config.MaxY != NetherConfig.MaxY {
		t.Fatalf("chunk config mismatch: %+v", chunk.Config)
	}
}

func TestParseChunkFromTailData_ReportsTrailingBytesWithPacketLength(t *testing.T) {
	resetY := withWorldY(0, 15)
	defer resetY()

	sections := encodeSectionSingle(0, 0)
	tail := buildTailData(sections)
	tail = append(tail, 0x42)

	_, err := ParseChunkFromTailData(0, 0, tail)
	if err == nil {
		t.Fatal("expected trailing byte error")
	}
	want := "chunk tail: trailing 1 bytes after parse (packet length"
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err.Error(), want)
	}
}

func TestParseChunkFromTailData_RejectsOversizedChunkDataSize(t *testing.T) {
	resetY := withWorldY(0, 15)
	defer resetY()

	tail := make([]byte, 0)
	tail = append(tail, encodeHeightmapsNBT()...)
	tail = append(tail, writeVarInt(int32(protocol.MaxRawPacketLen+1))...)

	_, err := ParseChunkFromTailData(0, 0, tail)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDecodeBlockStatesContainer_RejectsOversizedPaletteLen(t *testing.T) {
	resetY := withWorldY(0, 15)
	defer resetY()

	const oversized = 4097
	var section []byte
	section = binary.BigEndian.AppendUint16(section, 0) // block count
	section = append(section, 4)                        // bitsPerEntry=4 (indirect palette)
	section = append(section, writeVarInt(oversized)...)

	_, err := ParseChunkFromTailData(0, 0, buildTailData(section))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDecodeBlockStatesContainer_RejectsOversizedDataArrayLen(t *testing.T) {
	resetY := withWorldY(0, 15)
	defer resetY()

	const oversized = 98305
	var section []byte
	section = binary.BigEndian.AppendUint16(section, 0) // block count
	section = append(section, 9)                        // bitsPerEntry=9 (direct palette)
	section = append(section, writeVarInt(oversized)...)

	_, err := ParseChunkFromTailData(0, 0, buildTailData(section))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestParseChunkFromTailData_BlockStateBitsPerEntryCoercionAndModes(t *testing.T) {
	resetY := withWorldY(0, 15)
	defer resetY()

	tests := []struct {
		name             string
		bits             byte
		palette          []int32
		pattern          []int32
		expectFirstBlock uint16
	}{
		{
			name:             "bits_1_coerced_to_4",
			bits:             1,
			palette:          []int32{0, 9},
			pattern:          alternatingIndices(SectionVolume),
			expectFirstBlock: 0,
		},
		{
			name:             "bits_3_coerced_to_4",
			bits:             3,
			palette:          []int32{0, 9},
			pattern:          alternatingIndices(SectionVolume),
			expectFirstBlock: 0,
		},
		{
			name:             "bits_4_indirect_unchanged",
			bits:             4,
			palette:          []int32{0, 9},
			pattern:          alternatingIndices(SectionVolume),
			expectFirstBlock: 0,
		},
		{
			name:             "bits_9_direct_mode",
			bits:             9,
			pattern:          directPattern(SectionVolume, 257),
			expectFirstBlock: 257,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var section []byte
			switch {
			case tt.bits >= 1 && tt.bits <= 8:
				longs := packEntriesNoSpan(tt.pattern, 4)
				section = encodeSectionIndirect(2048, tt.bits, tt.palette, longs)
			case tt.bits >= 9:
				longs := packEntriesNoSpan(tt.pattern, int(tt.bits))
				section = encodeSectionDirect(2048, tt.bits, longs)
			default:
				t.Fatalf("unsupported test bits: %d", tt.bits)
			}

			chunk, err := ParseChunkFromTailData(0, 0, buildTailData(section))
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if got := chunk.Sections[0].Blocks[0]; got != tt.expectFirstBlock {
				t.Fatalf("first block got %d want %d", got, tt.expectFirstBlock)
			}
			if tt.bits <= 8 {
				if got := chunk.Sections[0].Blocks[1]; got != 9 {
					t.Fatalf("second block got %d want 9", got)
				}
			}
		})
	}
}

func TestParseChunkFromTailData_RejectsInvalidBlockStateBitsPerEntryAbove15(t *testing.T) {
	resetY := withWorldY(0, 15)
	defer resetY()

	section := make([]byte, 0)
	section = binary.BigEndian.AppendUint16(section, 0)
	section = append(section, 16) // invalid bitsPerEntry > 15 for 1.20.4 block states
	section = append(section, writeVarInt(0)...)

	_, err := ParseChunkFromTailData(0, 0, buildTailData(section))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid bits per entry for block states") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseChunkFromTailData_SectionByteConsumptionGuard(t *testing.T) {
	resetY := withWorldY(0, 15)
	defer resetY()

	// bits=4 expects 256 longs for 4096 entries, intentionally provide 255.
	pattern := alternatingIndices(SectionVolume)
	longs := packEntriesNoSpan(pattern, 4)
	longs = longs[:len(longs)-1]
	section := encodeSectionIndirect(2048, 4, []int32{0, 9}, longs)

	_, err := ParseChunkFromTailData(0, 0, buildTailData(section))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "data array len mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func buildTailData(sectionData []byte) []byte {
	tail := make([]byte, 0)
	tail = append(tail, encodeHeightmapsNBT()...)
	tail = append(tail, writeVarInt(int32(len(sectionData)))...)
	tail = append(tail, sectionData...)

	// Remaining fields after chunk data: block entities + light masks/arrays.
	tail = append(tail, writeVarInt(1)...) // block entities count
	tail = append(tail, byte(0))           // packed x/z
	tail = binary.BigEndian.AppendUint16(tail, uint16(64))
	tail = append(tail, writeVarInt(0)...) // block entity type
	tail = append(tail, 0)                 // NBT TAG_End
	tail = append(tail, writeVarInt(0)...) // sky light mask
	tail = append(tail, writeVarInt(0)...) // block light mask
	tail = append(tail, writeVarInt(0)...) // empty sky light mask
	tail = append(tail, writeVarInt(0)...) // empty block light mask
	tail = append(tail, writeVarInt(0)...) // sky light array count
	tail = append(tail, writeVarInt(0)...) // block light array count
	return tail
}

func buildTailDataWithHeightmaps(heightmapsNBT, sectionData []byte) []byte {
	tail := make([]byte, 0)
	tail = append(tail, heightmapsNBT...)
	tail = append(tail, writeVarInt(int32(len(sectionData)))...)
	tail = append(tail, sectionData...)

	// Remaining fields after chunk data: block entities + light masks/arrays.
	tail = append(tail, writeVarInt(1)...) // block entities count
	tail = append(tail, byte(0))           // packed x/z
	tail = binary.BigEndian.AppendUint16(tail, uint16(64))
	tail = append(tail, writeVarInt(0)...) // block entity type
	tail = append(tail, 0)                 // NBT TAG_End
	tail = append(tail, writeVarInt(0)...) // sky light mask
	tail = append(tail, writeVarInt(0)...) // block light mask
	tail = append(tail, writeVarInt(0)...) // empty sky light mask
	tail = append(tail, writeVarInt(0)...) // empty block light mask
	tail = append(tail, writeVarInt(0)...) // sky light array count
	tail = append(tail, writeVarInt(0)...) // block light array count
	return tail
}

func withWorldY(minY, maxY int) func() {
	oldMin, oldMax := MinY, MaxY
	MinY, MaxY = minY, maxY
	return func() {
		MinY, MaxY = oldMin, oldMax
	}
}

func encodeHeightmapsNBT() []byte {
	out := []byte{10, 0, 0} // TAG_Compound, empty name
	out = append(out, 12)   // TAG_Long_Array
	out = binary.BigEndian.AppendUint16(out, uint16(len("MOTION_BLOCKING")))
	out = append(out, []byte("MOTION_BLOCKING")...)
	out = binary.BigEndian.AppendUint32(out, 1)
	out = binary.BigEndian.AppendUint64(out, 0)
	out = append(out, 0) // TAG_End for compound
	return out
}

func encodeHeightmapsNBTWithMotionBlocking(values []int32) []byte {
	out := []byte{10, 0, 0} // TAG_Compound, empty name
	out = append(out, 12)   // TAG_Long_Array
	out = binary.BigEndian.AppendUint16(out, uint16(len("MOTION_BLOCKING")))
	out = append(out, []byte("MOTION_BLOCKING")...)
	longs := packEntriesNoSpan(values, 9)
	out = binary.BigEndian.AppendUint32(out, uint32(len(longs)))
	for _, v := range longs {
		out = binary.BigEndian.AppendUint64(out, v)
	}
	out = append(out, 0) // TAG_End for compound
	return out
}

func encodeSectionSingle(blockCount int16, stateID int32) []byte {
	out := make([]byte, 0)
	out = binary.BigEndian.AppendUint16(out, uint16(blockCount))
	out = append(out, 0) // block states bpe=0
	out = append(out, writeVarInt(stateID)...)
	out = append(out, writeVarInt(0)...)
	out = append(out, 0) // biome bpe=0
	out = append(out, writeVarInt(0)...)
	out = append(out, writeVarInt(0)...)
	return out
}

func encodeSectionIndirect(blockCount int16, bits byte, palette []int32, longs []uint64) []byte {
	out := make([]byte, 0)
	out = binary.BigEndian.AppendUint16(out, uint16(blockCount))
	out = append(out, bits)
	out = append(out, writeVarInt(int32(len(palette)))...)
	for _, p := range palette {
		out = append(out, writeVarInt(p)...)
	}
	out = append(out, writeVarInt(int32(len(longs)))...)
	for _, v := range longs {
		out = binary.BigEndian.AppendUint64(out, v)
	}
	out = append(out, 0) // biome bpe=0
	out = append(out, writeVarInt(0)...)
	out = append(out, writeVarInt(0)...)
	return out
}

func packEntries(entries []int32, bits int) []uint64 {
	longs := make([]uint64, (len(entries)*bits+63)/64)
	for i, v := range entries {
		bitOffset := i * bits
		longIndex := bitOffset / 64
		bitIndex := bitOffset % 64
		value := uint64(v)
		longs[longIndex] |= value << bitIndex
		if bitIndex+bits > 64 {
			highBits := bitIndex + bits - 64
			longs[longIndex+1] |= value >> (bits - highBits)
		}
	}
	return longs
}

func packEntriesNoSpan(entries []int32, bits int) []uint64 {
	if bits <= 0 {
		return nil
	}
	entriesPerLong := 64 / bits
	longCount := (len(entries) + entriesPerLong - 1) / entriesPerLong
	out := make([]uint64, longCount)
	mask := uint64((1 << bits) - 1)
	for i, value := range entries {
		longIndex := i / entriesPerLong
		bitIndex := (i % entriesPerLong) * bits
		out[longIndex] |= (uint64(value) & mask) << bitIndex
	}
	return out
}

func alternatingIndices(count int) []int32 {
	out := make([]int32, count)
	for i := range out {
		if i%2 == 1 {
			out[i] = 1
		}
	}
	return out
}

func directPattern(count int, value int32) []int32 {
	out := make([]int32, count)
	for i := range out {
		out[i] = value
	}
	return out
}

func encodeSectionDirect(blockCount int16, bits byte, longs []uint64) []byte {
	out := make([]byte, 0)
	out = binary.BigEndian.AppendUint16(out, uint16(blockCount))
	out = append(out, bits)
	out = append(out, writeVarInt(int32(len(longs)))...)
	for _, v := range longs {
		out = binary.BigEndian.AppendUint64(out, v)
	}
	out = append(out, 0) // biome bpe=0
	out = append(out, writeVarInt(0)...)
	out = append(out, writeVarInt(0)...)
	return out
}

func writeVarInt(v int32) []byte {
	u := uint32(v)
	out := make([]byte, 0, 5)
	for {
		if (u & ^uint32(0x7F)) == 0 {
			out = append(out, byte(u))
			return out
		}
		out = append(out, byte((u&0x7F)|0x80))
		u >>= 7
	}
}

func TestUnpackEntry_TableDriven_NoBoundaryBleed(t *testing.T) {
	tests := []struct {
		name  string
		bits  int
		count int
	}{
		{name: "bits_5", bits: 5, count: 256},
		{name: "bits_6", bits: 6, count: 256},
		{name: "bits_7", bits: 7, count: 256},
		{name: "bits_9", bits: 9, count: 256},
		{name: "bits_13", bits: 13, count: 256},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := deterministicEntries(tt.bits, tt.count)
			longsA := packEntriesNoSpan(want, tt.bits)
			longsB := packEntriesNoSpan(want, tt.bits)
			if !reflect.DeepEqual(longsA, longsB) {
				t.Fatalf("deterministic long generation mismatch for bits=%d", tt.bits)
			}

			// Assert all entries decode exactly to their source values.
			for i := 0; i < tt.count; i++ {
				got, err := unpackEntry(longsA, tt.bits, i)
				if err != nil {
					t.Fatalf("unpackEntry(bits=%d, index=%d) err=%v", tt.bits, i, err)
				}
				if got != int(want[i]) {
					t.Fatalf("unpackEntry(bits=%d, index=%d)=%d want=%d", tt.bits, i, got, want[i])
				}
			}

			// Target boundary-adjacent indices where reads are most error-prone.
			for _, idx := range boundaryIndices(tt.bits, tt.count) {
				got, err := unpackEntry(longsA, tt.bits, idx)
				if err != nil {
					t.Fatalf("boundary unpackEntry(bits=%d, index=%d) err=%v", tt.bits, idx, err)
				}
				if got != int(want[idx]) {
					t.Fatalf("boundary unpackEntry(bits=%d, index=%d)=%d want=%d", tt.bits, idx, got, want[idx])
				}
			}
		})
	}
}

func deterministicEntries(bits, count int) []int32 {
	mask := int32((1 << bits) - 1)
	out := make([]int32, count)
	for i := 0; i < count; i++ {
		// Deterministic integer mix, then clamp to bits.
		mixed := uint32(i*2654435761) ^ uint32((i+1)*(i+17))
		out[i] = int32(mixed) & mask
	}
	return out
}

func boundaryIndices(bits, count int) []int {
	seen := make(map[int]struct{})
	out := make([]int, 0)
	entriesPerLong := 64 / bits
	for i := 0; i < count; i++ {
		if entriesPerLong == 0 {
			break
		}
		slot := i % entriesPerLong
		if slot == 0 || slot == entriesPerLong-1 {
			if _, ok := seen[i]; !ok {
				out = append(out, i)
				seen[i] = struct{}{}
			}
		}
	}
	return out
}
