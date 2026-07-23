package grpc

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/conv"
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgorg"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/testingx"
	"github.com/zorcal/theapp/backend/pkg/x/slicesx"
)

// TestSystemRoleService_Integration exercises all system-role RPCs through the real cores and
// database, including the requirement that they are called through theapp's control project.
func TestSystemRoleService_Integration(t *testing.T) {
	srv := NewServerIntegrationTest(t)
	ctx := t.Context()

	// Seed the project anchors.

	theapp, err := srv.orgStore.CreateOrganization(ctx, pgorg.CreateOrganization{
		Name:               mdl.SystemOrgName,
		ControlProjectName: "control",
	})
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}

	otherOrg, err := srv.orgStore.CreateOrganization(ctx, pgorg.CreateOrganization{
		Name:               "acme",
		ControlProjectName: "control",
	})
	if err != nil {
		t.Fatalf("CreateOrganization() other organization error = %v", err)
	}

	// Seed the users participating in the assignment.

	actor, err := srv.userStore.CreateUser(ctx, pguser.CreateUser{
		Email: "system-role-actor@test.com",
		Name:  "System Role Actor",
	})
	if err != nil {
		t.Fatalf("CreateUser() actor error = %v", err)
	}

	target, err := srv.userStore.CreateUser(ctx, pguser.CreateUser{
		Email: "system-role-target@test.com",
		Name:  "System Role Target",
	})
	if err != nil {
		t.Fatalf("CreateUser() target error = %v", err)
	}

	// Give the actor authority to manage every system role.

	if err := srv.rbacStore.AssignSystemRole(ctx, actor.ExternalID, "superadmin"); err != nil {
		t.Fatalf("AssignSystemRole() actor error = %v", err)
	}

	// Authenticate the actor through theapp's control project.

	controlCtx := authCtxForUserAtProject(t, ctx, actor.ExternalID, theapp.ControlProjectID)

	// List the seeded role definitions through theapp's control project.

	list, err := srv.systemRoleServiceClient.ListSystemRoles(controlCtx, &pb.ListSystemRolesRequest{})
	if err != nil {
		t.Fatalf("ListSystemRoles() error = %v", err)
	}

	if idx := slices.IndexFunc(list.GetRoles(), func(role *pb.SystemRole) bool {
		return role.GetName() == "superadmin"
	}); idx == -1 {
		t.Fatal(`ListSystemRoles() does not contain "superadmin"`)
	}

	// Assign the role and observe it through the assignment-list endpoint.

	if _, err := srv.systemRoleServiceClient.AssignSystemRole(controlCtx, &pb.AssignSystemRoleRequest{
		UserId:   target.ExternalID.String(),
		RoleName: "superadmin",
	}); err != nil {
		t.Fatalf("AssignSystemRole() error = %v", err)
	}

	assignments, err := srv.systemRoleServiceClient.ListSystemRoleAssignments(
		controlCtx,
		&pb.ListSystemRoleAssignmentsRequest{UserId: target.ExternalID.String()},
	)
	if err != nil {
		t.Fatalf("ListSystemRoleAssignments() after assign error = %v", err)
	}

	wantPerms := slicesx.Map(mdl.AllPermissions(), func(permission mdl.Permission) string {
		return string(permission)
	})
	slices.Sort(wantPerms)

	wantAssignments := &pb.ListSystemRoleAssignmentsResponse{
		Roles: []*pb.SystemRole{
			{
				Name:        "superadmin",
				Permissions: wantPerms,
			},
		},
		TotalSize: 1,
	}
	testingx.AssertDiff(t, assignments, wantAssignments, defaultDiffOpts())

	// Unassign the role and observe that the assignment is gone.

	if _, err := srv.systemRoleServiceClient.UnassignSystemRole(controlCtx, &pb.UnassignSystemRoleRequest{
		UserId:   target.ExternalID.String(),
		RoleName: "superadmin",
	}); err != nil {
		t.Fatalf("UnassignSystemRole() error = %v", err)
	}

	assignments, err = srv.systemRoleServiceClient.ListSystemRoleAssignments(
		controlCtx,
		&pb.ListSystemRoleAssignmentsRequest{UserId: target.ExternalID.String()},
	)
	if err != nil {
		t.Fatalf("ListSystemRoleAssignments() after unassign error = %v", err)
	}

	testingx.AssertDiff(t, assignments, &pb.ListSystemRoleAssignmentsResponse{}, defaultDiffOpts())

	// Another organization's valid control project cannot anchor system-role management.

	otherProjectCtx := authCtxForUserAtProject(t, ctx, actor.ExternalID, otherOrg.ControlProjectID)
	if _, err := srv.systemRoleServiceClient.ListSystemRoles(otherProjectCtx, &pb.ListSystemRolesRequest{}); status.Code(err) != codes.PermissionDenied {
		t.Errorf("ListSystemRoles() other project code = %v, want %v", status.Code(err), codes.PermissionDenied)
	}
}

