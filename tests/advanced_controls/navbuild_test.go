package main

import (
	"strings"
	"testing"

	"github.com/qrjhamron/feast/pkg/feast"
)

func TestParseNavbuildArgs(t *testing.T) {
	tests := []struct {
		msg     string
		x, y, z int
		ok      bool
	}{
		{"!navbuild -198 57 -3", -198, 57, -3, true},
		{"!navbuild 10 64 20", 10, 64, 20, true},
		{"!navbuild 10 64", 0, 0, 0, false},
		{"!navbuild 10 64 bad", 0, 0, 0, false},
		{"!navbuild", 0, 0, 0, false},
	}
	for _, tt := range tests {
		fields := strings.Fields(tt.msg)
		x, y, z, ok := parseNavbuildArgs(fields)
		if ok != tt.ok {
			t.Errorf("parseNavbuildArgs(%q) ok=%v; want %v", tt.msg, ok, tt.ok)
			continue
		}
		if ok && (x != tt.x || y != tt.y || z != tt.z) {
			t.Errorf("parseNavbuildArgs(%q)=(%d,%d,%d); want (%d,%d,%d)", tt.msg, x, y, z, tt.x, tt.y, tt.z)
		}
	}
}

func TestCardinalToward(t *testing.T) {
	tests := []struct {
		feet, target feast.BlockPos
		dx, dz       int
		ok           bool
	}{
		{bp(0, 64, 0), bp(5, 64, 1), 1, 0, true},   // X dominant +
		{bp(0, 64, 0), bp(-5, 64, 1), -1, 0, true}, // X dominant -
		{bp(0, 64, 0), bp(1, 64, 5), 0, 1, true},   // Z dominant +
		{bp(0, 64, 0), bp(1, 64, -5), 0, -1, true}, // Z dominant -
		{bp(3, 64, 3), bp(3, 70, 3), 0, 0, false},  // aligned horizontally
	}
	for _, tt := range tests {
		dx, dz, ok := cardinalToward(tt.feet, tt.target)
		if ok != tt.ok || (ok && (dx != tt.dx || dz != tt.dz)) {
			t.Errorf("cardinalToward(%v,%v)=(%d,%d,%v); want (%d,%d,%v)",
				tt.feet, tt.target, dx, dz, ok, tt.dx, tt.dz, tt.ok)
		}
	}
}

func TestCardinalTowardTieBreaksToX(t *testing.T) {
	// Equal horizontal deltas should step along X (absX >= absZ).
	dx, dz, ok := cardinalToward(bp(0, 64, 0), bp(3, 64, 3))
	if !ok || dx != 1 || dz != 0 {
		t.Fatalf("tie should step +X, got (%d,%d,%v)", dx, dz, ok)
	}
}
