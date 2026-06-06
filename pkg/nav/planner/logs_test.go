package planner

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"
)

// TestPlannerDebugLogsGated verifies planner diagnostics are silent unless
// DebugLogs is explicitly enabled, so production planning never floods logs.
func TestPlannerDebugLogsGated(t *testing.T) {
	if DebugLogs {
		t.Fatal("DebugLogs must default to false")
	}
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	debugf("should not appear %d", 1)
	if buf.Len() != 0 {
		t.Fatalf("debugf must be silent when DebugLogs is false, got %q", buf.String())
	}

	DebugLogs = true
	debugf("should appear %d", 2)
	DebugLogs = false
	if !strings.Contains(buf.String(), "should appear 2") {
		t.Fatalf("debugf must log when DebugLogs is enabled, got %q", buf.String())
	}
}
