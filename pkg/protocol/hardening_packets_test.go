package protocol

import (
	"bytes"
	"testing"
)

func TestPlayerActionDigging(t *testing.T) {
	// Test cases for Status, negative block positions, and face
	cases := []struct {
		name     string
		status   int32
		pos      BlockPos
		face     byte
		sequence int32
		expected []byte
	}{
		{
			name:     "start_digging_positive_coords",
			status:   PlayerActionStartDigging,
			pos:      BlockPos{X: 10, Y: 64, Z: 20},
			face:     BlockFaceTop,
			sequence: 1,
			expected: []byte{
				0x00,                                           // Status (VarInt) = 0
				0x00, 0x00, 0x02, 0x80, 0x00, 0x01, 0x40, 0x40, // BlockPos (Long)
				0x01, // Face = 1 (Top)
				0x01, // Sequence (VarInt) = 1
			},
		},
		{
			name:     "finish_digging_negative_coords",
			status:   PlayerActionFinishDigging,
			pos:      BlockPos{X: -5, Y: 60, Z: -12},
			face:     BlockFaceNorth,
			sequence: 42,
			expected: []byte{
				0x02,                                           // Status (VarInt) = 2
				0xff, 0xff, 0xfe, 0xff, 0xff, 0xff, 0x40, 0x3c, // BlockPos (Long)
				0x02, // Face = 2 (North)
				0x2a, // Sequence (VarInt) = 42
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pkt := &PlayServerboundPlayerActionPacket{
				Status:   tc.status,
				Position: tc.pos,
				Face:     tc.face,
				Sequence: tc.sequence,
			}

			var buf bytes.Buffer
			w := NewWriter(&buf)
			if err := pkt.Marshal(w); err != nil {
				t.Fatalf("marshal failed: %v", err)
			}

			out := buf.Bytes()
			if !bytes.Equal(out, tc.expected) {
				t.Fatalf("PlayerAction encoding mismatch\ngot : %x\nwant: %x", out, tc.expected)
			}

			// Test round-trip
			decoded := &PlayServerboundPlayerActionPacket{}
			r := NewReader(bytes.NewReader(out))
			if err := decoded.Unmarshal(r); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}
			if decoded.Status != tc.status ||
				decoded.Position.X != tc.pos.X || decoded.Position.Y != tc.pos.Y || decoded.Position.Z != tc.pos.Z ||
				decoded.Face != tc.face ||
				decoded.Sequence != tc.sequence {
				t.Fatalf("roundtrip mismatch: %+v want %+v", decoded, pkt)
			}
		})
	}
}

func TestSetHeldItem(t *testing.T) {
	cases := []struct {
		name        string
		slot        int16
		shouldError bool
		expected    []byte
	}{
		{"slot_0", 0, false, []byte{0x00, 0x00}},
		{"slot_8", 8, false, []byte{0x00, 0x08}},
		{"invalid_negative", -1, true, nil},
		{"invalid_too_high", 9, true, nil},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pkt := &PlayServerboundSetHeldItemPacket{Slot: tc.slot}

			var buf bytes.Buffer
			w := NewWriter(&buf)
			err := pkt.Marshal(w)
			if tc.shouldError {
				if err == nil {
					t.Fatal("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("marshal failed: %v", err)
			}

			out := buf.Bytes()
			if !bytes.Equal(out, tc.expected) {
				t.Fatalf("SetHeldItem encoding mismatch\ngot : %x\nwant: %x", out, tc.expected)
			}

			// Test round-trip
			decoded := &PlayServerboundSetHeldItemPacket{}
			r := NewReader(bytes.NewReader(out))
			if err := decoded.Unmarshal(r); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}
			if decoded.Slot != tc.slot {
				t.Fatalf("roundtrip slot=%d want %d", decoded.Slot, tc.slot)
			}
		})
	}
}