func TestSystemRoleService_ListSystemRoles(t *testing.T) {
	nextPageToken, err := conv.EncodePageToken(2, "", &emptypb.Empty{})
	if err != nil {
		t.Fatalf("EncodePageToken() error = %v, want no error", err)
	}

	systemRoleCore := &MockedSystemRoleCore{
		SystemRolesFunc: func(_ context.Context, _, _ int) ([]mdl.SystemRole, int, error) {
			return []mdl.SystemRole{
				{Name: "systemrolesreader", Permissions: []mdl.Permission{mdl.PermissionSystemRoleRead}},
				{Name: "superadmin", Permissions: mdl.AllPermissions()},
			}, 3, nil
		},
	}
	srvTest := NewServerTest(t, ServerConfig{
		Log:            testingx.NewLogger(t),
		SystemRoleCore: systemRoleCore,
	})

	got, err := srvTest.systemRoleServiceClient.ListSystemRoles(
		authCtxForTestUser(t, t.Context()),
		&pb.ListSystemRolesRequest{PageSize: 2},
	)
	if err != nil {
		t.Fatalf("ListSystemRoles() error = %v, want no error", err)
	}

	want := &pb.ListSystemRolesResponse{
		Roles: []*pb.SystemRole{
			{
				Name:        "systemrolesreader",
				Permissions: []string{string(mdl.PermissionSystemRoleRead)},
			},
			{
				Name: "superadmin",
				Permissions: slicesx.Map(mdl.AllPermissions(), func(permission mdl.Permission) string {
					return string(permission)
				}),
			},
		},
		TotalSize:     3,
		NextPageToken: nextPageToken,
	}
	testingx.AssertDiff(t, got, want, defaultDiffOpts())
}

func TestSystemRoleService_ListSystemRoles_error(t *testing.T) {
	tests := []struct {
		name           string
		systemRoleCore SystemRoleCore
		in             *pb.ListSystemRolesRequest
		want           *status.Status
	}{
		{
			name:           "invalid page token",
			systemRoleCore: &MockedSystemRoleCore{},
			in:             &pb.ListSystemRolesRequest{PageToken: "not-a-token"},
			want:           status.New(codes.InvalidArgument, "invalid page_token"),
		},
		{
			name: "core error",
			systemRoleCore: &MockedSystemRoleCore{
				SystemRolesFunc: func(_ context.Context, _, _ int) ([]mdl.SystemRole, int, error) {
					return nil, 0, errors.New("boom")
				},
			},
			in:   &pb.ListSystemRolesRequest{},
			want: status.New(codes.Internal, codes.Internal.String()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:            testingx.NewLogger(t),
				SystemRoleCore: tt.systemRoleCore,
			})

			_, err := srvTest.systemRoleServiceClient.ListSystemRoles(authCtxForTestUser(t, t.Context()), tt.in)
			if err == nil {
				t.Fatal("ListSystemRoles() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("ListSystemRoles() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Proto(), tt.want.Proto(), defaultDiffOpts())
		})
	}
}

func TestSystemRoleService_AssignSystemRole(t *testing.T) {
	systemRoleCore := &MockedSystemRoleCore{
		AssignSystemRoleFunc: func(_ context.Context, _ uuid.UUID, _ string) error { return nil },
	}
	srvTest := NewServerTest(t, ServerConfig{
		Log:            testingx.NewLogger(t),
		SystemRoleCore: systemRoleCore,
	})

	got, err := srvTest.systemRoleServiceClient.AssignSystemRole(
		authCtxForTestUser(t, t.Context()),
		&pb.AssignSystemRoleRequest{UserId: uuid.NewString(), RoleName: "whatever"},
	)
	if err != nil {
		t.Fatalf("AssignSystemRole() error = %v, want no error", err)
	}

	testingx.AssertDiff(t, got, &pb.AssignSystemRoleResponse{}, defaultDiffOpts())
}

