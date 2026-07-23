// Package grpc provides the gRPC server for the application.
package grpc

import (
	"log/slog"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/stats"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/pkg/set"
)

// ServerConfig contains all the mandatory systems required by the GRPC server.
type ServerConfig struct {
	Log                        *slog.Logger
	UserCore                   UserCore
	AuthCore                   AuthCore
	SystemRoleCore             SystemRoleCore
	SystemRoleOrganizationCore SystemRoleOrganizationCore
	WorkflowAuthCore           WorkflowAuthCore
	// JWTKey is the HMAC secret used to validate access tokens.
	JWTKey      []byte
	JWTIssuer   string
	JWTAudience string
	// Reflection registers the gRPC reflection service. Enable for local development so ad-hoc clients (grpcurl, Evans,
	// ...) can discover the schema; keep it off elsewhere so the schema isn't exposed publicly.
	Reflection bool
}

// publicMethods lists gRPC methods that do not require a valid JWT. All other
// methods are authenticated by authUnaryInterceptor / authStreamInterceptor.
var publicMethods = set.Set[string]{
	"/theapp.v1.AuthService/RequestMagicLink":   {},
	"/theapp.v1.AuthService/VerifyMagicLink":    {},
	"/theapp.v1.AuthService/RefreshAccessToken": {},
	"/theapp.v1.AuthService/RevokeRefreshToken": {},
}

// noProjectMethods lists protected (non-public, see publicMethods) gRPC methods that legitimately
// have no project context, so they're exempt from requiring x-project-id metadata. All other
// protected methods require it.
var noProjectMethods = set.Set[string]{
	"/theapp.v1.AuthService/RevokeAllSessions": {},

	// UserService is a system-wide directory, not a project- or org-scoped resource.
	"/theapp.v1.UserService/GetUser":    {},
	"/theapp.v1.UserService/ListUsers":  {},
	"/theapp.v1.UserService/CreateUser": {},
	"/theapp.v1.UserService/UpdateUser": {},
}

// permissionRegistry maps every protected (non-public, see publicMethods) gRPC method to the
// permissions required to call it. All listed permissions must be held — this is a conjunction
// (AND), never a disjunction. A method with no entry here is denied rather than let through
// unchecked; an endpoint that legitimately requires no permission still needs an explicit empty-list
// entry, so a missing entry can never be mistaken for "deliberately open".
var permissionRegistry = map[string][]mdl.Permission{
	"/theapp.v1.AuthService/RevokeAllSessions": {},

	"/theapp.v1.UserService/GetUser":    {mdl.PermissionUserRead},
	"/theapp.v1.UserService/ListUsers":  {mdl.PermissionUserRead},
	"/theapp.v1.UserService/CreateUser": {mdl.PermissionUserCreate},
	"/theapp.v1.UserService/UpdateUser": {mdl.PermissionUserUpdate},

	"/theapp.v1.SystemRoleService/ListSystemRoles":           {mdl.PermissionSystemRoleRead},
	"/theapp.v1.SystemRoleService/AssignSystemRole":          {mdl.PermissionSystemRoleAssign},
	"/theapp.v1.SystemRoleService/UnassignSystemRole":        {mdl.PermissionSystemRoleUnassign},
	"/theapp.v1.SystemRoleService/ListSystemRoleAssignments": {mdl.PermissionSystemRoleRead},
}

// NewServer constructs the GRPC server.
func NewServer(cfg ServerConfig) *grpc.Server {
	srv := grpc.NewServer(
		grpc.MaxRecvMsgSize(2*1024*1024), // 2mb
		grpc.StatsHandler(otelgrpc.NewServerHandler(otelgrpc.WithFilter(notGRPCInfrastructure))),
		// recovery interceptor must run after error interceptor so a caught panic still gets error's structured log
		// (trace ID, status-level mapping) instead of unwinding past it, but before any other interceptor so panics
		// from parsing untrusted input there don't crash the process. logging and error themselves are left
		// unprotected: they don't parse untrusted input, so a panic there is considered a programming error, not a
		// runtime risk worth guarding against.
		grpc.ChainUnaryInterceptor(
			loggingUnaryInterceptor(cfg.Log),
			errorUnaryInterceptor(cfg.Log),
			recoveryUnaryInterceptor(),
			authUnaryInterceptor(cfg.JWTKey, cfg.JWTIssuer, cfg.JWTAudience, cfg.AuthCore),
			permissionUnaryInterceptor(),
			idempotencyUnaryInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			loggingStreamInterceptor(cfg.Log),
			errorStreamInterceptor(cfg.Log),
			recoveryStreamInterceptor(),
			authStreamInterceptor(cfg.JWTKey, cfg.JWTIssuer, cfg.JWTAudience, cfg.AuthCore),
			permissionStreamInterceptor(),
		),
	)

	pb.RegisterUserServiceServer(srv, &userService{
		userCore: cfg.UserCore,
	})

	pb.RegisterAuthServiceServer(srv, &authService{
		authCore:         cfg.AuthCore,
		workflowAuthCore: cfg.WorkflowAuthCore,
	})

	pb.RegisterSystemRoleServiceServer(srv, &systemRoleService{
		systemRoleCore:             cfg.SystemRoleCore,
		systemRoleOrganizationCore: cfg.SystemRoleOrganizationCore,
	})

	if cfg.Reflection {
		reflection.Register(srv)
	}

	return srv
}

// notGRPCInfrastructure returns false for gRPC's own infrastructure services
// (reflection, health) so they aren't traced. Without this they'd flood
// tracing backends like Tempo with reflection's bidi stream metadata,
// drowning out spans that describe actual application behavior.
func notGRPCInfrastructure(info *stats.RPCTagInfo) bool {
	return !strings.HasPrefix(info.FullMethodName, "/grpc.reflection.")
}
