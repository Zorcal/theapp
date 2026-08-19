package grpc

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/conv"
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/validate"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/pkg/mustconv"
)

type organizationService struct {
	pb.UnimplementedOrgServiceServer

	orgCore OrganizationCore
}

//go:generate go tool moq -rm -fmt goimports -out organization_core_moq_test.go . OrganizationCore:MockedOrganizationCore

type OrganizationCore interface {
	// CreateOrganization creates an organization with its control and default projects and
	// bootstraps the authenticated creator as its managed administrator.
	// Returns [mdl.ErrAlreadyExists] if the organization name already exists.
	// Returns [mdl.ErrControlProjectNameConflict] if the default project name conflicts with the control project.
	// Returns [mdl.ErrNotFound] if the authenticated creator no longer exists.
	// Returns [mdl.ErrValidation] if the input is invalid.
	CreateOrganization(ctx context.Context, organization mdl.CreateOrganization) (mdl.Organization, error)
	// CreateOrganizationUser returns the system user with the given email, creating it when needed,
	// and adds that user to the authenticated organization.
	// Returns [mdl.ErrValidation] if the input is invalid.
	CreateOrganizationUser(ctx context.Context, user mdl.CreateOrganizationUser) (mdl.User, error)
	// OrganizationUsers returns a page and total count of users in the authenticated organization.
	// Returns [mdl.ErrNotFound] if the project filter selects a project outside the organization.
	OrganizationUsers(ctx context.Context, filter mdl.OrganizationUserFilter, pageSize, pageOffset int) ([]mdl.User, int, error)
}

func (s *organizationService) CreateOrganizationUser(ctx context.Context, req *pb.CreateOrganizationUserRequest) (*pb.User, error) {
	if err := validate.CreateOrganizationUser(req); err != nil {
		return nil, fmt.Errorf("validate create organization user request: %w", err)
	}

	user, err := s.orgCore.CreateOrganizationUser(ctx, conv.CreateOrganizationUserFromPB(req))
	if err != nil {
		if errors.Is(err, mdl.ErrValidation) {
			return nil, status.Error(codes.InvalidArgument, "invalid organization user")
		}
		return nil, fmt.Errorf("create organization user: %w", err)
	}

	return conv.UserToPB(user), nil
}

func (s *organizationService) CreateOrganization(ctx context.Context, req *pb.CreateOrganizationRequest) (*pb.Organization, error) {
	if err := validate.CreateOrganization(req); err != nil {
		return nil, fmt.Errorf("validate create organization request: %w", err)
	}

	org, err := s.orgCore.CreateOrganization(ctx, conv.CreateOrganizationFromPB(req))
	if err != nil {
		switch {
		case errors.Is(err, mdl.ErrValidation):
			return nil, status.Error(codes.InvalidArgument, "invalid organization")
		case errors.Is(err, mdl.ErrAlreadyExists):
			return nil, status.Error(codes.AlreadyExists, "organization already exists")
		case errors.Is(err, mdl.ErrControlProjectNameConflict):
			return nil, errorStatus(codes.InvalidArgument, pb.ErrorCode_ERROR_CODE_CONTROL_PROJECT_NAME_CONFLICT,
				"project_name conflicts with the control project")
		case errors.Is(err, mdl.ErrNotFound):
			return nil, status.Error(codes.NotFound, "authenticated creator not found")
		default:
			return nil, fmt.Errorf("create organization: %w", err)
		}
	}

	return conv.OrganizationToPB(org), nil
}

func (s *organizationService) ListOrganizationUsers(ctx context.Context, req *pb.ListOrganizationUsersRequest) (*pb.ListOrganizationUsersResponse, error) {
	if err := validate.ListOrganizationUsers(req); err != nil {
		return nil, fmt.Errorf("validate list organization users request: %w", err)
	}

	pageSize := int(req.GetPageSize())
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 50
	}

	pageToken, err := conv.DecodePageToken[*pb.OrganizationUserFilter](req.GetPageToken())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", status.Error(codes.InvalidArgument, "invalid page_token"), err)
	}

	users, count, err := s.orgCore.OrganizationUsers(
		ctx,
		conv.OrganizationUserFilterFromPB(req.GetFilter()),
		pageSize,
		pageToken.Offset,
	)
	if err != nil {
		if errors.Is(err, mdl.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "filtered project not found in organization")
		}
		return nil, fmt.Errorf("list organization users: %w", err)
	}

	var nextPageToken string
	nextPageOffset := pageToken.Offset + pageSize
	if nextPageOffset < count {
		nextPageToken, err = conv.EncodePageToken(nextPageOffset, "", req.GetFilter())
		if err != nil {
			return nil, fmt.Errorf("encode next_page_token: %w", err)
		}
	}

	return &pb.ListOrganizationUsersResponse{
		Users:         conv.UsersToPB(users),
		TotalSize:     mustconv.Int32(count),
		NextPageToken: nextPageToken,
	}, nil
}
