// Package testingx extends the testing package from the standard library with
// application specific testing utilities.
package testingx

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/lmittmann/tint"
	"github.com/zorcal/theapp/backend/pkg/slogctx"
)

// NewLogger creates a logger that outputs to the test log.
func NewLogger(t *testing.T) *slog.Logger {
	t.Helper()
	h := slogctx.NewHandler(tint.NewHandler(&Writer{t: t}, &tint.Options{Level: slog.LevelDebug}))
	return slog.New(h)
}

// Writer implements io.Writer and redirects output to the test log.
type Writer struct {
	t *testing.T
}

// Write implements io.Writer.
func (tw *Writer) Write(p []byte) (n int, err error) {
	tw.t.Log(string(p))
	return len(p), nil
}

// DecodeJSON reads and unmarshals JSON data from r into type T.
func DecodeJSON[T any](t *testing.T, r io.Reader) T {
	t.Helper()

	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("DecodeJSON: failed to read data: %v", err)
	}

	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		t.Logf("DecodeJSON: data: %s", string(b))
		t.Fatalf("DecodeJSON: failed to unmarshal JSON: %v", err)
	}

	return v
}

// AssertDiff compares got and want using cmp.Diff and fails the test if they
// differ.
func AssertDiff[T any](t *testing.T, got, want T, opts ...cmp.Option) {
	t.Helper()
	if diff := cmp.Diff(got, want, opts...); diff != "" {
		t.Errorf("diff mismatch (-got +want):\n%s", diff)
	}
}
