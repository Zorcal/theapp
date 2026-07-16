package grpc

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/telemetry"
	"github.com/zorcal/theapp/backend/internal/workflows"
	"github.com/zorcal/theapp/backend/pkg/slogctx"
)

// idempotencyUnaryInterceptor reads the x-idempotency-key metadata header and attaches a derived DBOS
// workflow ID to the context so that workflow handlers can deduplicate on it. Must run after
// authUnaryInterceptor so the authenticated user ID (if any) is available to scope the key. The raw
// client-supplied key is never used as the workflow ID directly: without scoping it to the caller and
// request, two different users (or the same user retrying with a different request) that happen to send
// the same key would collide on the same DBOS workflow, and the second caller would silently receive the
// first caller's cached response.
func idempotencyUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if md, ok := metadata.FromIncomingContext(ctx); ok {
			if vals := md.Get("x-idempotency-key"); len(vals) > 0 && vals[0] != "" {
				payload, err := marshalRequest(req)
				if err != nil {
					return nil, fmt.Errorf("marshal request: %w", err)
				}
				id, err := scopedWorkflowID(ctx, info.FullMethod, payload, vals[0])
				if err != nil {
					return nil, fmt.Errorf("derive workflow id: %w", err)
				}
				ctx = workflows.WithWorkflowID(ctx, id)
			}
		}
		return handler(ctx, req)
	}
}

// scopedWorkflowID derives a DBOS workflow ID from a caller-supplied idempotency key by hashing it
// together with the authenticated user ID (empty for unauthenticated callers, e.g. RequestMagicLink),
// the gRPC method, and the request payload. Binding the request payload means an unauthenticated key can
// only ever collide with a retry of the exact same request, never with a different caller's request.
// Requires key to be a valid UUID so keys are bounded in size and shape before reaching DBOS.
//
// Only used by unary RPCs. Streaming RPCs don't have a request payload available at this point in the
// call — see the Idempotency section in README.md.
func scopedWorkflowID(ctx context.Context, method string, payload []byte, key string) (string, error) {
	if _, err := uuid.Parse(key); err != nil {
		return "", status.Error(codes.InvalidArgument, "x-idempotency-key must be a valid UUID")
	}

	var userID string
	if authUser, ok := mdl.AuthUserFromContext(ctx); ok {
		userID = authUser.UserID.String()
	}

	h := sha256.New()
	h.Write([]byte(userID))
	h.Write([]byte{0})
	h.Write([]byte(method))
	h.Write([]byte{0})
	h.Write(payload)
	h.Write([]byte{0})
	h.Write([]byte(key))
	return hex.EncodeToString(h.Sum(nil)), nil
}

// marshalRequest deterministically serializes req for use in scopedWorkflowID.
func marshalRequest(req any) ([]byte, error) {
	msg, ok := req.(proto.Message)
	if !ok {
		return nil, status.Error(codes.Internal, "request does not implement proto.Message")
	}
	payload, err := proto.MarshalOptions{Deterministic: true}.Marshal(msg)
	if err != nil {
		return nil, status.Error(codes.Internal, "marshal request for idempotency scoping")
	}
	return payload, nil
}

// authUnaryInterceptor validates the JWT Bearer token on every method not in publicMethods,
// resolves the caller's mdl.AuthUser via authCore, and attaches it to the context.
func authUnaryInterceptor(jwtKey []byte, issuer, audience string, authCore AuthCore) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if _, public := publicMethods[info.FullMethod]; public {
			return handler(ctx, req)
		}

		claims, err := parseBearer(ctx, jwtKey, issuer, audience)
		if err != nil {
			return nil, fmt.Errorf("parse bearer: %w", err)
		}

		authUser, err := authCore.AuthUser(ctx, claims.UserID)
		if err != nil {
			if errors.Is(err, mdl.ErrNotFound) {
				return nil, status.Error(codes.Unauthenticated, "unauthenticated")
			}
			return nil, fmt.Errorf("resolve auth user: %w", err)
		}

		return handler(mdl.ContextWithAuthUser(ctx, authUser), req)
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

// permissionUnaryInterceptor rejects the call with codes.PermissionDenied unless every permission
// permissionRegistry requires for the method is held by the mdl.AuthUser authUnaryInterceptor
// resolved into ctx. Must run after authUnaryInterceptor.
func permissionUnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if _, public := publicMethods[info.FullMethod]; public {
			return handler(ctx, req)
		}

		required, ok := permissionRegistry[info.FullMethod]
		if !ok {
			return nil, fmt.Errorf("method %q is not registered in the permission registry", info.FullMethod)
		}

		authUser, ok := mdl.AuthUserFromContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "unauthenticated")
		}

		for _, p := range required {
			if !slices.Contains(authUser.Permissions, p) {
				return nil, status.Error(codes.PermissionDenied, "missing required permission")
			}
		}

		return handler(ctx, req)
	}
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

		// A core method rejected a request the handler's own validation should have caught — the two
		// validation layers have drifted apart.
		if errors.Is(err, mdl.ErrValidation) {
			log.ErrorContext(ctx, "Core rejected a request the endpoint validation should have caught",
				slog.String("method", info.FullMethod), slog.String("error", err.Error()))
			return resp, status.Error(codes.InvalidArgument, "invalid request")
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
