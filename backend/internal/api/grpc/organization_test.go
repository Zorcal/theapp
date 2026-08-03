package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/testingx"
	"github.com/zorcal/theapp/backend/pkg/mustconv"
)

func TestOrganizationService_Integration(t *testing.T) {
	srv := NewServerIntegrationTest(t)
	ctx := t.Context()

	// Seed the system organization and authorized creator.

	systemOrg := seedOrganization(t, ctx, srv.orgStore, mdl.SystemOrgName, "control")
	creator := seedUser(t, ctx, srv.userStore, "organization-creator@test.com", "Organization Creator")
	seedOrgMembership(t, ctx, srv.orgStore, creator.ExternalID, systemOrg.ID)
	seedSystemRoleAssignment(t, ctx, srv.rbacStore, creator.ExternalID, "superadmin")

	// Create an organization through the API.

	created, err := srv.organizationServiceClient.CreateOrganization(
		authCtxForUserAtProject(t, ctx, creator.ExternalID, systemOrg.ControlProjectID),
		&pb.CreateOrganizationRequest{
			Organization: &pb.Organization{Name: "acme"},
			ProjectName:  "widgets",
		},
	)
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}

	if got, want := created.GetName(), "acme"; got != want {
		t.Errorf("CreateOrganization() name = %q, want %q", got, want)
	}
	if created.GetControlProjectId() <= 0 {
		t.Errorf("CreateOrganization() control_project_id = %d, want positive", created.GetControlProjectId())
	}

	// Create and reassign an organization user through the API.

	organizationCtx := authCtxForUserAtProject(t, ctx, creator.ExternalID, int(created.GetControlProjectId()))

	createdUser, err := srv.organizationServiceClient.CreateOrganizationUser(organizationCtx, &pb.CreateOrganizationUserRequest{
		Email: "member@test.com",
	})
	if err != nil {
		t.Fatalf("CreateOrganizationUser() error = %v", err)
	}

	if got, want := createdUser.GetEmail(), "member@test.com"; got != want {
		t.Errorf("CreateOrganizationUser() email = %q, want %q", got, want)
	}

	existingUser, err := srv.organizationServiceClient.CreateOrganizationUser(organizationCtx, &pb.CreateOrganizationUserRequest{
		Email: "member@test.com",
	})
	if err != nil {
		t.Fatalf("CreateOrganizationUser() existing member error = %v", err)
	}

	testingx.AssertDiff(t, existingUser, createdUser, defaultDiffOpts())
}

func TestOrganizationService_CreateOrganization(t *testing.T) {
	now := time.Now()
	systemOrg := mdl.Organization{
		ID:               1,
		ControlProjectID: 1,
	}
	createdOrg := mdl.Organization{
		ID:               2,
		Name:             "acme",
		ControlProjectID: 3,
		CreatedAt:        now,
	}
	organizationCore := &MockedOrganizationCore{
		OrganizationByNameFunc: func(_ context.Context, _ string) (mdl.Organization, error) {
			return systemOrg, nil
		},
		IsOrganizationMemberFunc: func(_ context.Context, _ uuid.UUID, _ int) (bool, error) {
			return true, nil
		},
		CreateOrganizationFunc: func(_ context.Context, _ mdl.CreateOrganization) (mdl.Organization, error) {
			return createdOrg, nil
		},
	}
	srv := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), OrganizationCore: organizationCore})

	got, err := srv.organizationServiceClient.CreateOrganization(
		authCtxForTestUser(t, t.Context()),
		&pb.CreateOrganizationRequest{Organization: &pb.Organization{Name: "acme"}, ProjectName: "widgets"},
	)
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}

	want := &pb.Organization{
		Id:               mustconv.Int32(createdOrg.ID),
		Name:             createdOrg.Name,
		ControlProjectId: mustconv.Int32(createdOrg.ControlProjectID),
		CreateTime:       timestamppb.New(createdOrg.CreatedAt),
		UpdateTime:       nil,
	}

	testingx.AssertDiff(t, got, want, defaultDiffOpts())
}

