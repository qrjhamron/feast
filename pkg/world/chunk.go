package world

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"sync"

	"github.com/qrjhamron/feast/pkg/protocol"
)

const (
	SectionSize   = 16
	SectionVolume = 16 * 16 * 16
	ChunkWidth    = 16
	ChunkDepth    = 16

	// DefaultMinY/DefaultMaxY match the 1.18+ Overworld build height.
	// These are variables (MinY/MaxY) so other dimensions can be supported by configuring
	// the world package at startup (before parsing chunks).
	DefaultMinY = -64
	DefaultMaxY = 319
	// MaxVanillaStateID is the highest known vanilla 1.20.4 block state ID.
	MaxVanillaStateID = 26643
)

var (
	MinY = DefaultMinY
	MaxY = DefaultMaxY
)

type WorldConfig struct {
	MinY         int
	MaxY         int
	SectionCount int
}

var (
	OverworldConfig = WorldConfig{MinY: -64, MaxY: 319, SectionCount: 24}
	NetherConfig    = WorldConfig{MinY: 0, MaxY: 255, SectionCount: 16}
	EndConfig       = WorldConfig{MinY: 0, MaxY: 255, SectionCount: 16}
)

// SectionCount returns the number of 16-block-tall chunk sections implied by MinY/MaxY.
func SectionCount() int {
	// Inclusive height in blocks.
	return sectionCountFor(WorldConfig{MinY: MinY, MaxY: MaxY})
}

func sectionCountFor(cfg WorldConfig) int {
	if cfg.SectionCount > 0 {
		return cfg.SectionCount
	}
	height := cfg.MaxY - cfg.MinY + 1
	if height <= 0 {
		return 0
	}
	return (height + (SectionSize - 1)) / SectionSize
}

// ChunkSection stores decoded global block state IDs for one 16x16x16 section.
type ChunkSection struct {
	BlockCount int16
	Blocks     [SectionVolume]uint16
}

// Chunk stores decoded section data for one chunk column.
type Chunk struct {
	mu            sync.RWMutex
	X             int32
	Z             int32
	ChunkX        int
	ChunkZ        int
	Config        WorldConfig
	Sections      []ChunkSection
	Light         ChunkLight
	BlockEntities []BlockEntity
	// Entities is kept for compatibility; it mirrors BlockEntities.
	Entities []BlockEntity
	SurfaceY [256]int16
	legacy   map[int]BlockState
}

type BlockState struct {
	Name  string
	Solid bool
	ID    int32
}

var (
	AirBlockState = BlockState{Name: "air", Solid: false, ID: 0}
)

type BlockEntity struct {
	X, Y, Z int
	TypeID  int32
}

// ChunkLight stores raw light update data from the Chunk Data packet tail.
// Arrays are nibble-packed (2048 bytes per section array).
type ChunkLight struct {
	SkyLightMask      []uint64
	BlockLightMask    []uint64
	EmptySkyLightMask []uint64
	EmptyBlockMask    []uint64

	SkyLightArrays   [][]byte
	BlockLightArrays [][]byte
}

// IsSolidBlockState returns whether a global blockstate ID is treated as solid.
func IsSolidBlockState(id int32) bool {
	name := ResolveBlockName(id)
	if name != "" {
		return !isPassableBlockName(name)
	}
	if id == 0 {
		return false
	}
	return true
}

// ParseChunkFromTailData decodes block states from Chunk Data and Update Light TailData.
func ParseChunkFromTailData(chunkX, chunkZ int32, tailData []byte) (*Chunk, error) {
	cfg := WorldConfig{MinY: MinY, MaxY: MaxY, SectionCount: SectionCount()}
	return ParseChunkFromTailDataWithConfig(chunkX, chunkZ, tailData, cfg)
}

