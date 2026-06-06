package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Logging verbosity for the advanced_controls runner.
//
//   - ADV_VERBOSE=true    enables compact per-step diagnostic chat lines.
//   - ADV_TRACE_PACKETS=true enables the first few raw movement-packet lines.
//
// Defaults keep chat output compact: a short summary per command, not a line
// per packet/correction/block.

func envBool(key string) bool {
	v := os.Getenv(key)
	if v == "" {
		return false
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false
	}
	return b
}

func advVerbose() bool      { return envBool("ADV_VERBOSE") }
func advTracePackets() bool { return envBool("ADV_TRACE_PACKETS") }

// splitChatLines normalizes a message into individual chat-safe lines. Chat
// cannot contain newlines, and the server rejects lines longer than 256 chars,
// so we split on newlines, drop carriage returns/blank lines, and chunk long
// lines. This is the pure core of sendSafeChat so it can be unit-tested.
func splitChatLines(msg string) []string {
	var out []string
	for _, raw := range strings.Split(msg, "\n") {
		line := strings.TrimSpace(strings.ReplaceAll(raw, "\r", ""))
		if line == "" {
			continue
		}
		for len(line) > 256 {
			out = append(out, line[:256])
			line = line[256:]
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// shouldLogProgress reports whether an item-N progress line should be emitted.
// In verbose mode every item is logged; otherwise only every `every`-th item.
func shouldLogProgress(n, every int) bool {
	if advVerbose() {
		return true
	}
	if every <= 0 {
		every = 5
	}
	return n%every == 0
}

// fmtLine is a tiny helper to keep call sites terse.
func fmtLine(format string, args ...any) string { return fmt.Sprintf(format, args...) }