func TestSetCreativeModeSlot(t *testing.T) {
	cases := []struct {
		name     string
		slot     int16
		item     ItemStack
		expected []byte
	}{
		{
			name: "stone_stack_slot_36",
			slot: 36,
			item: ItemStack{
				Present: true,
				ItemID:  1,
				Count:   64,
				NBT:     []byte{0x00},
			},
			expected: []byte{
				0x00, 0x24, // Slot = 36 (Short)
				0x01, // Present = true (Bool)
				0x01, // Item ID = 1 (VarInt)
				0x40, // Count = 64 (Byte)
				0x00, // NBT tag = 0 (TAG_End)
			},
		},
		{
			name: "empty_stack_slot_36",
			slot: 36,
			item: ItemStack{
				Present: false,
			},
			expected: []byte{
				0x00, 0x24, // Slot = 36 (Short)
				0x00, // Present = false (Bool)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pkt := &PlayServerboundSetCreativeModeSlotPacket{
				Slot: tc.slot,
				Item: tc.item,
			}

			var buf bytes.Buffer
			w := NewWriter(&buf)
			if err := pkt.Marshal(w); err != nil {
				t.Fatalf("marshal failed: %v", err)
			}

			out := buf.Bytes()
			if !bytes.Equal(out, tc.expected) {
				t.Fatalf("SetCreativeModeSlot encoding mismatch\ngot : %x\nwant: %x", out, tc.expected)
			}

			// Test round-trip
			decoded := &PlayServerboundSetCreativeModeSlotPacket{}
			r := NewReader(bytes.NewReader(out))
			if err := decoded.Unmarshal(r); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}
			if decoded.Slot != tc.slot ||
				decoded.Item.Present != tc.item.Present ||
				(tc.item.Present && (decoded.Item.ItemID != tc.item.ItemID || decoded.Item.Count != tc.item.Count)) {
				t.Fatalf("roundtrip mismatch: %+v want %+v", decoded, pkt)
			}
		})
	}
}

func TestUseItemOnPlacement(t *testing.T) {
	cases := []struct {
		name       string
		hand       int32
		pos        BlockPos
		face       byte
		cx, cy, cz float32
		inside     bool
		sequence   int32
		expected   []byte
	}{
		{
			name:     "place_stone_top_face_positive_coords",
			hand:     MainHand,
			pos:      BlockPos{X: 10, Y: 64, Z: 20},
			face:     BlockFaceTop,
			cx:       0.5,
			cy:       1.0,
			cz:       0.5,
			inside:   false,
			sequence: 5,
			expected: []byte{
				0x00,                                           // Hand (VarInt) = 0
				0x00, 0x00, 0x02, 0x80, 0x00, 0x01, 0x40, 0x40, // BlockPos (Long)
				0x01,                   // Face (VarInt) = 1 (Top)
				0x3f, 0x00, 0x00, 0x00, // CursorX = 0.5 (Float)
				0x3f, 0x80, 0x00, 0x00, // CursorY = 1.0 (Float)
				0x3f, 0x00, 0x00, 0x00, // CursorZ = 0.5 (Float)
				0x00, // Inside block = false (Bool)
				0x05, // Sequence (VarInt) = 5
			},
		},
		{
			name:     "place_stone_bottom_face_negative_coords",
			hand:     MainHand,
			pos:      BlockPos{X: -5, Y: 60, Z: -12},
			face:     BlockFaceBottom,
			cx:       0.25,
			cy:       0.0,
			cz:       0.75,
			inside:   true,
			sequence: 99,
			expected: []byte{
				0x00,                                           // Hand (VarInt) = 0
				0xff, 0xff, 0xfe, 0xff, 0xff, 0xff, 0x40, 0x3c, // BlockPos (Long)
				0x00,                   // Face (VarInt) = 0 (Bottom)
				0x3e, 0x80, 0x00, 0x00, // CursorX = 0.25 (Float)
				0x00, 0x00, 0x00, 0x00, // CursorY = 0.0 (Float)
				0x3f, 0x40, 0x00, 0x00, // CursorZ = 0.75 (Float)
				0x01, // Inside block = true (Bool)
				0x63, // Sequence (VarInt) = 99
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pkt := &PlayServerboundUseItemOnPacket{
				Hand:        tc.hand,
				Position:    tc.pos,
				Face:        tc.face,
				CursorX:     tc.cx,
				CursorY:     tc.cy,
				CursorZ:     tc.cz,
				InsideBlock: tc.inside,
				Sequence:    tc.sequence,
			}

			var buf bytes.Buffer
			w := NewWriter(&buf)
			if err := pkt.Marshal(w); err != nil {
				t.Fatalf("marshal failed: %v", err)
			}

			out := buf.Bytes()
			if !bytes.Equal(out, tc.expected) {
				t.Fatalf("UseItemOn encoding mismatch\ngot : %x\nwant: %x", out, tc.expected)
			}

			// Test round-trip
			decoded := &PlayServerboundUseItemOnPacket{}
			r := NewReader(bytes.NewReader(out))
			if err := decoded.Unmarshal(r); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}
			if decoded.Hand != tc.hand ||
				decoded.Position.X != tc.pos.X || decoded.Position.Y != tc.pos.Y || decoded.Position.Z != tc.pos.Z ||
				decoded.Face != tc.face ||
				decoded.CursorX != tc.cx || decoded.CursorY != tc.cy || decoded.CursorZ != tc.cz ||
				decoded.InsideBlock != tc.inside ||
				decoded.Sequence != tc.sequence {
				t.Fatalf("roundtrip mismatch: %+v want %+v", decoded, pkt)
			}
		})
	}
}