// ParseChunkFromTailDataWithConfig decodes block states from Chunk Data and Update Light TailData.
func ParseChunkFromTailDataWithConfig(chunkX, chunkZ int32, tailData []byte, cfg WorldConfig) (*Chunk, error) {
	if cfg.SectionCount == 0 {
		cfg.SectionCount = sectionCountFor(cfg)
	}
	if cfg.SectionCount < 0 {
		return nil, fmt.Errorf("invalid world config section count: %d", cfg.SectionCount)
	}

	r := bytes.NewReader(tailData)
	surface, err := decodeHeightmapsNBT(r, cfg)
	if err != nil {
		return nil, fmt.Errorf("skip heightmaps nbt: %w", err)
	}
	dataSize, _, err := protocol.ReadVarInt(r)
	if err != nil {
		return nil, fmt.Errorf("read chunk data size: %w", err)
	}
	if dataSize < 0 {
		return nil, fmt.Errorf("negative chunk data size: %d", dataSize)
	}
	// Defensive cap: prevents unbounded allocations on malformed chunk packets.
	if dataSize > protocol.MaxRawPacketLen {
		return nil, fmt.Errorf("chunk data size %d exceeds max %d", dataSize, protocol.MaxRawPacketLen)
	}

	data := make([]byte, dataSize)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("read chunk data: %w", err)
	}

	chunk := &Chunk{
		X:        chunkX,
		Z:        chunkZ,
		ChunkX:   int(chunkX),
		ChunkZ:   int(chunkZ),
		Config:   cfg,
		Sections: make([]ChunkSection, cfg.SectionCount),
		SurfaceY: surface,
		legacy:   make(map[int]BlockState),
	}
	if err := decodeChunkSections(data, chunk.Sections, cfg); err != nil {
		return nil, err
	}

	if err := decodeBlockEntities(r, chunk); err != nil {
		return nil, err
	}
	if err := decodeLightTail(r, &chunk.Light); err != nil {
		return nil, err
	}
	if r.Len() != 0 {
		return nil, fmt.Errorf("chunk tail: trailing %d bytes after parse (packet length %d)", r.Len(), len(tailData))
	}

	return chunk, nil
}

func decodeBlockEntities(r *bytes.Reader, chunk *Chunk) error {
	count, _, err := protocol.ReadVarInt(r)
	if err != nil {
		return fmt.Errorf("read block entity count: %w", err)
	}
	if count < 0 {
		return fmt.Errorf("negative block entity count: %d", count)
	}
	if count > 1024 {
		return fmt.Errorf("block entity count too large: %d", count)
	}

	if count == 0 {
		return nil
	}
	chunk.BlockEntities = make([]BlockEntity, 0, count)
	for i := int32(0); i < count; i++ {
		packedXZ, err := readU8(r)
		if err != nil {
			return fmt.Errorf("block entity %d packedXZ: %w", i, err)
		}
		y, err := readInt16(r)
		if err != nil {
			return fmt.Errorf("block entity %d y: %w", i, err)
		}
		typeID, _, err := protocol.ReadVarInt(r)
		if err != nil {
			return fmt.Errorf("block entity %d type id: %w", i, err)
		}
		if err := skipNBT(r); err != nil {
			return fmt.Errorf("block entity %d nbt: %w", i, err)
		}

		localX := int((packedXZ >> 4) & 0x0F)
		localZ := int(packedXZ & 0x0F)
		absX := int(chunk.X)*ChunkWidth + localX
		absZ := int(chunk.Z)*ChunkDepth + localZ
		chunk.BlockEntities = append(chunk.BlockEntities, BlockEntity{
			X:      absX,
			Y:      int(y),
			Z:      absZ,
			TypeID: typeID,
		})
	}
	chunk.Entities = chunk.BlockEntities
	return nil
}

