package slogctx

import (
	"context"
	"log/slog"
)

type ctxKey struct{}

var contextKey = ctxKey{}

// Attach returns a context that includes the given attributes in each output
// operation. Arguments are converted to a slice of slog.Attr as in slog.Log.
func Attach(ctx context.Context, args ...any) context.Context {
	attrs := append(getAttrs(ctx), argsToAttrSlice(args)...)
	if len(attrs) == 0 {
		return ctx
	}
	return context.WithValue(ctx, contextKey, attrs)
}

func getAttrs(ctx context.Context) []slog.Attr {
	if v := ctx.Value(contextKey); v != nil {
		if attrs, ok := v.([]slog.Attr); ok {
			return attrs
		}
	}
	return nil
}

// Below is copied from src/log/slog/record.go of the standard library.

const badKey = "!BADKEY"

func argsToAttrSlice(args []any) []slog.Attr {
	var (
		attr  slog.Attr
		attrs []slog.Attr
	)
	for len(args) > 0 {
		attr, args = argsToAttr(args)
		attrs = append(attrs, attr)
	}
	return attrs
}

// argsToAttr turns a prefix of the nonempty args slice into an Attr
// and returns the unconsumed portion of the slice.
// If args[0] is an Attr, it returns it.
// If args[0] is a string, it treats the first two elements as
// a key-value pair.
// Otherwise, it treats args[0] as a value with a missing key.
func argsToAttr(args []any) (slog.Attr, []any) {
	switch x := args[0].(type) {
	case string:
		if len(args) == 1 {
			return slog.String(badKey, x), nil
		}
		return slog.Any(x, args[1]), args[2:]

	case slog.Attr:
		return x, args[1:]

	default:
		return slog.Any(badKey, x), args[1:]
	}
}
