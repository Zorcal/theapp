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

	created, err := srv.orgServiceClient.CreateOrganization(
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

	orgCtx := authCtxForUserAtProject(t, ctx, creator.ExternalID, int(created.GetControlProjectId()))

	createdUser, err := srv.orgServiceClient.CreateOrganizationUser(orgCtx, &pb.CreateOrganizationUserRequest{
		Email: "member@test.com",
	})
	if err != nil {
		t.Fatalf("CreateOrganizationUser() error = %v", err)
	}

	if got, want := createdUser.GetEmail(), "member@test.com"; got != want {
		t.Errorf("CreateOrganizationUser() email = %q, want %q", got, want)
	}

	existingUser, err := srv.orgServiceClient.CreateOrganizationUser(orgCtx, &pb.CreateOrganizationUserRequest{
		Email: "member@test.com",
	})
	if err != nil {
		t.Fatalf("CreateOrganizationUser() existing member error = %v", err)
	}

	testingx.AssertDiff(t, existingUser, createdUser, defaultDiffOpts())

	// List the organization's users through the API.

	users, err := srv.orgServiceClient.ListOrganizationUsers(orgCtx, &pb.ListOrganizationUsersRequest{})
	if err != nil {
		t.Fatalf("ListOrganizationUsers() error = %v", err)
	}

	if got, want := users.GetTotalSize(), int32(2); got != want {
		t.Errorf("ListOrganizationUsers() total_size = %d, want %d", got, want)
	}
	if got, want := len(users.GetUsers()), 2; got != want {
		t.Errorf("ListOrganizationUsers() users length = %d, want %d", got, want)
	}

	// Remove the organization user through the API.

	if _, err := srv.orgServiceClient.RemoveOrganizationUser(orgCtx, &pb.RemoveOrganizationUserRequest{Id: createdUser.GetId()}); err != nil {
		t.Fatalf("RemoveOrganizationUser() error = %v", err)
	}

	users, err = srv.orgServiceClient.ListOrganizationUsers(orgCtx, &pb.ListOrganizationUsersRequest{})
	if err != nil {
		t.Fatalf("ListOrganizationUsers() after removal error = %v", err)
	}
	if got, want := users.GetTotalSize(), int32(1); got != want {
		t.Errorf("ListOrganizationUsers() total_size after removal = %d, want %d", got, want)
	}
}

func TestOrganizationService_CreateOrganization(t *testing.T) {
	now := time.Now()
	mockedOrg := mdl.Organization{
		ID:               2,
		Name:             "acme",
		ControlProjectID: 3,
		CreatedAt:        now,
	}
	orgCore := &MockedOrganizationCore{
		CreateOrganizationFunc: func(_ context.Context, _ mdl.CreateOrganization) (mdl.Organization, error) {
			return mockedOrg, nil
		},
	}
	srv := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), OrganizationCore: orgCore})

	got, err := srv.orgServiceClient.CreateOrganization(
		authCtxForTestUser(t, t.Context()),
		&pb.CreateOrganizationRequest{Organization: &pb.Organization{Name: "acme"}, ProjectName: "widgets"},
	)
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}

	want := &pb.Organization{
		Id:               mustconv.Int32(mockedOrg.ID),
		Name:             mockedOrg.Name,
		ControlProjectId: mustconv.Int32(mockedOrg.ControlProjectID),
		CreateTime:       timestamppb.New(mockedOrg.CreatedAt),
	}

	testingx.AssertDiff(t, got, want, defaultDiffOpts())
}

