package obs_test

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/obs"
)

// errCloser 任意のエラーを返すフェイクio.Closerです。
type errCloser struct {
	err error
}

func (c errCloser) Close() error { return c.err }

func TestCloseAndLog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		closeErr  error
		wantLog   bool
		wantLevel string
	}{
		{name: "no error logs nothing", closeErr: nil, wantLog: false},
		{name: "error is logged at warn", closeErr: errors.New("boom"), wantLog: true, wantLevel: "WARN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

			obs.CloseAndLog(logger, errCloser{err: tt.closeErr}, "closing feeds file")

			got := buf.String()
			if tt.wantLog {
				if !strings.Contains(got, "closing feeds file") {
					t.Fatalf("log got %q want it to contain context message", got)
				}
				if !strings.Contains(got, tt.wantLevel) {
					t.Fatalf("log got %q want level %q", got, tt.wantLevel)
				}
				if !strings.Contains(got, "boom") {
					t.Fatalf("log got %q want it to contain the close error", got)
				}
			} else if got != "" {
				t.Fatalf("log got %q want empty", got)
			}
		})
	}
}

func TestWriteAndLog(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	var sink bytes.Buffer
	n, err := obs.WriteAndLog(logger, &sink, []byte("payload"), "writing response")
	if err != nil {
		t.Fatalf("WriteAndLog returned error: %v", err)
	}
	if n != len("payload") {
		t.Fatalf("written got %d want %d", n, len("payload"))
	}
	if sink.String() != "payload" {
		t.Fatalf("sink got %q want %q", sink.String(), "payload")
	}
	if buf.String() != "" {
		t.Fatalf("log got %q want empty on success", buf.String())
	}
}

// shortWriter 要求より少ないバイト数しか書けないフェイクio.Writerです。
type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) {
	return 0, errors.New("disk full")
}

func TestWriteAndLogError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	_, err := obs.WriteAndLog(logger, shortWriter{}, []byte("payload"), "writing response")
	if err == nil {
		t.Fatal("WriteAndLog got nil error want non-nil")
	}
	if !strings.Contains(buf.String(), "writing response") {
		t.Fatalf("log got %q want it to contain context message", buf.String())
	}
	if !strings.Contains(buf.String(), "ERROR") {
		t.Fatalf("log got %q want ERROR level", buf.String())
	}
}
