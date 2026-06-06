package protocol

import (
	"bytes"
	"encoding/binary"
	"math"
	"testing"
)

func TestSetPlayerPositionAndRotationExactBytes(t *testing.T) {
	pkt := &PlayServerboundSetPlayerPositionAndRotationPacket{
		X: 1.5, Y: 64.0, Z: -2.5, Yaw: 90.0, Pitch: -12.0, OnGround: true,
	}
	var buf bytes.Buffer
	if err := pkt.Marshal(NewWriter(&buf)); err != nil {
		t.Fatal(err)
	}
	got := buf.Bytes()

	// Expected: 3 doubles + 2 floats + 1 bool = 8+8+8+4+4+1 = 33 bytes
	if len(got) != 33 {
		t.Fatalf("expected 33 bytes, got %d", len(got))
	}

	// Verify X
	x := math.Float64frombits(binary.BigEndian.Uint64(got[0:8]))
	if x != 1.5 {
		t.Fatalf("X mismatch: %f", x)
	}
	// Verify Y
	y := math.Float64frombits(binary.BigEndian.Uint64(got[8:16]))
	if y != 64.0 {
		t.Fatalf("Y mismatch: %f", y)
	}
	// Verify Z
	z := math.Float64frombits(binary.BigEndian.Uint64(got[16:24]))
	if z != -2.5 {
		t.Fatalf("Z mismatch: %f", z)
	}
	// Yaw
	yaw := math.Float32frombits(binary.BigEndian.Uint32(got[24:28]))
	if yaw != 90.0 {
		t.Fatalf("Yaw mismatch: %f", yaw)
	}
	// Pitch
	pitch := math.Float32frombits(binary.BigEndian.Uint32(got[28:32]))
	if pitch != -12.0 {
		t.Fatalf("Pitch mismatch: %f", pitch)
	}
	// OnGround
	if got[32] != 1 {
		t.Fatalf("OnGround byte mismatch: %d", got[32])
	}

	// Round-trip
	var dec PlayServerboundSetPlayerPositionAndRotationPacket
	if err := dec.Unmarshal(NewReader(bytes.NewReader(got))); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if dec.X != pkt.X || dec.Y != pkt.Y || dec.Z != pkt.Z || dec.Yaw != pkt.Yaw || dec.Pitch != pkt.Pitch || dec.OnGround != pkt.OnGround {
		t.Fatalf("round-trip mismatch: %+v", dec)
	}
}

func TestClickContainerExactBytes(t *testing.T) {
	pkt := &PlayServerboundClickContainerPacket{
		WindowID:     3,
		StateID:      7,
		Slot:         36,
		Button:       0,
		Mode:         0,
		ChangedSlots: nil,
		CarriedItem:  ItemStack{Present: false},
	}
	var buf bytes.Buffer
	if err := pkt.Marshal(NewWriter(&buf)); err != nil {
		t.Fatal(err)
	}
	got := buf.Bytes()

	// WindowID=1 byte, StateID=VarInt(7)=1, Slot=short(36)=2,
	// Button=1, Mode=VarInt(0)=1, ChangedSlots count=VarInt(0)=1,
	// CarriedItem present=1byte(false)
	// Total: 1+1+2+1+1+1+1 = 8 bytes
	if len(got) != 8 {
		t.Fatalf("expected 8 bytes, got %d: %x", len(got), got)
	}
	if got[0] != 3 { // WindowID
		t.Fatalf("WindowID mismatch: %d", got[0])
	}
	if got[1] != 7 { // StateID varint
		t.Fatalf("StateID mismatch: %d", got[1])
	}
	// Slot: big-endian int16 = 0x00, 0x24
	if got[2] != 0x00 || got[3] != 0x24 {
		t.Fatalf("Slot mismatch: %x%x", got[2], got[3])
	}
	if got[4] != 0 { // Button
		t.Fatalf("Button mismatch: %d", got[4])
	}
	if got[5] != 0 { // Mode varint
		t.Fatalf("Mode mismatch: %d", got[5])
	}
	if got[6] != 0 { // ChangedSlots count varint
		t.Fatalf("ChangedSlots count mismatch: %d", got[6])
	}
	if got[7] != 0 { // CarriedItem present=false
		t.Fatalf("CarriedItem present byte mismatch: %d", got[7])
	}
}