func TestOrganizationService_CreateOrganization_error(t *testing.T) {
	tests := []struct {
		name    string
		orgCore OrganizationCore
		in      *pb.CreateOrganizationRequest
		want    *status.Status
	}{
		{
			name:    "validated request",
			orgCore: &MockedOrganizationCore{},
			in:      &pb.CreateOrganizationRequest{},
			want:    status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name: "already exists",
			orgCore: &MockedOrganizationCore{
				CreateOrganizationFunc: func(_ context.Context, _ mdl.CreateOrganization) (mdl.Organization, error) {
					return mdl.Organization{}, mdl.ErrAlreadyExists
				},
			},
			in:   &pb.CreateOrganizationRequest{Organization: &pb.Organization{Name: "acme"}, ProjectName: "widgets"},
			want: status.New(codes.AlreadyExists, "organization already exists"),
		},
		{
			name: "control project name conflict",
			orgCore: &MockedOrganizationCore{
				CreateOrganizationFunc: func(_ context.Context, _ mdl.CreateOrganization) (mdl.Organization, error) {
					return mdl.Organization{}, mdl.ErrControlProjectNameConflict
				},
			},
			in:   &pb.CreateOrganizationRequest{Organization: &pb.Organization{Name: "acme"}, ProjectName: "control"},
			want: status.New(codes.InvalidArgument, "project_name conflicts with the control project"),
		},
		{
			name: "internal",
			orgCore: &MockedOrganizationCore{
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
				OrganizationCore: tt.orgCore,
			})

			_, err := srv.orgServiceClient.CreateOrganization(authCtxForTestUser(t, t.Context()), tt.in)
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
	orgCore := &MockedOrganizationCore{
		CreateOrganizationUserFunc: func(_ context.Context, _ mdl.CreateOrganizationUser) (mdl.User, error) {
			return mockedUser, nil
		},
	}
	srv := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), OrganizationCore: orgCore})

	got, err := srv.orgServiceClient.CreateOrganizationUser(
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
		name    string
		orgCore OrganizationCore
		in      *pb.CreateOrganizationUserRequest
		want    *status.Status
	}{
		{
			name:    "validated request",
			orgCore: &MockedOrganizationCore{},
			in:      &pb.CreateOrganizationUserRequest{},
			want: status.Convert(invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
				{Field: "email", Description: "required"},
			})),
		},
		{
			name: "deleted user",
			orgCore: &MockedOrganizationCore{
				CreateOrganizationUserFunc: func(_ context.Context, _ mdl.CreateOrganizationUser) (mdl.User, error) {
					return mdl.User{}, mdl.ErrUserDeleted
				},
			},
			in:   &pb.CreateOrganizationUserRequest{Email: "member@test.com"},
			want: status.New(codes.FailedPrecondition, "a deleted user with this email must be restored"),
		},
		{
			name: "core",
			orgCore: &MockedOrganizationCore{
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
			srv := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), OrganizationCore: tt.orgCore})

			_, err := srv.orgServiceClient.CreateOrganizationUser(authCtxForTestUser(t, t.Context()), tt.in)
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

