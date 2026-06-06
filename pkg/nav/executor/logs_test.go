package executor

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/qrjhamron/feast/pkg/world"
)

// captureStdout redirects os.Stdout for the duration of fn and returns whatever
// was written. The executor's verbose diagnostics go to stdout via fmt.Printf.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	fn()
	_ = w.Close()
	os.Stdout = old
	return <-done
}

func TestExecutorNoDebugLogsByDefault(t *testing.T) {
	old, had := os.LookupEnv("FEAST_DEBUG")
	os.Unsetenv("FEAST_DEBUG")
	defer func() {
		if had {
			os.Setenv("FEAST_DEBUG", old)
		}
	}()

	out := captureStdout(t, func() {
		_ = Execute(context.Background(), &mockClient{}, world.NewWorld(), mockGoal{satisfied: true}, nil)
	})
	if strings.Contains(out, "[move]") {
		t.Fatalf("expected no [move] debug logs by default, got: %q", out)
	}
}

func TestExecutorDebugLogsWhenEnabled(t *testing.T) {
	old, had := os.LookupEnv("FEAST_DEBUG")
	os.Setenv("FEAST_DEBUG", "true")
	defer func() {
		if had {
			os.Setenv("FEAST_DEBUG", old)
		} else {
			os.Unsetenv("FEAST_DEBUG")
		}
	}()

	out := captureStdout(t, func() {
		_ = Execute(context.Background(), &mockClient{}, world.NewWorld(), mockGoal{satisfied: true}, nil)
	})
	if !strings.Contains(out, "[move]") {
		t.Fatalf("expected [move] debug logs when FEAST_DEBUG=true, got: %q", out)
	}
}
