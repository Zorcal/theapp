package grpc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"slices"
	"time"

	"github.com/zorcal/theapp/backend/internal/telemetry"
	"github.com/zorcal/theapp/backend/pkg/slogctx"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func loggingUnaryInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, retErr error) {
		now := time.Now()

		traceID := telemetry.GetTraceID(ctx)
		if traceID != "" {
			ctx = slogctx.Attach(ctx, "trace_id", traceID)
		}

		log.InfoContext(ctx, fmt.Sprintf("gRPC Unary Request - %s", info.FullMethod),
			"method", info.FullMethod,
			"request", req)

		defer func() {
			log.InfoContext(ctx, fmt.Sprintf("gRPC Unary Response - %s", info.FullMethod),
				"method", info.FullMethod,
				"duration_ms", time.Since(now).Milliseconds(),
				"response", resp)
		}()

		resp, err := handler(ctx, req)
		if err != nil {
			return resp, err
		}

		return resp, nil
	}
}

func recoveryUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, retErr error) {
		defer func() {
			if r := recover(); r != nil {
				trace := debug.Stack()
				retErr = fmt.Errorf("PANIC [%v] TRACE[%s]", r, string(trace))
			}
		}()

		resp, err := handler(ctx, req)
		if err != nil {
			return resp, err
		}

		return resp, nil
	}
}

func errorUnaryInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, retErr error) {
		resp, err := handler(ctx, req)
		if err == nil {
			return resp, nil
		}

		st := status.New(codes.Internal, codes.Internal.String())

		var gs interface{ GRPCStatus() *status.Status }
		if errors.As(err, &gs) && gs.GRPCStatus() != nil {
			st = gs.GRPCStatus()
		}

		attrs := slices.Concat(
			[]slog.Attr{
				slog.String("method", info.FullMethod),
				slog.String("error", err.Error()),
				slog.String("external_error", st.Err().Error()),
			},
			grpcStatusDetailsLogAttrs(st),
		)
		log.LogAttrs(ctx, codeToLevel(st.Code()), "gRPC Unary Error", attrs...)

		return resp, st.Err()
	}
}

// codeToLevel maps a gRPC status code to a log level by where the fault lies:
// caller-driven problems are logged at Warn, and expected benign outcomes at
// Info so they don't pollute the error signal. Everything else falls through to
// Error so unexpected system states surface in alerting. This deliberately
// includes Unavailable and ResourceExhausted: from our own server these signal
// a downed dependency or genuine resource exhaustion rather than a caller
// mistake. Any unmapped code also pages rather than going silent.
func codeToLevel(code codes.Code) slog.Level {
	switch code {
	case codes.NotFound, codes.AlreadyExists, codes.Canceled, codes.OK:
		return slog.LevelInfo
	case codes.InvalidArgument, codes.FailedPrecondition, codes.PermissionDenied,
		codes.Unauthenticated, codes.Aborted, codes.OutOfRange,
		codes.DeadlineExceeded:
		return slog.LevelWarn
	default:
		// Internal, Unknown, DataLoss, Unimplemented, Unavailable,
		// ResourceExhausted, and any unmapped code.
		return slog.LevelError
	}
}

func grpcStatusDetailsLogAttrs(st *status.Status) []slog.Attr {
	if st == nil {
		return nil
	}

	var attrs []slog.Attr
	for i, det := range st.Details() {
		switch t := det.(type) {
		case *errdetails.BadRequest:
			attrs = append(attrs, slog.Any("field_violations", t.GetFieldViolations()))
		default:
			// Shouldn't happen, but we don't want to miss logging any
			// important data in case of a programming error.
			attrs = append(attrs, slog.Any(fmt.Sprintf("detail_%d", i+1), det))
		}
	}

	return attrs
}