func TestSystemRoleService_AssignSystemRole_error(t *testing.T) {
	tests := []struct {
		name           string
		systemRoleCore SystemRoleCore
		in             *pb.AssignSystemRoleRequest
		want           *status.Status
	}{
		{
			name:           "invalid user id",
			systemRoleCore: &MockedSystemRoleCore{},
			in:             &pb.AssignSystemRoleRequest{UserId: "not-a-uuid", RoleName: "whatever"},
			want: status.Convert(invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
				{Field: "user_id", Description: "must be a valid UUID"},
			})),
		},
		{
			name:           "role name required",
			systemRoleCore: &MockedSystemRoleCore{},
			in:             &pb.AssignSystemRoleRequest{UserId: uuid.NewString()},
			want: status.Convert(invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
				{Field: "role_name", Description: "required"},
			})),
		},
		{
			name: "not found",
			systemRoleCore: &MockedSystemRoleCore{
				AssignSystemRoleFunc: func(_ context.Context, _ uuid.UUID, _ string) error {
					return mdl.ErrNotFound
				},
			},
			in:   &pb.AssignSystemRoleRequest{UserId: uuid.NewString(), RoleName: "whatever"},
			want: status.New(codes.NotFound, "user or system role not found"),
		},
		{
			name: "permission denied",
			systemRoleCore: &MockedSystemRoleCore{
				AssignSystemRoleFunc: func(_ context.Context, _ uuid.UUID, _ string) error {
					return mdl.ErrPermissionDenied
				},
			},
			in:   &pb.AssignSystemRoleRequest{UserId: uuid.NewString(), RoleName: "whatever"},
			want: status.New(codes.PermissionDenied, codes.PermissionDenied.String()),
		},
		{
			name: "already assigned",
			systemRoleCore: &MockedSystemRoleCore{
				AssignSystemRoleFunc: func(_ context.Context, _ uuid.UUID, _ string) error {
					return mdl.ErrAlreadyExists
				},
			},
			in:   &pb.AssignSystemRoleRequest{UserId: uuid.NewString(), RoleName: "whatever"},
			want: status.New(codes.AlreadyExists, "user already has system role"),
		},
		{
			name: "core error",
			systemRoleCore: &MockedSystemRoleCore{
				AssignSystemRoleFunc: func(_ context.Context, _ uuid.UUID, _ string) error {
					return errors.New("boom")
				},
			},
			in:   &pb.AssignSystemRoleRequest{UserId: uuid.NewString(), RoleName: "whatever"},
			want: status.New(codes.Internal, codes.Internal.String()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:            testingx.NewLogger(t),
				SystemRoleCore: tt.systemRoleCore,
			})

			_, err := srvTest.systemRoleServiceClient.AssignSystemRole(authCtxForTestUser(t, t.Context()), tt.in)
			if err == nil {
				t.Fatal("AssignSystemRole() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("AssignSystemRole() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Proto(), tt.want.Proto(), defaultDiffOpts())
		})
	}
}

func TestSystemRoleService_UnassignSystemRole(t *testing.T) {
	systemRoleCore := &MockedSystemRoleCore{
		UnassignSystemRoleFunc: func(_ context.Context, _ uuid.UUID, _ string) error { return nil },
	}
	srvTest := NewServerTest(t, ServerConfig{
		Log:            testingx.NewLogger(t),
		SystemRoleCore: systemRoleCore,
	})

	got, err := srvTest.systemRoleServiceClient.UnassignSystemRole(
		authCtxForTestUser(t, t.Context()),
		&pb.UnassignSystemRoleRequest{UserId: uuid.NewString(), RoleName: "whatever"},
	)
	if err != nil {
		t.Fatalf("UnassignSystemRole() error = %v, want no error", err)
	}

	testingx.AssertDiff(t, got, &pb.UnassignSystemRoleResponse{}, defaultDiffOpts())
}

func TestSystemRoleService_UnassignSystemRole_error(t *testing.T) {
	tests := []struct {
		name           string
		systemRoleCore SystemRoleCore
		want           *status.Status
	}{
		{
			name: "not found",
			systemRoleCore: &MockedSystemRoleCore{
				UnassignSystemRoleFunc: func(_ context.Context, _ uuid.UUID, _ string) error {
					return mdl.ErrNotFound
				},
			},
			want: status.New(codes.NotFound, "system role assignment not found"),
		},
		{
			name: "permission denied",
			systemRoleCore: &MockedSystemRoleCore{
				UnassignSystemRoleFunc: func(_ context.Context, _ uuid.UUID, _ string) error {
					return mdl.ErrPermissionDenied
				},
			},
			want: status.New(codes.PermissionDenied, codes.PermissionDenied.String()),
		},
		{
			name: "last role manager",
			systemRoleCore: &MockedSystemRoleCore{
				UnassignSystemRoleFunc: func(_ context.Context, _ uuid.UUID, _ string) error {
					return mdl.ErrLastRoleManager
				},
			},
			want: status.New(codes.FailedPrecondition, "cannot remove the last system role manager"),
		},
		{
			name: "core error",
			systemRoleCore: &MockedSystemRoleCore{
				UnassignSystemRoleFunc: func(_ context.Context, _ uuid.UUID, _ string) error {
					return errors.New("boom")
				},
			},
			want: status.New(codes.Internal, codes.Internal.String()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:            testingx.NewLogger(t),
				SystemRoleCore: tt.systemRoleCore,
			})

			_, err := srvTest.systemRoleServiceClient.UnassignSystemRole(
				authCtxForTestUser(t, t.Context()),
				&pb.UnassignSystemRoleRequest{UserId: uuid.NewString(), RoleName: "whatever"},
			)
			if err == nil {
				t.Fatal("UnassignSystemRole() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("UnassignSystemRole() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Proto(), tt.want.Proto(), defaultDiffOpts())
		})
	}
}