func TestOrganizationService_CreateOrganization_error(t *testing.T) {
	tests := []struct {
		name             string
		organizationCore OrganizationCore
		in               *pb.CreateOrganizationRequest
		want             *status.Status
	}{
		{
			name: "validated request",
			organizationCore: &MockedOrganizationCore{
				OrganizationByNameFunc: func(_ context.Context, _ string) (mdl.Organization, error) {
					return mdl.Organization{ID: 1, ControlProjectID: 1}, nil
				},
				IsOrganizationMemberFunc: func(_ context.Context, _ uuid.UUID, _ int) (bool, error) {
					return true, nil
				},
			},
			in:   &pb.CreateOrganizationRequest{},
			want: status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name: "system organization membership",
			organizationCore: &MockedOrganizationCore{
				OrganizationByNameFunc: func(_ context.Context, _ string) (mdl.Organization, error) {
					return mdl.Organization{ID: 1, ControlProjectID: 1}, nil
				},
				IsOrganizationMemberFunc: func(_ context.Context, _ uuid.UUID, _ int) (bool, error) {
					return false, nil
				},
			},
			in:   &pb.CreateOrganizationRequest{Organization: &pb.Organization{Name: "acme"}, ProjectName: "widgets"},
			want: status.New(codes.PermissionDenied, codes.PermissionDenied.String()),
		},
		{
			name: "already exists",
			organizationCore: &MockedOrganizationCore{
				OrganizationByNameFunc: func(_ context.Context, _ string) (mdl.Organization, error) {
					return mdl.Organization{ID: 1, ControlProjectID: 1}, nil
				},
				IsOrganizationMemberFunc: func(_ context.Context, _ uuid.UUID, _ int) (bool, error) {
					return true, nil
				},
				CreateOrganizationFunc: func(_ context.Context, _ mdl.CreateOrganization) (mdl.Organization, error) {
					return mdl.Organization{}, mdl.ErrAlreadyExists
				},
			},
			in:   &pb.CreateOrganizationRequest{Organization: &pb.Organization{Name: "acme"}, ProjectName: "widgets"},
			want: status.New(codes.AlreadyExists, "organization already exists"),
		},
		{
			name: "control project name conflict",
			organizationCore: &MockedOrganizationCore{
				OrganizationByNameFunc: func(_ context.Context, _ string) (mdl.Organization, error) {
					return mdl.Organization{ID: 1, ControlProjectID: 1}, nil
				},
				IsOrganizationMemberFunc: func(_ context.Context, _ uuid.UUID, _ int) (bool, error) {
					return true, nil
				},
				CreateOrganizationFunc: func(_ context.Context, _ mdl.CreateOrganization) (mdl.Organization, error) {
					return mdl.Organization{}, mdl.ErrControlProjectNameConflict
				},
			},
			in:   &pb.CreateOrganizationRequest{Organization: &pb.Organization{Name: "acme"}, ProjectName: "control"},
			want: status.New(codes.InvalidArgument, "project_name conflicts with the control project"),
		},
		{
			name: "internal",
			organizationCore: &MockedOrganizationCore{
				OrganizationByNameFunc: func(_ context.Context, _ string) (mdl.Organization, error) {
					return mdl.Organization{ID: 1, ControlProjectID: 1}, nil
				},
				IsOrganizationMemberFunc: func(_ context.Context, _ uuid.UUID, _ int) (bool, error) {
					return true, nil
				},
				CreateOrganizationFunc: func(_ context.Context, _ mdl.CreateOrganization) (mdl.Organization, error) {
					return mdl.Organization{}, errors.New("boom")
				},
			},
			in:   &pb.CreateOrganizationRequest{Organization: &pb.Organization{Name: "acme"}, ProjectName: "widgets"},
			want: status.New(codes.Internal, codes.Internal.String()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := NewServerTest(t, ServerConfig{
				Log:              testingx.NewLogger(t),
				OrganizationCore: tt.organizationCore,
			})

			_, err := srv.organizationServiceClient.CreateOrganization(authCtxForTestUser(t, t.Context()), tt.in)
			if err == nil {
				t.Fatal("CreateOrganization() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("CreateOrganization() error = %q, want a gRPC status error", err)
			}

			if got.Code() != tt.want.Code() || got.Message() != tt.want.Message() {
				t.Errorf("CreateOrganization() status = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOrganizationService_CreateOrganizationUser(t *testing.T) {
	now := time.Now()
	mockedUser := mdl.User{
		ID:        uuid.New(),
		Email:     "member@test.com",
		CreatedAt: now.Add(time.Hour * -3),
		UpdatedAt: new(now),
		ETag:      uuid.NewString(),
	}
	organizationCore := &MockedOrganizationCore{
		IsOrganizationControlProjectFunc: func(_ context.Context, _, _ int) (bool, error) {
			return true, nil
		},
		CreateOrganizationUserFunc: func(_ context.Context, _ mdl.CreateOrganizationUser) (mdl.User, error) {
			return mockedUser, nil
		},
	}
	srv := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), OrganizationCore: organizationCore})

	got, err := srv.organizationServiceClient.CreateOrganizationUser(
		authCtxForTestUser(t, t.Context()),
		&pb.CreateOrganizationUserRequest{Email: mockedUser.Email},
	)
	if err != nil {
		t.Fatalf("CreateOrganizationUser() error = %v, want nil", err)
	}

	want := &pb.User{
		Id:         mockedUser.ID.String(),
		Email:      mockedUser.Email,
		CreateTime: timestamppb.New(mockedUser.CreatedAt),
		UpdateTime: timestamppb.New(*mockedUser.UpdatedAt),
		Etag:       mockedUser.ETag,
	}
	testingx.AssertDiff(t, got, want, defaultDiffOpts())
}

func TestOrganizationService_CreateOrganizationUser_error(t *testing.T) {
	tests := []struct {
		name             string
		organizationCore OrganizationCore
		in               *pb.CreateOrganizationUserRequest
		want             *status.Status
	}{
		{
			name: "validated request",
			organizationCore: &MockedOrganizationCore{
				IsOrganizationControlProjectFunc: func(_ context.Context, _, _ int) (bool, error) {
					return true, nil
				},
			},
			in: &pb.CreateOrganizationUserRequest{},
			want: status.Convert(invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
				{Field: "email", Description: "required"},
			})),
		},
		{
			name: "core",
			organizationCore: &MockedOrganizationCore{
				IsOrganizationControlProjectFunc: func(_ context.Context, _, _ int) (bool, error) {
					return true, nil
				},
				CreateOrganizationUserFunc: func(_ context.Context, _ mdl.CreateOrganizationUser) (mdl.User, error) {
					return mdl.User{}, errors.New("boom")
				},
			},
			in:   &pb.CreateOrganizationUserRequest{Email: "member@test.com"},
			want: status.New(codes.Internal, codes.Internal.String()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), OrganizationCore: tt.organizationCore})

			_, err := srv.organizationServiceClient.CreateOrganizationUser(authCtxForTestUser(t, t.Context()), tt.in)
			if err == nil {
				t.Fatal("CreateOrganizationUser() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("CreateOrganizationUser() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Proto(), tt.want.Proto(), defaultDiffOpts())
		})
	}
}