func decodeLightTail(r *bytes.Reader, out *ChunkLight) error {
	var err error
	if out.SkyLightMask, err = readBitSet(r); err != nil {
		return fmt.Errorf("read sky light mask: %w", err)
	}
	if out.BlockLightMask, err = readBitSet(r); err != nil {
		return fmt.Errorf("read block light mask: %w", err)
	}
	if out.EmptySkyLightMask, err = readBitSet(r); err != nil {
		return fmt.Errorf("read empty sky light mask: %w", err)
	}
	if out.EmptyBlockMask, err = readBitSet(r); err != nil {
		return fmt.Errorf("read empty block light mask: %w", err)
	}

	skyCount, _, err := protocol.ReadVarInt(r)
	if err != nil {
		return fmt.Errorf("read sky light array count: %w", err)
	}
	if skyCount < 0 {
		return fmt.Errorf("negative sky light array count: %d", skyCount)
	}
	if skyCount > 4096 {
		return fmt.Errorf("sky light array count too large: %d", skyCount)
	}
	out.SkyLightArrays = make([][]byte, 0, skyCount)
	for i := int32(0); i < skyCount; i++ {
		arr, err := readByteArray(r, 2048)
		if err != nil {
			return fmt.Errorf("read sky light array %d: %w", i, err)
		}
		out.SkyLightArrays = append(out.SkyLightArrays, arr)
	}

	blockCount, _, err := protocol.ReadVarInt(r)
	if err != nil {
		return fmt.Errorf("read block light array count: %w", err)
	}
	if blockCount < 0 {
		return fmt.Errorf("negative block light array count: %d", blockCount)
	}
	if blockCount > 4096 {
		return fmt.Errorf("block light array count too large: %d", blockCount)
	}
	out.BlockLightArrays = make([][]byte, 0, blockCount)
	for i := int32(0); i < blockCount; i++ {
		arr, err := readByteArray(r, 2048)
		if err != nil {
			return fmt.Errorf("read block light array %d: %w", i, err)
		}
		out.BlockLightArrays = append(out.BlockLightArrays, arr)
	}

	return nil
}

func readBitSet(r *bytes.Reader) ([]uint64, error) {
	n, _, err := protocol.ReadVarInt(r)
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, fmt.Errorf("negative bitset length: %d", n)
	}
	if n > 4096 {
		return nil, fmt.Errorf("bitset too large: %d", n)
	}
	if n == 0 {
		return nil, nil
	}
	out := make([]uint64, n)
	for i := int32(0); i < n; i++ {
		v, err := readInt64(r)
		if err != nil {
			return nil, err
		}
		out[i] = uint64(v)
	}
	return out, nil
}

