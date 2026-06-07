package hpa

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestDebugLogsGated(t *testing.T) {
	oldDebug := DebugLogs
	oldOutput := log.Writer()
	defer func() {
		DebugLogs = oldDebug
		log.SetOutput(oldOutput)
	}()

	var buf bytes.Buffer
	log.SetOutput(&buf)

	DebugLogs = false
	debugf("should not appear %d", 1)
	if buf.Len() != 0 {
		t.Fatalf("debugf must be silent when DebugLogs is false, got %q", buf.String())
	}

	DebugLogs = true
	debugf("should appear %d", 2)
	if !strings.Contains(buf.String(), "should appear 2") {
		t.Fatalf("debugf must log when DebugLogs is enabled, got %q", buf.String())
	}
}
