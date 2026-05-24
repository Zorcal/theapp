package slogctx_test

import (
	"bytes"
	"context"
	"log/slog"
	"testing"

	"github.com/zorcal/theapp/backend/pkg/slogctx"
)

func TestHandler(t *testing.T) {
	ctx := context.Background()

	buf := bytes.Buffer{}
	h := slogctx.NewHandler(slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		// Ignore time.
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == "time" {
				return slog.Attr{}
			}
			return a
		},
	}))
	l := slog.New(h)

	l = l.With("k1", "v1")
	ctx = slogctx.Attach(ctx, slog.String("k2", "v2"), slog.Int("k3", 3))
	l.InfoContext(ctx, "Test")

	want := `{"level":"INFO","msg":"Test","k1":"v1","k2":"v2","k3":3}
`
	if got := buf.String(); got != want {
		t.Errorf("\ngot: %q\nwant %q", got, want)
	}
}
