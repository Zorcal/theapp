package grpc

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/zorcal/theapp/backend/internal/telemetry"
	"github.com/zorcal/theapp/backend/internal/workflows"
	"github.com/zorcal/theapp/backend/pkg/slogctx"
)

// idempotencyStreamInterceptor is the streaming counterpart of idempotencyUnaryInterceptor.
func idempotencyStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if vals := md.Get("x-idempotency-key"); len(vals) > 0 && vals[0] != "" {
				ss = newCtxOverrideStream(ss, workflows.WithWorkflowID(ctx, vals[0]))
			}
		}
		return handler(srv, ss)
	}
}

// authStreamInterceptor is the streaming counterpart of authUnaryInterceptor.
func authStreamInterceptor(jwtKey []byte, issuer, audience string) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if _, public := publicMethods[info.FullMethod]; public {
			return handler(srv, ss)
		}

		claims, err := parseBearer(ss.Context(), jwtKey, issuer, audience)
		if err != nil {
			return fmt.Errorf("parse bearer: %w", err)
		}

		return handler(srv, newCtxOverrideStream(ss, contextWithUserID(ss.Context(), claims.UserID)))
	}
}

// loggingStreamInterceptor logs the method and duration at stream boundaries, and each individual message at DEBUG level.
// Sensitive pb types implement slog.LogValuer to ensure credentials are redacted, not logged in plaintext.
func loggingStreamInterceptor(log *slog.Logger) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		// Skip gRPC infrastructure services (reflection, health). Their
		// metadata streams produce noisy per-message logs with no insight
		// into application behavior.
		if strings.HasPrefix(info.FullMethod, "/grpc.reflection.") {
			return handler(srv, ss)
		}

		now := time.Now()
		ctx := ss.Context()

		if traceID := telemetry.GetTraceID(ctx); traceID != "" {
			ctx = slogctx.Attach(ctx, "trace_id", traceID)
		}

		log.InfoContext(ctx, "gRPC Stream Start - "+info.FullMethod,
			"method", info.FullMethod)

		defer func() {
			log.InfoContext(ctx, "gRPC Stream End - "+info.FullMethod,
				"method", info.FullMethod,
				"duration_ms", time.Since(now).Milliseconds())
		}()

		wrapped := newLoggingStream(newCtxOverrideStream(ss, ctx), log, info.FullMethod)
		if err := handler(srv, wrapped); err != nil {
			return err
		}

		return nil
	}
}

func recoveryStreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (retErr error) {
		defer func() {
			if r := recover(); r != nil {
				trace := debug.Stack()
				retErr = fmt.Errorf("PANIC [%v] TRACE[%s]", r, string(trace))
			}
		}()
		return handler(srv, ss)
	}
}

func errorStreamInterceptor(log *slog.Logger) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		ctx := ss.Context()

		err := handler(srv, ss)
		if err == nil {
			return nil
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
		log.LogAttrs(ctx, codeToLevel(st.Code()), "gRPC Stream Error", attrs...)

		return st.Err()
	}
}

// ctxOverrideStream is the only way for a stream interceptor to enrich a ctx
// and make it available to subsequent interceptors and the handler. Stream
// handlers read ctx from ss.Context() — without substituting the stream, a
// derived ctx never reaches them. Context() returns the supplied ctx; all
// other methods delegate to the embedded stream. Nested wraps compose.
type ctxOverrideStream struct {
	grpc.ServerStream

	ctx context.Context //nolint:containedctx // the whole point of this type.
}

func newCtxOverrideStream(ss grpc.ServerStream, ctx context.Context) *ctxOverrideStream {
	return &ctxOverrideStream{ServerStream: ss, ctx: ctx}
}

func (s *ctxOverrideStream) Context() context.Context {
	return s.ctx
}

// loggingStream emits a DEBUG-level log line for every message sent or
// received on the stream. Without it, only the stream's start and end are
// logged — individual messages are invisible, in contrast to unary RPCs
// where the request and response are logged on each call.
type loggingStream struct {
	grpc.ServerStream

	log    *slog.Logger
	method string
}

func newLoggingStream(ss grpc.ServerStream, log *slog.Logger, method string) *loggingStream {
	return &loggingStream{ServerStream: ss, log: log, method: method}
}

func (s *loggingStream) SendMsg(m any) (retErr error) {
	defer func() {
		attrs := []slog.Attr{
			slog.String("method", s.method),
			slog.Any("message", m),
		}
		if retErr != nil {
			attrs = append(attrs, slog.String("error", retErr.Error()))
		}
		s.log.LogAttrs(s.Context(), slog.LevelDebug, "gRPC Stream Send", attrs...)
	}()
	return s.ServerStream.SendMsg(m)
}

func (s *loggingStream) RecvMsg(m any) error {
	err := s.ServerStream.RecvMsg(m)
	// io.EOF is the normal end-of-stream signal from the client, not an
	// error condition worth a log line.
	if errors.Is(err, io.EOF) {
		return err
	}
	attrs := []slog.Attr{
		slog.String("method", s.method),
		slog.Any("message", m),
	}
	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
	}
	s.log.LogAttrs(s.Context(), slog.LevelDebug, "gRPC Stream Recv", attrs...)
	return err
}