func TestBlockUpdatePacket(t *testing.T) {
	cases := []struct {
		name     string
		pos      BlockPos
		stateID  int32
		expected []byte
	}{
		{
			name:    "update_to_stone_positive_coords",
			pos:     BlockPos{X: 10, Y: 64, Z: 20},
			stateID: 1, // stone
			expected: []byte{
				0x00, 0x00, 0x02, 0x80, 0x00, 0x01, 0x40, 0x40, // BlockPos (Long)
				0x01, // StateID (VarInt) = 1
			},
		},
		{
			name:    "update_to_air_negative_coords",
			pos:     BlockPos{X: -5, Y: 60, Z: -12},
			stateID: 0, // air
			expected: []byte{
				0xff, 0xff, 0xfe, 0xff, 0xff, 0xff, 0x40, 0x3c, // BlockPos (Long)
				0x00, // StateID (VarInt) = 0
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pkt := &PlayClientboundBlockUpdatePacket{
				Position: tc.pos,
				StateID:  tc.stateID,
			}

			var buf bytes.Buffer
			w := NewWriter(&buf)
			if err := pkt.Marshal(w); err != nil {
				t.Fatalf("marshal failed: %v", err)
			}

			out := buf.Bytes()
			if !bytes.Equal(out, tc.expected) {
				t.Fatalf("BlockUpdate encoding mismatch\ngot : %x\nwant: %x", out, tc.expected)
			}

			// Test round-trip
			decoded := &PlayClientboundBlockUpdatePacket{}
			r := NewReader(bytes.NewReader(out))
			if err := decoded.Unmarshal(r); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}
			if decoded.Position.X != tc.pos.X || decoded.Position.Y != tc.pos.Y || decoded.Position.Z != tc.pos.Z ||
				decoded.StateID != tc.stateID {
				t.Fatalf("roundtrip mismatch: %+v want %+v", decoded, pkt)
			}
		})
	}
}

