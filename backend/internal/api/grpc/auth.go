package grpc

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/conv"
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/email"
)

type authService struct {
	pb.UnimplementedAuthServiceServer

	authCore         AuthCore
	workflowAuthCore WorkflowAuthCore
}

//go:generate moq -rm -fmt goimports -out auth_core_moq_test.go . AuthCore:MockedAuthCore

// AuthCore handles direct, non-durable auth operations.
// Implemented by *core/auth.Core.
type AuthCore interface {
	// VerifyMagicLink validates a magic-link token and returns a token pair.
	// Returns [mdl.ErrTokenInvalid] if the token is expired, consumed, or not found.
	VerifyMagicLink(ctx context.Context, rawToken string) (mdl.AuthTokenPair, error)
	// RefreshAccessToken rotates the refresh token and returns a new token pair.
	// Returns [mdl.ErrTokenInvalid] if the token is expired, revoked, or not found.
	RefreshAccessToken(ctx context.Context, rawToken string) (mdl.AuthTokenPair, error)
	// RevokeRefreshToken invalidates a refresh token.
	// Returns [mdl.ErrTokenInvalid] if the token is not found or already revoked.
	RevokeRefreshToken(ctx context.Context, rawToken string) error
	// RevokeAllUserRefreshTokens revokes all active refresh tokens for the user.
	RevokeAllUserRefreshTokens(ctx context.Context, userExternalID uuid.UUID) error
}

//go:generate moq -rm -fmt goimports -out workflow_auth_core_moq_test.go . WorkflowAuthCore:MockedWorkflowAuthCore

// WorkflowAuthCore handles durable auth operations backed by DBOS.
// Implemented by *workflows/auth.WorkflowCore.
type WorkflowAuthCore interface {
	RequestMagicLink(ctx context.Context, email string) error
}

func (s *authService) RequestMagicLink(ctx context.Context, req *pb.RequestMagicLinkRequest) (*emptypb.Empty, error) {
	if req.GetEmail() == "" {
		return nil, invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
			{Field: "email", Description: "required"},
		})
	}
	if !email.Validate(req.GetEmail()) {
		return nil, invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
			{Field: "email", Description: "invalid format"},
		})
	}

	if err := s.workflowAuthCore.RequestMagicLink(ctx, req.GetEmail()); err != nil {
		return nil, fmt.Errorf("request magic link: %w", err)
	}

	return &emptypb.Empty{}, nil
}

func (s *authService) VerifyMagicLink(ctx context.Context, req *pb.VerifyMagicLinkRequest) (*pb.TokenPair, error) {
	if req.GetToken() == "" {
		return nil, invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
			{Field: "token", Description: "required"},
		})
	}

	pair, err := s.authCore.VerifyMagicLink(ctx, req.GetToken())
	if err != nil {
		if errors.Is(err, mdl.ErrTokenInvalid) {
			return nil, status.Error(codes.Unauthenticated, "token invalid or expired")
		}
		return nil, fmt.Errorf("verify magic link: %w", err)
	}

	return conv.TokenPairToPB(pair), nil
}

func (s *authService) RefreshAccessToken(ctx context.Context, req *pb.RefreshAccessTokenRequest) (*pb.TokenPair, error) {
	if req.GetRefreshToken() == "" {
		return nil, invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
			{Field: "refresh_token", Description: "required"},
		})
	}

	pair, err := s.authCore.RefreshAccessToken(ctx, req.GetRefreshToken())
	if err != nil {
		if errors.Is(err, mdl.ErrTokenInvalid) {
			return nil, status.Error(codes.Unauthenticated, "refresh token invalid, expired, or revoked")
		}
		return nil, fmt.Errorf("refresh access token: %w", err)
	}

	return conv.TokenPairToPB(pair), nil
}

func (s *authService) RevokeRefreshToken(ctx context.Context, req *pb.RevokeRefreshTokenRequest) (*emptypb.Empty, error) {
	if req.GetRefreshToken() == "" {
		return nil, invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
			{Field: "refresh_token", Description: "required"},
		})
	}

	if err := s.authCore.RevokeRefreshToken(ctx, req.GetRefreshToken()); err != nil {
		if errors.Is(err, mdl.ErrTokenInvalid) {
			return nil, status.Error(codes.NotFound, "refresh token not found or already revoked")
		}
		return nil, fmt.Errorf("revoke refresh token: %w", err)
	}

	return &emptypb.Empty{}, nil
}

func (s *authService) RevokeAllSessions(ctx context.Context, _ *emptypb.Empty) (*emptypb.Empty, error) {
	userID, ok := UserIDFromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "unauthenticated")
	}

	if err := s.authCore.RevokeAllUserRefreshTokens(ctx, userID); err != nil {
		return nil, fmt.Errorf("revoke all sessions: %w", err)
	}

	return &emptypb.Empty{}, nil
}
