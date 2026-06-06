package main

import (
	"os"
	"strings"
	"testing"
)

func TestSplitChatLinesBasic(t *testing.T) {
	lines := splitChatLines("hello world")
	if len(lines) != 1 || lines[0] != "hello world" {
		t.Fatalf("unexpected lines: %#v", lines)
	}
}

func TestSplitChatLinesNoNewlines(t *testing.T) {
	in := "line one\nline two\r\nline three"
	lines := splitChatLines(in)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d (%#v)", len(lines), lines)
	}
	for _, l := range lines {
		if strings.ContainsAny(l, "\n\r") {
			t.Fatalf("chat line must not contain newlines: %q", l)
		}
	}
}

func TestSplitChatLinesDropsBlank(t *testing.T) {
	lines := splitChatLines("\n\n   \n\t\n")
	if len(lines) != 0 {
		t.Fatalf("expected no lines from blank input, got %#v", lines)
	}
}

func TestSplitChatLinesChunksLongLine(t *testing.T) {
	long := strings.Repeat("a", 600)
	lines := splitChatLines(long)
	if len(lines) != 3 {
		t.Fatalf("expected 3 chunks (256+256+88), got %d", len(lines))
	}
	for _, l := range lines {
		if len(l) > 256 {
			t.Fatalf("chunk exceeds 256 chars: %d", len(l))
		}
	}
}

func TestEnvBoolParsing(t *testing.T) {
	tests := []struct {
		val  string
		want bool
	}{
		{"", false},
		{"true", true},
		{"1", true},
		{"false", false},
		{"0", false},
		{"garbage", false},
	}
	for _, tt := range tests {
		os.Unsetenv("ADV_TEST_FLAG")
		if tt.val != "" {
			os.Setenv("ADV_TEST_FLAG", tt.val)
		}
		if got := envBool("ADV_TEST_FLAG"); got != tt.want {
			t.Errorf("envBool(%q)=%v; want %v", tt.val, got, tt.want)
		}
	}
	os.Unsetenv("ADV_TEST_FLAG")
}

func TestVerboseFlagParsing(t *testing.T) {
	os.Setenv("ADV_VERBOSE", "true")
	defer os.Unsetenv("ADV_VERBOSE")
	if !advVerbose() {
		t.Fatal("ADV_VERBOSE=true should enable verbose")
	}
	os.Setenv("ADV_VERBOSE", "false")
	if advVerbose() {
		t.Fatal("ADV_VERBOSE=false should disable verbose")
	}
}

func TestShouldLogProgressAggregatesByDefault(t *testing.T) {
	os.Setenv("ADV_VERBOSE", "false")
	defer os.Unsetenv("ADV_VERBOSE")

	// Default: only every 5th item logs.
	logged := 0
	for n := 1; n <= 12; n++ {
		if shouldLogProgress(n, 5) {
			logged++
		}
	}
	if logged != 2 { // n=5,10
		t.Fatalf("default aggregation logged %d of 12; want 2", logged)
	}
}

func TestShouldLogProgressVerboseLogsEvery(t *testing.T) {
	os.Setenv("ADV_VERBOSE", "true")
	defer os.Unsetenv("ADV_VERBOSE")

	logged := 0
	for n := 1; n <= 12; n++ {
		if shouldLogProgress(n, 5) {
			logged++
		}
	}
	if logged != 12 {
		t.Fatalf("verbose mode logged %d of 12; want all 12", logged)
	}
}
