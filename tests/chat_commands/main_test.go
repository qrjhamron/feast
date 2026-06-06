package main

import (
	"testing"
)

func TestCleanChatMessage(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"!help", "!help"},
		{"<Alice>: !pos", "!pos"},
		{"[Server] !goto dirt 10", "!goto dirt 10"},
		{"just chat message", ""},
		{"   !inv  ", "!inv"},
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
		{"dirt", "dirt"},
		{"minecraft:dirt", "dirt"},
		{"oak", "oak_log"},
		{"wood", "oak_log"},
		{"log", "oak_log"},
		{"grass", "grass_block"},
		{"stone", "stone"},
		{"stone_bricks", "stone_bricks"},
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
		{"!find dirt", "dirt", 1, 64, true},
		{"!find stone 5 32", "stone", 5, 32, true},
		{"!find", "", 0, 0, false},
		{"!find oak bad_count", "", 0, 0, false},
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
