package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/qrjhamron/feast/pkg/feast"
)

func TestCleanChatMessage(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"!help", "!help"},
		{"<Bob>: !pos", "!pos"},
		{"[Server] !find dirt 5 32", "!find dirt 5 32"},
		{"normal chat message", ""},
		{"   !inv  ", "!inv"},
		{"!stop", "!stop"},
	}

	for _, tt := range tests {
		got := cleanChatMessage(tt.input)
		if got != tt.expected {
			t.Errorf("cleanChatMessage(%q) = %q; expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestResolveBlockName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"oak", "oak_log"},
		{"wood", "oak_log"},
		{"log", "oak_log"},
		{"grass", "grass_block"},
		{"dirt", "dirt"},
		{"minecraft:stone", "stone"},
	}

	for _, tt := range tests {
		got := resolveBlockName(tt.input)
		if got != tt.expected {
			t.Errorf("resolveBlockName(%q) = %q; expected %q", tt.input, got, tt.expected)
		}
	}
}

func TestParseFindArgs(t *testing.T) {
	tests := []struct {
		msg       string
		expBlock  string
		expCount  int
		expRadius int
		expOk     bool
	}{
		{"!find stone", "stone", 1, 64, true},
		{"!find stone 15 100", "stone", 15, 100, true},
		{"!find stone 0 0", "stone", 1, 1, true},        // Clamped
		{"!find stone 100 200", "stone", 20, 128, true}, // Clamped
		{"!find", "", 0, 0, false},
		{"!find stone bad_count", "", 0, 0, false},
	}

	for _, tt := range tests {
		block, count, radius, ok := parseFindArgs(tt.msg)
		if ok != tt.expOk {
			t.Errorf("parseFindArgs(%q) ok = %v; expected %v", tt.msg, ok, tt.expOk)
			continue
		}
		if ok {
			if block != tt.expBlock || count != tt.expCount || radius != tt.expRadius {
				t.Errorf("parseFindArgs(%q) = (%q, %d, %d); expected (%q, %d, %d)",
					tt.msg, block, count, radius, tt.expBlock, tt.expCount, tt.expRadius)
			}
		}
	}
}

func TestParseNavArgs(t *testing.T) {
	tests := []struct {
		msg       string
		expBlock  string
		expX      int
		expY      int
		expZ      int
		expCoord  bool
		expRadius int
		expOk     bool
	}{
		{"!nav dirt", "dirt", 0, 0, 0, false, 64, true},
		{"!nav dirt 32", "dirt", 0, 0, 0, false, 32, true},
		{"!nav -150 64 -40", "", -150, 64, -40, true, 0, true},
		{"!nav", "", 0, 0, 0, false, 0, false},
	}

	for _, tt := range tests {
		fields := strings.Fields(tt.msg)
		block, x, y, z, isCoord, radius, ok := parseNavArgs(fields)
		if ok != tt.expOk {
			t.Errorf("parseNavArgs(%q) ok = %v; expected %v", tt.msg, ok, tt.expOk)
			continue
		}
		if ok {
			if isCoord != tt.expCoord {
				t.Errorf("parseNavArgs(%q) isCoord = %v; expected %v", tt.msg, isCoord, tt.expCoord)
			}
			if isCoord {
				if x != tt.expX || y != tt.expY || z != tt.expZ {
					t.Errorf("parseNavArgs(%q) coord = (%d,%d,%d); expected (%d,%d,%d)",
						tt.msg, x, y, z, tt.expX, tt.expY, tt.expZ)
				}
			} else {
				if block != tt.expBlock || radius != tt.expRadius {
					t.Errorf("parseNavArgs(%q) block = %s, radius = %d; expected %s, %d",
						tt.msg, block, radius, tt.expBlock, tt.expRadius)
				}
			}
		}
	}
}

func TestParseHPAArgs(t *testing.T) {
	tests := []struct {
		msg   string
		expX  int
		expY  int
		expZ  int
		expOk bool
	}{
		{"!hpa -100 64 -100", -100, 64, -100, true},
		{"!hpa -100 64", 0, 0, 0, false},
		{"!hpa -100 64 bad_z", 0, 0, 0, false},
	}

	for _, tt := range tests {
		fields := strings.Fields(tt.msg)
		x, y, z, ok := parseHPAArgs(fields)
		if ok != tt.expOk {
			t.Errorf("parseHPAArgs(%q) ok = %v; expected %v", tt.msg, ok, tt.expOk)
			continue
		}
		if ok {
			if x != tt.expX || y != tt.expY || z != tt.expZ {
				t.Errorf("parseHPAArgs(%q) = (%d,%d,%d); expected (%d,%d,%d)",
					tt.msg, x, y, z, tt.expX, tt.expY, tt.expZ)
			}
		}
	}
}

func TestParseMoveArgs(t *testing.T) {
	tests := []struct {
		msg    string
		expDst int
		expOk  bool
	}{
		{"!move 100", 100, true},
		{"!move 200", 150, true}, // clamped
		{"!move -10", 1, true},   // clamped
		{"!move", 0, false},
		{"!move bad", 0, false},
	}

	for _, tt := range tests {
		fields := strings.Fields(tt.msg)
		dst, ok := parseMoveArgs(fields)
		if ok != tt.expOk {
			t.Errorf("parseMoveArgs(%q) ok = %v; expected %v", tt.msg, ok, tt.expOk)
			continue
		}
		if ok && dst != tt.expDst {
			t.Errorf("parseMoveArgs(%q) = %d; expected %d", tt.msg, dst, tt.expDst)
		}
	}
}

func TestParseSwimArgs(t *testing.T) {
	tests := []struct {
		msg       string
		expRadius int
		expOk     bool
	}{
		{"!swim", 64, true},
		{"!swim 32", 32, true},
		{"!swim 200", 128, true}, // clamped
		{"!swim -10", 1, true},   // clamped
	}

	for _, tt := range tests {
		fields := strings.Fields(tt.msg)
		radius, ok := parseSwimArgs(fields)
		if ok != tt.expOk {
			t.Errorf("parseSwimArgs(%q) ok = %v; expected %v", tt.msg, ok, tt.expOk)
			continue
		}
		if ok && radius != tt.expRadius {
			t.Errorf("parseSwimArgs(%q) = %d; expected %d", tt.msg, radius, tt.expRadius)
		}
	}
}

func TestParseScaffoldArgs(t *testing.T) {
	tests := []struct {
		msg    string
		expLen int
		expDir string
		expOk  bool
	}{
		{"!scaffold 10", 10, "forward", true},
		{"!scaffold 5 north", 5, "north", true},
		{"!scaffold 5 east", 5, "east", true},
		{"!scaffold 5 south", 5, "south", true},
		{"!scaffold 5 west", 5, "west", true},
		{"!scaffold 5 forward", 5, "forward", true},
		{"!scaffold 100", 32, "forward", true}, // clamped
		{"!scaffold -5", 1, "forward", true},   // clamped
		{"!scaffold", 0, "", false},
		{"!scaffold bad", 0, "", false},
		{"!scaffold 5 invalid", 0, "", false}, // bad direction
	}

	for _, tt := range tests {
		fields := strings.Fields(tt.msg)
		length, dir, ok := parseScaffoldArgs(fields)
		if ok != tt.expOk {
			t.Errorf("parseScaffoldArgs(%q) ok = %v; expected %v", tt.msg, ok, tt.expOk)
			continue
		}
		if ok && length != tt.expLen {
			t.Errorf("parseScaffoldArgs(%q) length = %d; expected %d", tt.msg, length, tt.expLen)
		}
		if ok && dir != tt.expDir {
			t.Errorf("parseScaffoldArgs(%q) dir = %q; expected %q", tt.msg, dir, tt.expDir)
		}
	}
}

func TestParseBreakNArgs(t *testing.T) {
	tests := []struct {
		msg       string
		expBlock  string
		expCount  int
		expRadius int
		expOk     bool
	}{
		{"!breakn stone 10", "stone", 10, 32, true},
		{"!breakn stone 10 64", "stone", 10, 64, true},
		{"!breakn stone 100 200", "stone", 50, 128, true}, // clamped
		{"!breakn stone", "", 0, 0, false},
		{"!breakn", "", 0, 0, false},
	}

	for _, tt := range tests {
		fields := strings.Fields(tt.msg)
		block, count, radius, ok := parseBreakNArgs(fields)
		if ok != tt.expOk {
			t.Errorf("parseBreakNArgs(%q) ok = %v; expected %v", tt.msg, ok, tt.expOk)
			continue
		}
		if ok {
			if block != tt.expBlock || count != tt.expCount || radius != tt.expRadius {
				t.Errorf("parseBreakNArgs(%q) = (%q, %d, %d); expected (%q, %d, %d)",
					tt.msg, block, count, radius, tt.expBlock, tt.expCount, tt.expRadius)
			}
		}
	}
}

func TestParseBuild10Args(t *testing.T) {
	tests := []struct {
		msg      string
		expBlock string
		expOk    bool
	}{
		{"!build10 stone", "stone", true},
		{"!build10", "", false},
	}

	for _, tt := range tests {
		fields := strings.Fields(tt.msg)
		block, ok := parseBuild10Args(fields)
		if ok != tt.expOk {
			t.Errorf("parseBuild10Args(%q) ok = %v; expected %v", tt.msg, ok, tt.expOk)
			continue
		}
		if ok && block != tt.expBlock {
			t.Errorf("parseBuild10Args(%q) = %s; expected %s", tt.msg, block, tt.expBlock)
		}
	}
}

func TestStopCommandParse(t *testing.T) {
	msg := "!stop"
	fields := strings.Fields(msg)
	if len(fields) == 0 || fields[0] != "!stop" {
		t.Errorf("expected !stop command, got %v", fields)
	}
}

func TestGenerateBuild10Coordinates(t *testing.T) {
	coords := generateBuild10Coordinates(10, 64, 20)
	if len(coords) != 100 {
		t.Fatalf("expected 100 coordinates, got %d", len(coords))
	}

	// Verify all on consistent Y level
	for _, c := range coords {
		if c.Y != 64 {
			t.Errorf("expected Y level 64, got %d", c.Y)
		}
	}
}

func TestGenerateScaffoldTarget(t *testing.T) {
	tests := []struct {
		yaw     float32
		expDx   int
		expDz   int
		expFace feast.Direction
	}{
		{0.0, 0, 1, feast.FaceSouth},
		{90.0, -1, 0, feast.FaceWest},
		{180.0, 0, -1, feast.FaceNorth},
		{270.0, 1, 0, feast.FaceEast},
	}

	for _, tt := range tests {
		target, face, dx, dz := generateScaffoldTarget(0, 10, 0, tt.yaw)
		if dx != tt.expDx || dz != tt.expDz {
			t.Errorf("yaw %f: expected delta (%d,%d), got (%d,%d)", tt.yaw, tt.expDx, tt.expDz, dx, dz)
		}
		if face != tt.expFace {
			t.Errorf("yaw %f: expected face %d, got %d", tt.yaw, tt.expFace, face)
		}
		if target.Y != 9 {
			t.Errorf("yaw %f: expected target Y 9, got %d", tt.yaw, target.Y)
		}
	}
}

func TestSortCoordinatesByDistance(t *testing.T) {
	coords := []feast.BlockPos{
		{X: 10, Y: 64, Z: 10},
		{X: 0, Y: 64, Z: 0},
		{X: 5, Y: 64, Z: 5},
	}

	sortCoordinatesByDistance(coords, 0.0, 64.0, 0.0)

	expected := []feast.BlockPos{
		{X: 0, Y: 64, Z: 0},
		{X: 5, Y: 64, Z: 5},
		{X: 10, Y: 64, Z: 10},
	}

	if !reflect.DeepEqual(coords, expected) {
		t.Errorf("expected sorted coordinates %v, got %v", expected, coords)
	}
}

func TestBusyLockBehavior(t *testing.T) {
	isCommandRunning = false
	if !acquireCommandLock() {
		t.Fatal("expected to acquire lock")
	}
	if acquireCommandLock() {
		t.Fatal("expected NOT to acquire lock while running")
	}
	releaseCommandLock()
	if !acquireCommandLock() {
		t.Fatal("expected to acquire lock after release")
	}
	releaseCommandLock()
}