func TestOrganizationService_ListOrganizationUsers(t *testing.T) {
	now := time.Now()
	firstUser := mdl.User{
		ID:              uuid.New(),
		Email:           "a-member@test.com",
		Name:            "First Member",
		EmailVerifiedAt: new(now.Add(-2 * time.Hour)),
		CreatedAt:       now.Add(-24 * time.Hour),
		UpdatedAt:       new(now.Add(-time.Hour)),
		ETag:            uuid.NewString(),
	}
	secondUser := mdl.User{
		ID:        uuid.New(),
		Email:     "b-member@test.com",
		Name:      "Second Member",
		CreatedAt: now.Add(-12 * time.Hour),
		ETag:      uuid.NewString(),
	}
	thirdUser := mdl.User{
		ID:        uuid.New(),
		Email:     "c-member@test.com",
		Name:      "Third Member",
		CreatedAt: now.Add(-6 * time.Hour),
		ETag:      uuid.NewString(),
	}
	pbFirstUser := &pb.User{
		Id:                firstUser.ID.String(),
		Email:             firstUser.Email,
		Name:              firstUser.Name,
		EmailVerifiedTime: timestamppb.New(*firstUser.EmailVerifiedAt),
		CreateTime:        timestamppb.New(firstUser.CreatedAt),
		UpdateTime:        timestamppb.New(*firstUser.UpdatedAt),
		Etag:              firstUser.ETag,
	}
	pbSecondUser := &pb.User{
		Id:         secondUser.ID.String(),
		Email:      secondUser.Email,
		Name:       secondUser.Name,
		CreateTime: timestamppb.New(secondUser.CreatedAt),
		Etag:       secondUser.ETag,
	}
	pbThirdUser := &pb.User{
		Id:         thirdUser.ID.String(),
		Email:      thirdUser.Email,
		Name:       thirdUser.Name,
		CreateTime: timestamppb.New(thirdUser.CreatedAt),
		Etag:       thirdUser.ETag,
	}

	tests := []struct {
		name    string
		orgCore OrganizationCore
		in      *pb.ListOrganizationUsersRequest
		want    *pb.ListOrganizationUsersResponse
	}{
		{
			name: "empty request",
			orgCore: &MockedOrganizationCore{
				OrganizationUsersFunc: func(_ context.Context, _ mdl.OrganizationUserFilter, _, _ int) ([]mdl.User, int, error) {
					return []mdl.User{firstUser, secondUser, thirdUser}, 3, nil
				},
			},
			in: &pb.ListOrganizationUsersRequest{},
			want: &pb.ListOrganizationUsersResponse{
				Users:     []*pb.User{pbFirstUser, pbSecondUser, pbThirdUser},
				TotalSize: 3,
			},
		},
		{
			name: "empty result",
			orgCore: &MockedOrganizationCore{
				OrganizationUsersFunc: func(_ context.Context, _ mdl.OrganizationUserFilter, _, _ int) ([]mdl.User, int, error) {
					return nil, 0, nil
				},
			},
			in:   &pb.ListOrganizationUsersRequest{},
			want: &pb.ListOrganizationUsersResponse{},
		},
		{
			name: "first page carries filter into next_page_token",
			orgCore: &MockedOrganizationCore{
				OrganizationUsersFunc: func(_ context.Context, _ mdl.OrganizationUserFilter, _, _ int) ([]mdl.User, int, error) {
					return []mdl.User{firstUser, secondUser}, 5, nil
				},
			},
			in: &pb.ListOrganizationUsersRequest{
				PageSize: 2,
				Filter:   &pb.OrganizationUserFilter{ProjectId: 7},
			},
			want: &pb.ListOrganizationUsersResponse{
				Users:         []*pb.User{pbFirstUser, pbSecondUser},
				TotalSize:     5,
				NextPageToken: "eyJvIjoyLCJmIjoiQ0FjPSJ9",
			},
		},
		{
			name: "page_token offset is honored",
			orgCore: &MockedOrganizationCore{
				OrganizationUsersFunc: func(_ context.Context, _ mdl.OrganizationUserFilter, _, _ int) ([]mdl.User, int, error) {
					return []mdl.User{thirdUser}, 5, nil
				},
			},
			in: &pb.ListOrganizationUsersRequest{
				PageSize:  2,
				PageToken: "eyJvIjoyLCJmIjoiQ0FjPSJ9",
				Filter:    &pb.OrganizationUserFilter{ProjectId: 7},
			},
			want: &pb.ListOrganizationUsersResponse{
				Users:         []*pb.User{pbThirdUser},
				TotalSize:     5,
				NextPageToken: "eyJvIjo0LCJmIjoiQ0FjPSJ9",
			},
		},
		{
			name: "single page returns no next_page_token",
			orgCore: &MockedOrganizationCore{
				OrganizationUsersFunc: func(_ context.Context, _ mdl.OrganizationUserFilter, _, _ int) ([]mdl.User, int, error) {
					return []mdl.User{firstUser, secondUser, thirdUser}, 3, nil
				},
			},
			in: &pb.ListOrganizationUsersRequest{PageSize: 10},
			want: &pb.ListOrganizationUsersResponse{
				Users:     []*pb.User{pbFirstUser, pbSecondUser, pbThirdUser},
				TotalSize: 3,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), OrganizationCore: tt.orgCore})

			got, err := srv.orgServiceClient.ListOrganizationUsers(authCtxForTestUser(t, t.Context()), tt.in)
			if err != nil {
				t.Fatalf("ListOrganizationUsers() error = %v", err)
			}

			testingx.AssertDiff(t, got, tt.want, defaultDiffOpts())
		})
	}
}

func TestOrganizationService_ListOrganizationUsers_error(t *testing.T) {
	tests := []struct {
		name    string
		orgCore OrganizationCore
		in      *pb.ListOrganizationUsersRequest
		want    codes.Code
	}{
		{
			name:    "validated request",
			orgCore: &MockedOrganizationCore{},
			in:      &pb.ListOrganizationUsersRequest{Filter: &pb.OrganizationUserFilter{ProjectId: -1}},
			want:    codes.InvalidArgument,
		},
		{
			name: "project not found",
			orgCore: &MockedOrganizationCore{
				OrganizationUsersFunc: func(_ context.Context, _ mdl.OrganizationUserFilter, _, _ int) ([]mdl.User, int, error) {
					return nil, 0, mdl.ErrNotFound
				},
			},
			in:   &pb.ListOrganizationUsersRequest{},
			want: codes.NotFound,
		},
		{
			name: "core",
			orgCore: &MockedOrganizationCore{
				OrganizationUsersFunc: func(_ context.Context, _ mdl.OrganizationUserFilter, _, _ int) ([]mdl.User, int, error) {
					return nil, 0, errors.New("boom")
				},
			},
			in:   &pb.ListOrganizationUsersRequest{},
			want: codes.Internal,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), OrganizationCore: tt.orgCore})

			_, err := srv.orgServiceClient.ListOrganizationUsers(authCtxForTestUser(t, t.Context()), tt.in)
			if status.Code(err) != tt.want {
				t.Errorf("ListOrganizationUsers() code = %s, want %s", status.Code(err), tt.want)
			}
		})
	}
}
