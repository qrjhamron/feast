package protocol

// BlockPos encodes/decodes the packed Block Position format used in many packets.
// See: https://minecraft.wiki/w/Java_Edition_protocol#Position
type BlockPos struct {
	X int32
	Y int32
	Z int32
}

func readBlockPos(r *Reader) (BlockPos, error) {
	v, err := r.ReadLong()
	if err != nil {
		return BlockPos{}, err
	}

	ux := int32(uint64(v) >> 38)
	uz := int32((uint64(v) >> 12) & 0x3FFFFFF)
	uy := int32(uint64(v) & 0xFFF)

	// Sign extend.
	if ux >= 1<<25 {
		ux -= 1 << 26
	}
	if uz >= 1<<25 {
		uz -= 1 << 26
	}
	if uy >= 1<<11 {
		uy -= 1 << 12
	}

	return BlockPos{X: ux, Y: uy, Z: uz}, nil
}

func writeBlockPos(w *Writer, p BlockPos) error {
	x := int64(p.X) & 0x3FFFFFF
	z := int64(p.Z) & 0x3FFFFFF
	y := int64(p.Y) & 0xFFF
	packed := (x << 38) | (z << 12) | y
	return w.WriteLong(packed)
}

// ChunkSectionPos encodes/decodes the packed chunk section position used by
// Update Section Blocks. It is chunkX, sectionY, chunkZ (not world coords).
type ChunkSectionPos struct {
	X int32
	Y int32
	Z int32
}

func readChunkSectionPos(r *Reader) (ChunkSectionPos, error) {
	v, err := r.ReadLong()
	if err != nil {
		return ChunkSectionPos{}, err
	}

	ux := int32(uint64(v) >> 42)
	uz := int32((uint64(v) >> 20) & 0x3FFFFF)
	uy := int32(uint64(v) & 0xFFFFF)

	// Sign extend.
	if ux >= 1<<21 {
		ux -= 1 << 22
	}
	if uz >= 1<<21 {
		uz -= 1 << 22
	}
	if uy >= 1<<19 {
		uy -= 1 << 20
	}

	return ChunkSectionPos{X: ux, Y: uy, Z: uz}, nil
}

func writeChunkSectionPos(w *Writer, p ChunkSectionPos) error {
	x := int64(p.X) & 0x3FFFFF
	z := int64(p.Z) & 0x3FFFFF
	y := int64(p.Y) & 0xFFFFF
	packed := (x << 42) | (z << 20) | y
	return w.WriteLong(packed)
}