func TestSystemRoleService_ListSystemRoleAssignments(t *testing.T) {
	systemRoleCore := &MockedSystemRoleCore{
		UserSystemRolesFunc: func(_ context.Context, _ uuid.UUID, _, _ int) ([]mdl.SystemRole, int, error) {
			return []mdl.SystemRole{
				{Name: "systemrolesreader", Permissions: []mdl.Permission{mdl.PermissionSystemRoleRead}},
			}, 1, nil
		},
	}
	srvTest := NewServerTest(t, ServerConfig{
		Log:            testingx.NewLogger(t),
		SystemRoleCore: systemRoleCore,
	})

	got, err := srvTest.systemRoleServiceClient.ListSystemRoleAssignments(
		authCtxForTestUser(t, t.Context()),
		&pb.ListSystemRoleAssignmentsRequest{UserId: uuid.NewString()},
	)
	if err != nil {
		t.Fatalf("ListSystemRoleAssignments() error = %v, want no error", err)
	}

	want := &pb.ListSystemRoleAssignmentsResponse{
		Roles: []*pb.SystemRole{
			{Name: "systemrolesreader", Permissions: []string{string(mdl.PermissionSystemRoleRead)}},
		},
		TotalSize: 1,
	}
	testingx.AssertDiff(t, got, want, defaultDiffOpts())
}

func TestSystemRoleService_ListSystemRoleAssignments_error(t *testing.T) {
	tests := []struct {
		name           string
		systemRoleCore SystemRoleCore
		in             *pb.ListSystemRoleAssignmentsRequest
		want           *status.Status
	}{
		{
			name:           "invalid user id",
			systemRoleCore: &MockedSystemRoleCore{},
			in:             &pb.ListSystemRoleAssignmentsRequest{UserId: "not-a-uuid"},
			want: status.Convert(invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
				{Field: "user_id", Description: "must be a valid UUID"},
			})),
		},
		{
			name:           "invalid page token",
			systemRoleCore: &MockedSystemRoleCore{},
			in:             &pb.ListSystemRoleAssignmentsRequest{UserId: uuid.NewString(), PageToken: "not-a-token"},
			want:           status.New(codes.InvalidArgument, "invalid page_token"),
		},
		{
			name: "user not found",
			systemRoleCore: &MockedSystemRoleCore{
				UserSystemRolesFunc: func(_ context.Context, _ uuid.UUID, _, _ int) ([]mdl.SystemRole, int, error) {
					return nil, 0, mdl.ErrNotFound
				},
			},
			in:   &pb.ListSystemRoleAssignmentsRequest{UserId: "00000000-0000-0000-0000-000000000002"},
			want: status.New(codes.NotFound, `user "00000000-0000-0000-0000-000000000002" not found`),
		},
		{
			name: "core error",
			systemRoleCore: &MockedSystemRoleCore{
				UserSystemRolesFunc: func(_ context.Context, _ uuid.UUID, _, _ int) ([]mdl.SystemRole, int, error) {
					return nil, 0, errors.New("boom")
				},
			},
			in:   &pb.ListSystemRoleAssignmentsRequest{UserId: uuid.NewString()},
			want: status.New(codes.Internal, codes.Internal.String()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:            testingx.NewLogger(t),
				SystemRoleCore: tt.systemRoleCore,
			})

			_, err := srvTest.systemRoleServiceClient.ListSystemRoleAssignments(authCtxForTestUser(t, t.Context()), tt.in)
			if err == nil {
				t.Fatal("ListSystemRoleAssignments() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("ListSystemRoleAssignments() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Proto(), tt.want.Proto(), defaultDiffOpts())
		})
	}
}
