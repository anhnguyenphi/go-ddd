package testsupport

import (
	"bytes"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// LogRecorder captures an app's log output as JSON lines so tests can assert
// on side effects that are only visible in logs (e.g. the log-only email
// sender). Enable it with WithLogCapture; read it from App.Logs.
type LogRecorder struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write implements io.Writer for the slog JSON handler.
func (r *LogRecorder) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.buf.Write(p)
}

// Lines returns every captured line so far.
func (r *LogRecorder) Lines() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := strings.TrimRight(r.buf.String(), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// Find returns the first captured line matching pattern (a regexp), or "".
func (r *LogRecorder) Find(pattern string) string {
	re := regexp.MustCompile(pattern)
	for _, l := range r.Lines() {
		if re.MatchString(l) {
			return l
		}
	}
	return ""
}

// Await polls until a line matches pattern, failing the test after timeout.
func (r *LogRecorder) Await(t testing.TB, pattern string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if l := r.Find(pattern); l != "" {
			return l
		}
		if time.Now().After(deadline) {
			t.Fatalf("no log line matched %q within %s; captured %d lines", pattern, timeout, len(r.Lines()))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (r *LogRecorder) logger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(r, &slog.HandlerOptions{Level: slog.LevelInfo}))
}
