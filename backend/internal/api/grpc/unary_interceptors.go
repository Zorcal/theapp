package grpc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/telemetry"
	"github.com/zorcal/theapp/backend/pkg/slogctx"
)

// publicMethods lists gRPC methods that do not require a valid JWT. All other
// methods are authenticated by authUnaryInterceptor.
var publicMethods = map[string]struct{}{
	"/theapp.v1.AuthService/RequestMagicLink":   {},
	"/theapp.v1.AuthService/VerifyMagicLink":    {},
	"/theapp.v1.AuthService/RefreshAccessToken": {},
	"/theapp.v1.AuthService/RevokeRefreshToken": {},
}

// authUnaryInterceptor validates the JWT Bearer token on every method not in
// publicMethods and attaches the authenticated user ID to the context.
func authUnaryInterceptor(jwtKey []byte, issuer, audience string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if _, public := publicMethods[info.FullMethod]; public {
			return handler(ctx, req)
		}

		claims, err := parseBearer(ctx, jwtKey, issuer, audience)
		if err != nil {
			return nil, fmt.Errorf("parse bearer: %w", err)
		}

		return handler(contextWithUserID(ctx, claims.UserID), req)
	}
}

// parseBearer extracts the Authorization: Bearer header from ctx, validates the
// JWT against jwtKey, and returns the parsed claims.
func parseBearer(ctx context.Context, jwtKey []byte, issuer, audience string) (*mdl.AuthClaims, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing metadata")
	}

	vals := md.Get("authorization")
	if len(vals) == 0 {
		return nil, status.Error(codes.Unauthenticated, "missing authorization header")
	}

	tokenStr, ok := strings.CutPrefix(vals[0], "Bearer ")
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authorization header must use Bearer scheme")
	}

	claims := &mdl.AuthClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return jwtKey, nil
	}, jwt.WithIssuer(issuer), jwt.WithAudience(audience))
	if err != nil || !token.Valid {
		return nil, status.Error(codes.Unauthenticated, "invalid or expired token")
	}

	return claims, nil
}

// loggingUnaryInterceptor logs the method, request, response, and duration for every unary RPC.
// Sensitive pb types implement slog.LogValuer to ensure credentials are redacted, not logged in plaintext.
func loggingUnaryInterceptor(log *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, retErr error) {
		now := time.Now()

		traceID := telemetry.GetTraceID(ctx)
		if traceID != "" {
			ctx = slogctx.Attach(ctx, "trace_id", traceID)
		}

		log.InfoContext(ctx, "gRPC Unary Request - "+info.FullMethod,
			"method", info.FullMethod,
			"request", req)

		defer func() {
			log.InfoContext(ctx, "gRPC Unary Response - "+info.FullMethod,
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