func TestInventoryWireLayout(t *testing.T) {
	// 1. Container content with multiple slots
	t.Run("container_content_multiple_slots", func(t *testing.T) {
		pkt := &PlayClientboundSetContainerContentPacket{
			WindowID:  1,
			StateID:   2,
			SlotCount: 2,
			Slots: []ItemStack{
				{Present: true, ItemID: 1, Count: 64, NBT: []byte{0x00}},        // stone
				{Present: true, ItemID: 999, Count: 1, NBT: []byte{0x0a, 0x00}}, // unknown item ID with NBT
			},
			CarriedItem: ItemStack{Present: false},
		}

		var buf bytes.Buffer
		w := NewWriter(&buf)
		if err := pkt.Marshal(w); err != nil {
			t.Fatalf("marshal failed: %v", err)
		}

		decoded := &PlayClientboundSetContainerContentPacket{}
		r := NewReader(bytes.NewReader(buf.Bytes()))
		if err := decoded.Unmarshal(r); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		if decoded.WindowID != pkt.WindowID || decoded.StateID != pkt.StateID || decoded.SlotCount != pkt.SlotCount {
			t.Fatalf("mismatch: %+v want %+v", decoded, pkt)
		}
		if len(decoded.Slots) != 2 {
			t.Fatalf("expected 2 slots, got %d", len(decoded.Slots))
		}
	})

	// 2. Set slot update with unknown item ID and NBT-present item stack
	t.Run("set_slot_unknown_nbt", func(t *testing.T) {
		pkt := &PlayClientboundSetContainerSlotPacket{
			WindowID: 0,
			StateID:  5,
			Slot:     36,
			Item: ItemStack{
				Present: true,
				ItemID:  9999,
				Count:   1,
				NBT:     []byte{0x0a, 0x00}, // TAG_Compound, TAG_End
			},
		}

		var buf bytes.Buffer
		w := NewWriter(&buf)
		if err := pkt.Marshal(w); err != nil {
			t.Fatalf("marshal failed: %v", err)
		}

		decoded := &PlayClientboundSetContainerSlotPacket{}
		r := NewReader(bytes.NewReader(buf.Bytes()))
		if err := decoded.Unmarshal(r); err != nil {
			t.Fatalf("unmarshal failed: %v", err)
		}

		if decoded.WindowID != pkt.WindowID || decoded.StateID != pkt.StateID || decoded.Slot != pkt.Slot {
			t.Fatalf("mismatch: %+v want %+v", decoded, pkt)
		}
		if !decoded.Item.Present || decoded.Item.ItemID != 9999 || decoded.Item.Count != 1 {
			t.Fatalf("item mismatch: %+v", decoded.Item)
		}
		if !bytes.Equal(decoded.Item.NBT, pkt.Item.NBT) {
			t.Fatalf("NBT mismatch: %x want %x", decoded.Item.NBT, pkt.Item.NBT)
		}
	})
}

func TestEntityMetadataDecoding(t *testing.T) {
	// 1. Test case for known metadata
	t.Run("known_metadata", func(t *testing.T) {
		var buf bytes.Buffer
		w := NewWriter(&buf)
		_ = w.WriteByte(0)    // Index 0
		_ = w.WriteVarInt(0)  // Type 0 (Byte)
		_ = w.WriteByte(0x7F) // Value
		_ = w.WriteByte(6)    // Index 6
		_ = w.WriteVarInt(20) // Type 20 (Pose / VarInt)
		_ = w.WriteVarInt(2)  // Pose = Sneaking (2)
		_ = w.WriteByte(0xFF) // End marker

		r := NewReader(bytes.NewReader(buf.Bytes()))
		entries, err := ReadEntityMetadata(r)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(entries) != 2 {
			t.Fatalf("expected 2 entries, got %d", len(entries))
		}
		if entries[0].Index != 0 || entries[0].TypeID != 0 || entries[0].Value.(byte) != 0x7F {
			t.Errorf("unexpected entry 0: %+v", entries[0])
		}
		if entries[1].Index != 6 || entries[1].TypeID != 20 || entries[1].Value.(int32) != 2 {
			t.Errorf("unexpected entry 1: %+v", entries[1])
		}
	})

	// 2. Test case for unknown metadata type (should skip safely or return error)
	t.Run("unknown_metadata_type", func(t *testing.T) {
		var buf bytes.Buffer
		w := NewWriter(&buf)
		_ = w.WriteByte(0)    // Index 0
		_ = w.WriteVarInt(99) // Type 99 (Unknown)
		_ = w.WriteByte(0xFF) // End marker

		r := NewReader(bytes.NewReader(buf.Bytes()))
		_, err := ReadEntityMetadata(r)
		if err == nil {
			t.Fatalf("expected error for unknown metadata type, got nil")
		}
	})

	// 3. Test case for malformed metadata (no end marker / truncated)
	t.Run("malformed_metadata_truncated", func(t *testing.T) {
		var buf bytes.Buffer
		w := NewWriter(&buf)
		_ = w.WriteByte(0)   // Index 0
		_ = w.WriteVarInt(0) // Type 0 (Byte)
		// Missing value and end marker

		r := NewReader(bytes.NewReader(buf.Bytes()))
		_, err := ReadEntityMetadata(r)
		if err == nil {
			t.Fatalf("expected error for malformed truncated metadata, got nil")
		}
	})
}
