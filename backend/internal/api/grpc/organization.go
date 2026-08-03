package grpc

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/conv"
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/validate"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
)

type organizationService struct {
	pb.UnimplementedOrgServiceServer

	organizationCore OrganizationCore
}

//go:generate moq -rm -fmt goimports -out organization_core_moq_test.go . OrganizationCore:MockedOrganizationCore

type OrganizationCore interface {
	// CreateOrganization creates an organization with its control and default projects and
	// bootstraps the authenticated creator as its managed administrator.
	// Returns [mdl.ErrAlreadyExists] if the organization name already exists.
	// Returns [mdl.ErrControlProjectNameConflict] if the default project name conflicts with the control project.
	// Returns [mdl.ErrNotFound] if the authenticated creator no longer exists.
	// Returns [mdl.ErrValidation] if the input is invalid.
	CreateOrganization(ctx context.Context, organization mdl.CreateOrganization) (mdl.Organization, error)
	// OrganizationByName returns the organization with the given name.
	// Returns [mdl.ErrNotFound] if no organization with that name exists.
	OrganizationByName(ctx context.Context, name string) (mdl.Organization, error)
	// IsOrganizationMember reports whether userID is a member of orgID.
	IsOrganizationMember(ctx context.Context, userID uuid.UUID, orgID int) (bool, error)
	// IsOrganizationControlProject reports whether projectID is orgID's control project.
	IsOrganizationControlProject(ctx context.Context, orgID, projectID int) (bool, error)
	// CreateOrganizationUser returns the system user with the given email, creating it when needed,
	// and adds that user to the authenticated organization.
	// Returns [mdl.ErrValidation] if the input is invalid.
	CreateOrganizationUser(ctx context.Context, user mdl.CreateOrganizationUser) (mdl.User, error)
}

func (s *organizationService) CreateOrganizationUser(ctx context.Context, req *pb.CreateOrganizationUserRequest) (*pb.User, error) {
	if err := validate.CreateOrganizationUser(req); err != nil {
		return nil, fmt.Errorf("validate create organization user request: %w", err)
	}

	user, err := s.organizationCore.CreateOrganizationUser(ctx, conv.CreateOrganizationUserFromPB(req))
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

	org, err := s.organizationCore.CreateOrganization(ctx, conv.CreateOrganizationFromPB(req))
	if err != nil {
		switch {
		case errors.Is(err, mdl.ErrValidation):
			return nil, status.Error(codes.InvalidArgument, "invalid organization")
		case errors.Is(err, mdl.ErrAlreadyExists):
			return nil, status.Error(codes.AlreadyExists, "organization already exists")
		case errors.Is(err, mdl.ErrControlProjectNameConflict):
			return nil, status.Error(codes.InvalidArgument, "project_name conflicts with the control project")
		case errors.Is(err, mdl.ErrNotFound):
			return nil, status.Error(codes.NotFound, "authenticated creator not found")
		default:
			return nil, fmt.Errorf("create organization: %w", err)
		}
	}

	return conv.OrganizationToPB(org), nil
}