func readByteArray(r *bytes.Reader, wantLen int) ([]byte, error) {
	l, _, err := protocol.ReadVarInt(r)
	if err != nil {
		return nil, err
	}
	if l < 0 {
		return nil, fmt.Errorf("negative byte array length: %d", l)
	}
	if wantLen >= 0 && int(l) != wantLen {
		return nil, fmt.Errorf("unexpected byte array length: got %d want %d", l, wantLen)
	}
	buf := make([]byte, l)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func NewChunk(chunkX, chunkZ int) *Chunk {
	cfg := OverworldConfig
	return &Chunk{
		X:      int32(chunkX),
		Z:      int32(chunkZ),
		ChunkX: chunkX,
		ChunkZ: chunkZ,
		Config: cfg,
		Sections: func() []ChunkSection {
			return make([]ChunkSection, sectionCountFor(cfg))
		}(),
		legacy: make(map[int]BlockState),
	}
}

func (c *Chunk) SetBlock(x, y, z int, block BlockState) bool {
	idx, section, local, ok := sectionIndexWithConfig(x, y, z, c.Config)
	if !ok {
		return false
	}
	if section < 0 || section >= len(c.Sections) {
		return false
	}
	c.mu.Lock()
	c.Sections[section].Blocks[local] = uint16(block.ID)
	if block.Name != "" {
		c.legacy[idx] = block
	}
	c.mu.Unlock()
	return true
}

func (c *Chunk) BlockAt(x, y, z int) (BlockState, bool) {
	idx, section, local, ok := sectionIndexWithConfig(x, y, z, c.Config)
	if !ok {
		return BlockState{}, false
	}
	if section < 0 || section >= len(c.Sections) {
		return BlockState{}, false
	}
	c.mu.RLock()
	if b, ok := c.legacy[idx]; ok {
		c.mu.RUnlock()
		return b, true
	}
	id := int32(c.Sections[section].Blocks[local])
	c.mu.RUnlock()
	return blockStateFromID(id), true
}

// blockNameAt returns only the registry block name at the given LOCAL chunk
// coordinates. Unlike BlockAt it does not compute solidity, so it avoids the
// extra registry lookups in blockStateFromID. The block search scans for name
// matches and only needs the full BlockState on the (rare) match, so this is its
// hot path.
func (c *Chunk) blockNameAt(x, y, z int) (string, bool) {
	idx, section, local, ok := sectionIndexWithConfig(x, y, z, c.Config)
	if !ok {
		return "", false
	}
	if section < 0 || section >= len(c.Sections) {
		return "", false
	}
	c.mu.RLock()
	if b, ok := c.legacy[idx]; ok {
		c.mu.RUnlock()
		return b.Name, true
	}
	id := int32(c.Sections[section].Blocks[local])
	c.mu.RUnlock()
	return ResolveBlockName(id), true
}

func sectionIndex(x, y, z int) (idx int, section int, local int, ok bool) {
	return sectionIndexWithConfig(x, y, z, WorldConfig{MinY: MinY, MaxY: MaxY, SectionCount: SectionCount()})
}

func sectionIndexWithConfig(x, y, z int, cfg WorldConfig) (idx int, section int, local int, ok bool) {
	if cfg.SectionCount == 0 {
		cfg.SectionCount = sectionCountFor(cfg)
	}
	if x < 0 || x >= ChunkWidth || z < 0 || z >= ChunkDepth || y < cfg.MinY || y > cfg.MaxY {
		return 0, 0, 0, false
	}
	section = (y - cfg.MinY) >> 4
	if section < 0 || section >= cfg.SectionCount {
		return 0, 0, 0, false
	}
	localY := y & 0xF
	local = (localY*SectionDepth+z)*SectionWidth + x
	idx = ((y-cfg.MinY)*ChunkDepth+z)*ChunkWidth + x
	return idx, section, local, true
}

const (
	SectionWidth = 16
	SectionDepth = 16
)

func blockStateFromID(id int32) BlockState {
	name := ResolveBlockName(id)
	if name == "" && id == 0 {
		name = "air"
	}
	solid := IsSolidBlockState(id)
	return BlockState{Name: name, Solid: solid, ID: id}
}

func decodeChunkSections(data []byte, sections []ChunkSection, cfg WorldConfig) error {
	expectedSections := sectionCountFor(cfg)
	if len(sections) != expectedSections {
		return fmt.Errorf("chunk data: section array len %d does not match config section count %d", len(sections), expectedSections)
	}

	r := bytes.NewReader(data)
	for i := 0; i < len(sections); i++ {
		sectionStart := r.Len()
		count, err := readInt16(r)
		if err != nil {
			return fmt.Errorf("section %d block count: %w", i, err)
		}
		sections[i].BlockCount = count

		blocks, blockStatesConsumed, err := decodeBlockStatesContainer(r, SectionVolume)
		if err != nil {
			return fmt.Errorf("section %d block states: %w", i, err)
		}
		sections[i].Blocks = blocks

		biomeConsumed, err := skipPalettedContainer(r, 64)
		if err != nil {
			return fmt.Errorf("section %d biomes: %w", i, err)
		}

		consumed := sectionStart - r.Len()
		expected := 2 + blockStatesConsumed + biomeConsumed
		if consumed != expected {
			delta := consumed - expected
			return fmt.Errorf("section %d byte consumption mismatch: consumed=%d expected=%d delta=%d", i, consumed, expected, delta)
		}
	}
	if r.Len() != 0 {
		return fmt.Errorf("chunk data: trailing %d bytes after %d sections", r.Len(), len(sections))
	}
	return nil
}

func decodeBlockStatesContainer(r *bytes.Reader, entries int) ([SectionVolume]uint16, int, error) {
	var out [SectionVolume]uint16
	expectedBytes := 1 // bitsPerEntry byte
	bitsPerEntry, err := readU8(r)
	if err != nil {
		return out, 0, err
	}

	effectiveBits := int(bitsPerEntry)
	indirectPalette := false
	switch {
	case bitsPerEntry == 0:
	case bitsPerEntry >= 1 && bitsPerEntry <= 3:
		effectiveBits = 4
		indirectPalette = true
	case bitsPerEntry >= 4 && bitsPerEntry <= 8:
		indirectPalette = true
	case bitsPerEntry >= 9 && bitsPerEntry <= 15:
	default:
		return out, 0, fmt.Errorf("invalid bits per entry for block states: %d", bitsPerEntry)
	}

	if bitsPerEntry == 0 {
		stateID, stateIDLen, err := protocol.ReadVarInt(r)
		if err != nil {
			return out, 0, fmt.Errorf("read single palette value: %w", err)
		}
		expectedBytes += stateIDLen
		arrayLen, arrayLenBytes, err := protocol.ReadVarInt(r)
		if err != nil {
			return out, 0, fmt.Errorf("read single data array len: %w", err)
		}
		expectedBytes += arrayLenBytes
		if arrayLen != 0 {
			return out, 0, fmt.Errorf("single-valued container data array len must be 0, got %d", arrayLen)
		}
		state := clampStateID(stateID)
		for i := 0; i < entries; i++ {
			out[i] = state
		}
		return out, expectedBytes, nil
	}

	var palette []int32
	if indirectPalette {
		paletteLen, paletteLenBytes, err := protocol.ReadVarInt(r)
		if err != nil {
			return out, 0, fmt.Errorf("read palette len: %w", err)
		}
		expectedBytes += paletteLenBytes
		if paletteLen <= 0 {
			return out, 0, fmt.Errorf("invalid palette len: %d", paletteLen)
		}
		if paletteLen > 4096 {
			return out, 0, fmt.Errorf("palette len too large: %d", paletteLen)
		}
		palette = make([]int32, paletteLen)
		for i := range palette {
			v, vBytes, err := protocol.ReadVarInt(r)
			if err != nil {
				return out, 0, fmt.Errorf("read palette value %d: %w", i, err)
			}
			expectedBytes += vBytes
			palette[i] = v
		}
	}

	longsLen, longsLenBytes, err := protocol.ReadVarInt(r)
	if err != nil {
		return out, 0, fmt.Errorf("read data array len: %w", err)
	}
	expectedBytes += longsLenBytes
	if longsLen < 0 {
		return out, 0, fmt.Errorf("negative data array len: %d", longsLen)
	}
	// Defensive cap: prevents unbounded allocations on malformed section data.
	// 98304 is the maximum longs across 24 sections (4096 longs/section at 64 bpe).
	if longsLen > 98304 {
		return out, 0, fmt.Errorf("data array len too large: %d", longsLen)
	}
	entriesPerLong := 64 / effectiveBits
	if entriesPerLong == 0 {
		return out, 0, fmt.Errorf("invalid effective bits per entry: %d", effectiveBits)
	}
	expectedLongs := (entries + entriesPerLong - 1) / entriesPerLong
	if int(longsLen) != expectedLongs {
		delta := int(longsLen) - expectedLongs
		return out, 0, fmt.Errorf("block states data array len mismatch: got %d want %d (delta=%d)", longsLen, expectedLongs, delta)
	}
	expectedBytes += int(longsLen) * 8
	longs := make([]uint64, longsLen)
	for i := range longs {
		v, err := readInt64(r)
		if err != nil {
			return out, 0, fmt.Errorf("read data long %d: %w", i, err)
		}
		longs[i] = uint64(v)
	}

	for i := 0; i < entries; i++ {
		idx, err := unpackEntry(longs, effectiveBits, i)
		if err != nil {
			return out, 0, err
		}
		if indirectPalette {
			if idx < 0 || idx >= len(palette) {
				return out, 0, fmt.Errorf("palette index out of range: %d", idx)
			}
			out[i] = clampStateID(palette[idx])
		} else {
			out[i] = clampStateID(int32(idx))
		}
	}

	return out, expectedBytes, nil
}

func skipPalettedContainer(r *bytes.Reader, entries int) (int, error) {
	expectedBytes := 1 // bitsPerEntry byte
	bitsPerEntry, err := readU8(r)
	if err != nil {
		return 0, err
	}

	if bitsPerEntry == 0 {
		_, singleValLen, err := protocol.ReadVarInt(r)
		if err != nil {
			return 0, err
		}
		expectedBytes += singleValLen
		arrayLen, arrayLenLen, err := protocol.ReadVarInt(r)
		if err != nil {
			return 0, err
		}
		expectedBytes += arrayLenLen
		if arrayLen != 0 {
			return 0, fmt.Errorf("single-valued container data array len must be 0, got %d", arrayLen)
		}
		return expectedBytes, nil
	}

	if bitsPerEntry <= 8 {
		paletteLen, paletteLenBytes, err := protocol.ReadVarInt(r)
		if err != nil {
			return 0, err
		}
		expectedBytes += paletteLenBytes
		if paletteLen <= 0 {
			return 0, fmt.Errorf("invalid palette len: %d", paletteLen)
		}
		if paletteLen > 4096 {
			return 0, fmt.Errorf("palette len too large: %d", paletteLen)
		}
		for i := int32(0); i < paletteLen; i++ {
			_, itemLen, err := protocol.ReadVarInt(r)
			if err != nil {
				return 0, err
			}
			expectedBytes += itemLen
		}
	}

	longsLen, longsLenBytes, err := protocol.ReadVarInt(r)
	if err != nil {
		return 0, err
	}
	expectedBytes += longsLenBytes
	if longsLen < 0 {
		return 0, fmt.Errorf("negative data array len: %d", longsLen)
	}
	if longsLen > 98304 {
		return 0, fmt.Errorf("data array len too large: %d", longsLen)
	}

	entriesPerLong := 64 / int(bitsPerEntry)
	if entriesPerLong == 0 {
		return 0, fmt.Errorf("invalid bits per entry: %d", bitsPerEntry)
	}
	neededLongs := (entries + entriesPerLong - 1) / entriesPerLong
	if int(longsLen) != neededLongs {
		delta := int(longsLen) - neededLongs
		return 0, fmt.Errorf("paletted container data array len mismatch: got %d want %d (delta=%d)", longsLen, neededLongs, delta)
	}
	expectedBytes += int(longsLen) * 8
	for i := int32(0); i < longsLen; i++ {
		if _, err := readInt64(r); err != nil {
			return 0, err
		}
	}
	return expectedBytes, nil
}

func unpackEntry(longs []uint64, bits, index int) (int, error) {
	if bits == 0 {
		return 0, nil
	}
	entriesPerLong := 64 / bits
	if entriesPerLong == 0 {
		return 0, fmt.Errorf("invalid bits per entry: %d", bits)
	}
	longIndex := index / entriesPerLong
	bitIndex := (index % entriesPerLong) * bits
	if longIndex >= len(longs) {
		return 0, fmt.Errorf("data array too short")
	}

	var mask uint64
	if bits == 64 {
		mask = ^uint64(0)
	} else {
		mask = (uint64(1) << bits) - 1
	}
	return int((longs[longIndex] >> bitIndex) & mask), nil
}

func skipNBT(r *bytes.Reader) error {
	tagType, err := readU8(r)
	if err != nil {
		return err
	}
	if tagType == 0 {
		return nil
	}
	// Since 1.20.2, NBT sent over the network omits the root TAG_Compound name.
	// Older encodings (and some test fixtures) may still include a root name (often empty).
	// To be robust, detect and discard an empty-name prefix for compound roots.
	if tagType == 10 {
		pos, err := r.Seek(0, io.SeekCurrent)
		if err != nil {
			return err
		}
		var peek [2]byte
		if _, err := r.ReadAt(peek[:], pos); err == nil {
			if peek[0] == 0 && peek[1] == 0 {
				var rootNameLen [2]byte
				if _, err := io.ReadFull(r, rootNameLen[:]); err != nil {
					return err
				}
			}
		}
	}
	return skipNBTPayload(r, tagType)
}

func decodeHeightmapsNBT(r *bytes.Reader, cfg WorldConfig) ([256]int16, error) {
	var surface [256]int16
	for i := range surface {
		surface[i] = int16(cfg.MinY - 1)
	}

	tagType, err := readU8(r)
	if err != nil {
		return surface, err
	}
	if tagType == 0 {
		return surface, nil
	}
	if tagType != 10 {
		return surface, fmt.Errorf("heightmaps root is not compound: %d", tagType)
	}

	// Some fixtures/older encodings include an empty root name.
	pos, err := r.Seek(0, io.SeekCurrent)
	if err == nil {
		var peek [2]byte
		if _, err := r.ReadAt(peek[:], pos); err == nil && peek[0] == 0 && peek[1] == 0 {
			var rootNameLen [2]byte
			if _, err := io.ReadFull(r, rootNameLen[:]); err != nil {
				return surface, err
			}
		}
	}

	for {
		t, err := readU8(r)
		if err != nil {
			return surface, err
		}
		if t == 0 {
			return surface, nil
		}
		name, err := readNBTString(r)
		if err != nil {
			return surface, err
		}

		if t == 12 && name == "MOTION_BLOCKING" {
			values, err := readNBTLongArray(r)
			if err != nil {
				return surface, err
			}
			fillSurfaceFromHeightmap(&surface, values, cfg)
			continue
		}
		if err := skipNBTPayload(r, t); err != nil {
			return surface, err
		}
	}
}

func readNBTLongArray(r *bytes.Reader) ([]int64, error) {
	l, err := readInt32(r)
	if err != nil {
		return nil, err
	}
	if l < 0 {
		return nil, fmt.Errorf("negative long array length")
	}
	if l > 4096 {
		return nil, fmt.Errorf("long array too large: %d", l)
	}
	out := make([]int64, l)
	for i := range out {
		v, err := readInt64(r)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

func fillSurfaceFromHeightmap(surface *[256]int16, longs []int64, cfg WorldConfig) {
	const bitsPerEntry = 9
	const entriesPerLong = 64 / bitsPerEntry // compact no-spanning layout
	const mask = uint64(0x1FF)

	for i := 0; i < len(surface); i++ {
		longIndex := i / entriesPerLong
		if longIndex >= len(longs) {
			return
		}
		bitOffset := (i % entriesPerLong) * bitsPerEntry
		value := (uint64(longs[longIndex]) >> bitOffset) & mask
		y := cfg.MinY - 1 + int(value)
		surface[i] = int16(y)
	}
}

func clampStateID(id int32) uint16 {
	if id < 0 || id > MaxVanillaStateID || id > 0xFFFF {
		log.Printf("world: invalid block state ID %d, clamping to air", id)
		return 0
	}
	return uint16(id)
}

func skipNBTPayload(r *bytes.Reader, tagType byte) error {
	switch tagType {
	case 0:
		return nil
	case 1:
		_, err := readU8(r)
		return err
	case 2:
		_, err := readInt16(r)
		return err
	case 3:
		_, err := readInt32(r)
		return err
	case 4:
		_, err := readInt64(r)
		return err
	case 5:
		_, err := readInt32(r)
		return err
	case 6:
		_, err := readInt64(r)
		return err
	case 7:
		l, err := readInt32(r)
		if err != nil {
			return err
		}
		if l < 0 {
			return fmt.Errorf("negative byte array length")
		}
		_, err = io.CopyN(io.Discard, r, int64(l))
		return err
	case 8:
		_, err := readNBTString(r)
		return err
	case 9:
		elemType, err := readU8(r)
		if err != nil {
			return err
		}
		l, err := readInt32(r)
		if err != nil {
			return err
		}
		if l < 0 {
			return fmt.Errorf("negative list length")
		}
		for i := int32(0); i < l; i++ {
			if err := skipNBTPayload(r, elemType); err != nil {
				return err
			}
		}
		return nil
	case 10:
		for {
			t, err := readU8(r)
			if err != nil {
				return err
			}
			if t == 0 {
				return nil
			}
			if _, err := readNBTString(r); err != nil {
				return err
			}
			if err := skipNBTPayload(r, t); err != nil {
				return err
			}
		}
	case 11:
		l, err := readInt32(r)
		if err != nil {
			return err
		}
		if l < 0 {
			return fmt.Errorf("negative int array length")
		}
		_, err = io.CopyN(io.Discard, r, int64(l)*4)
		return err
	case 12:
		l, err := readInt32(r)
		if err != nil {
			return err
		}
		if l < 0 {
			return fmt.Errorf("negative long array length")
		}
		_, err = io.CopyN(io.Discard, r, int64(l)*8)
		return err
	default:
		return fmt.Errorf("unknown nbt tag type: %d", tagType)
	}
}

func readNBTString(r io.Reader) (string, error) {
	var l uint16
	if err := binary.Read(r, binary.BigEndian, &l); err != nil {
		return "", err
	}
	buf := make([]byte, l)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}

func readU8(r io.Reader) (byte, error) {
	var b [1]byte
	_, err := io.ReadFull(r, b[:])
	return b[0], err
}

func readInt16(r io.Reader) (int16, error) {
	var v int16
	err := binary.Read(r, binary.BigEndian, &v)
	return v, err
}

func readInt32(r io.Reader) (int32, error) {
	var v int32
	err := binary.Read(r, binary.BigEndian, &v)
	return v, err
}

func readInt64(r io.Reader) (int64, error) {
	var v int64
	err := binary.Read(r, binary.BigEndian, &v)
	return v, err
}
