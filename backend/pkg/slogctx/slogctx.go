// Package slogctx provides a slog.Handler that adds attributes to each log record
// based on the log context.
package slogctx

import (
	"context"
	"fmt"
	"log/slog"
)

// Handler wraps a slog.Handler and adds attributes to each log record
// based on the context.
type Handler struct {
	h slog.Handler
}

// NewHandler returns a new Handler that wraps h.
func NewHandler(h slog.Handler) *Handler {
	return &Handler{h: h}
}

// Enabled calls Enabled on the underlying handler.
func (h *Handler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.h.Enabled(ctx, l)
}

// Handle logs a record by calling the underlying handler. Before calling the
// underlying handler, it adds any attributes to the record that was added to
// the context using the Attach function.
func (h *Handler) Handle(ctx context.Context, rec slog.Record) error {
	if attrs := getAttrs(ctx); len(attrs) > 0 {
		rec = rec.Clone()
		rec.AddAttrs(attrs...)
	}
	if err := h.h.Handle(ctx, rec); err != nil {
		return fmt.Errorf("handle: %w", err)
	}
	return nil
}

// WithAttrs calls WithAttrs on the underlying handler.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return NewHandler(h.h.WithAttrs(attrs))
}

// WithGroup calls WithGroup on the underlying handler.
func (h *Handler) WithGroup(name string) slog.Handler {
	return NewHandler(h.h.WithGroup(name))
}
