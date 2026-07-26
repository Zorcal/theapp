package grpc

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgorg"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

// TestRoleService_Integration exercises every custom-role RPC through the real core and database,
// including reads, field updates, permission changes, and deletion in the caller's organization.
func TestRoleService_Integration(t *testing.T) {
	srv := NewServerIntegrationTest(t)
	ctx := t.Context()

	// Seed the organizations used to verify role ownership.

	org, err := srv.orgStore.CreateOrganization(ctx, pgorg.CreateOrganization{
		Name:               "role-service-org",
		ControlProjectName: "control",
	})
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}

	otherOrg, err := srv.orgStore.CreateOrganization(ctx, pgorg.CreateOrganization{
		Name:               "other-role-service-org",
		ControlProjectName: "control",
	})
	if err != nil {
		t.Fatalf("CreateOrganization() error = %v", err)
	}

	// Seed an authorized actor and an organization-member assignment target.

	actor, err := srv.userStore.CreateUser(ctx, pguser.CreateUser{
		Email: "role-service-actor@test.com",
		Name:  "Role Service Actor",
	})
	if err != nil {
		t.Fatalf("CreateUser() error = %v", err)
	}

	if err := srv.rbacStore.AssignSystemRole(ctx, actor.ExternalID, "superadmin"); err != nil {
		t.Fatalf("AssignSystemRole() error = %v", err)
	}

	target, err := srv.userStore.CreateUser(ctx, pguser.CreateUser{
		Email: "role-service-target@test.com",
		Name:  "Role Service Target",
	})
	if err != nil {
		t.Fatalf("CreateUser() target error = %v", err)
	}

	seedOrgMembership(t, ctx, srv.pool, target.ID, org.ID)

	// Authenticate the actor through the owning organization's control project.

	authCtx := authCtxForUserAtProject(t, ctx, actor.ExternalID, org.ControlProjectID)
	otherAuthCtx := authCtxForUserAtProject(t, ctx, actor.ExternalID, otherOrg.ControlProjectID)

	// Create a custom role in the caller's organization.

	created, err := srv.customRoleServiceClient.CreateRole(authCtx, &pb.CreateRoleRequest{
		Role: &pb.Role{
			Name:        "role manager",
			Permissions: []pb.Permission{pb.Permission_PERMISSION_CUSTOM_ROLE_READ},
		},
	})
	if err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}

	// Read the role by id.

	got, err := srv.customRoleServiceClient.GetRole(authCtx, &pb.GetRoleRequest{Id: created.GetId()})
	if err != nil {
		t.Fatalf("GetRole() error = %v", err)
	}

	testingx.AssertDiff(t, got, created, defaultDiffOpts())

	// List roles.

	list, err := srv.customRoleServiceClient.ListRoles(authCtx, &pb.ListRolesRequest{PageSize: 1})
	if err != nil {
		t.Fatalf("ListRoles() error = %v", err)
	}

	wantRoles := []*pb.Role{created}

	testingx.AssertDiff(t, list.GetRoles(), wantRoles, defaultDiffOpts())

	if got, want := list.GetTotalSize(), int32(1); got != want {
		t.Errorf("ListRoles() total size = %d, want %d", got, want)
	}

	// Update the role's selected resource fields.

	updated, err := srv.customRoleServiceClient.UpdateRole(authCtx, &pb.UpdateRoleRequest{
		Role: &pb.Role{
			Id:          created.GetId(),
			Name:        "custom role manager",
			Permissions: []pb.Permission{pb.Permission_PERMISSION_CUSTOM_ROLE_DELETE},
		},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name", "permissions"}},
	})
	if err != nil {
		t.Fatalf("UpdateRole() error = %v", err)
	}

	if got, want := updated.GetName(), "custom role manager"; got != want {
		t.Errorf("UpdateRole() name = %q, want %q", got, want)
	}

	if got, want := updated.GetPermissions(), []pb.Permission{pb.Permission_PERMISSION_CUSTOM_ROLE_DELETE}; !slices.Equal(got, want) {
		t.Errorf("UpdateRole() permissions = %v, want %v", got, want)
	}

	// Atomically add and remove permissions.

	modified, err := srv.customRoleServiceClient.ModifyRolePermissions(authCtx, &pb.ModifyRolePermissionsRequest{
		Id:                created.GetId(),
		AddPermissions:    []pb.Permission{pb.Permission_PERMISSION_CUSTOM_ROLE_UPDATE},
		RemovePermissions: []pb.Permission{pb.Permission_PERMISSION_CUSTOM_ROLE_DELETE},
	})
	if err != nil {
		t.Fatalf("ModifyRolePermissions() error = %v", err)
	}

	if got, want := modified.GetPermissions(), []pb.Permission{pb.Permission_PERMISSION_CUSTOM_ROLE_UPDATE}; !slices.Equal(got, want) {
		t.Errorf("ModifyRolePermissions() permissions = %v, want %v", got, want)
	}

	// Assign the role in the request's project.

	if _, err := srv.customRoleServiceClient.AssignRoleToProject(authCtx, &pb.AssignRoleToProjectRequest{
		RoleId: created.GetId(),
		UserId: target.ExternalID.String(),
	}); err != nil {
		t.Fatalf("AssignRoleToProject() error = %v", err)
	}

	// List the target user's project role assignments.

	projectAssignments, err := srv.customRoleServiceClient.ListProjectRoleAssignments(authCtx, &pb.ListProjectRoleAssignmentsRequest{
		UserId: target.ExternalID.String(),
	})
	if err != nil {
		t.Fatalf("ListProjectRoleAssignments() error = %v", err)
	}

	wantProjectRoles := []*pb.Role{modified}

	testingx.AssertDiff(t, projectAssignments.GetRoles(), wantProjectRoles, defaultDiffOpts())

	if got, want := projectAssignments.GetTotalSize(), int32(1); got != want {
		t.Errorf("ListProjectRoleAssignments() total size = %d, want %d", got, want)
	}

	// Unassign the role from the request's project.

	if _, err := srv.customRoleServiceClient.UnassignRoleFromProject(authCtx, &pb.UnassignRoleFromProjectRequest{
		RoleId: created.GetId(),
		UserId: target.ExternalID.String(),
	}); err != nil {
		t.Fatalf("UnassignRoleFromProject() error = %v", err)
	}

	// List the target user's project role assignments after unassignment.

	projectAssignments, err = srv.customRoleServiceClient.ListProjectRoleAssignments(authCtx, &pb.ListProjectRoleAssignmentsRequest{
		UserId: target.ExternalID.String(),
	})
	if err != nil {
		t.Fatalf("ListProjectRoleAssignments() after unassign error = %v", err)
	}

	testingx.AssertDiff(t, projectAssignments.GetRoles(), []*pb.Role{}, append(defaultDiffOpts(), cmpopts.EquateEmpty())...)

	if got, want := projectAssignments.GetTotalSize(), int32(0); got != want {
		t.Errorf("ListProjectRoleAssignments() after unassign total size = %d, want %d", got, want)
	}

	// Assign the role across the request project's organization.

	if _, err := srv.customRoleServiceClient.AssignRoleToOrganization(authCtx, &pb.AssignRoleToOrganizationRequest{
		RoleId: created.GetId(),
		UserId: target.ExternalID.String(),
	}); err != nil {
		t.Fatalf("AssignRoleToOrganization() error = %v", err)
	}

	// List the target user's organization role assignments.

	orgAssignments, err := srv.customRoleServiceClient.ListOrganizationRoleAssignments(authCtx, &pb.ListOrganizationRoleAssignmentsRequest{
		UserId: target.ExternalID.String(),
	})
	if err != nil {
		t.Fatalf("ListOrganizationRoleAssignments() error = %v", err)
	}

	wantOrgRoles := []*pb.Role{modified}

	testingx.AssertDiff(t, orgAssignments.GetRoles(), wantOrgRoles, defaultDiffOpts())

	if got, want := orgAssignments.GetTotalSize(), int32(1); got != want {
		t.Errorf("ListOrganizationRoleAssignments() total size = %d, want %d", got, want)
	}

	// Unassign the role from the request project's organization.

	if _, err := srv.customRoleServiceClient.UnassignRoleFromOrganization(authCtx, &pb.UnassignRoleFromOrganizationRequest{
		RoleId: created.GetId(),
		UserId: target.ExternalID.String(),
	}); err != nil {
		t.Fatalf("UnassignRoleFromOrganization() error = %v", err)
	}

	// List the target user's organization role assignments after unassignment.

	orgAssignments, err = srv.customRoleServiceClient.ListOrganizationRoleAssignments(authCtx, &pb.ListOrganizationRoleAssignmentsRequest{
		UserId: target.ExternalID.String(),
	})
	if err != nil {
		t.Fatalf("ListOrganizationRoleAssignments() after unassign error = %v", err)
	}

	testingx.AssertDiff(t, orgAssignments.GetRoles(), []*pb.Role{}, append(defaultDiffOpts(), cmpopts.EquateEmpty())...)

	if got, want := orgAssignments.GetTotalSize(), int32(0); got != want {
		t.Errorf("ListOrganizationRoleAssignments() after unassign total size = %d, want %d", got, want)
	}

	// Verify the role is inaccessible through another organization.

	if _, err := srv.customRoleServiceClient.UpdateRole(otherAuthCtx, &pb.UpdateRoleRequest{
		Role:       &pb.Role{Id: created.GetId(), Name: "other role manager"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
	}); status.Code(err) != codes.NotFound {
		t.Errorf("UpdateRole() through another organization code = %v, want %v", status.Code(err), codes.NotFound)
	}

	// Delete the role and verify it is no longer directly readable.

	if _, err := srv.customRoleServiceClient.DeleteRole(authCtx, &pb.DeleteRoleRequest{Id: created.GetId()}); err != nil {
		t.Fatalf("DeleteRole() error = %v", err)
	}

	// Verify the deleted role no longer appears in the organization's collection.

	list, err = srv.customRoleServiceClient.ListRoles(authCtx, &pb.ListRolesRequest{})
	if err != nil {
		t.Fatalf("ListRoles() after delete error = %v", err)
	}

	wantRoles = []*pb.Role{}

	testingx.AssertDiff(t, list.GetRoles(), wantRoles, append(defaultDiffOpts(), cmpopts.EquateEmpty())...)

	if got, want := list.GetTotalSize(), int32(0); got != want {
		t.Errorf("ListRoles() after delete total size = %d, want %d", got, want)
	}
}

func TestRoleService_CreateRole(t *testing.T) {
	mockedRole := mdl.CustomRole{
		ID:          uuid.New(),
		Name:        "role manager",
		Permissions: []mdl.Permission{mdl.PermissionCustomRoleRead},
		CreatedAt:   time.Now(),
		UpdatedAt:   new(time.Now().Add(time.Minute)),
		ETag:        uuid.NewString(),
	}
	customRoleCore := &MockedCustomRoleCore{
		CreateCustomRoleFunc: func(_ context.Context, _ mdl.CreateCustomRole) (mdl.CustomRole, error) {
			return mockedRole, nil
		},
	}
	srvTest := NewServerTest(t, ServerConfig{
		Log:            testingx.NewLogger(t),
		CustomRoleCore: customRoleCore,
	})

	got, err := srvTest.customRoleServiceClient.CreateRole(
		authCtxForTestUser(t, t.Context()),
		&pb.CreateRoleRequest{
			Role: &pb.Role{Name: mockedRole.Name, Permissions: []pb.Permission{pb.Permission_PERMISSION_CUSTOM_ROLE_READ}},
		},
	)
	if err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}

	want := &pb.Role{
		Id:          mockedRole.ID.String(),
		Name:        mockedRole.Name,
		Permissions: []pb.Permission{pb.Permission_PERMISSION_CUSTOM_ROLE_READ},
		CreateTime:  timestamppb.New(mockedRole.CreatedAt),
		UpdateTime:  timestamppb.New(*mockedRole.UpdatedAt),
		Etag:        mockedRole.ETag,
	}

	testingx.AssertDiff(t, got, want, defaultDiffOpts())
}

func TestRoleService_CreateRole_error(t *testing.T) {
	invalidArgWithViolation := func(field, desc string) *status.Status {
		st, err := status.New(codes.InvalidArgument, codes.InvalidArgument.String()).WithDetails(
			&errdetails.BadRequest{FieldViolations: []*errdetails.BadRequest_FieldViolation{
				{Field: field, Description: desc},
			}},
		)
		if err != nil {
			t.Fatalf("invalidArgWithViolation(%q, %q) build status error = %v", field, desc, err)
		}
		return st
	}

	tests := []struct {
		name           string
		customRoleCore CustomRoleCore
		in             *pb.CreateRoleRequest
		want           *status.Status
	}{
		{
			name:           "validated request",
			customRoleCore: &MockedCustomRoleCore{},
			in:             &pb.CreateRoleRequest{},
			want:           invalidArgWithViolation("role", "required"),
		},
		{
			name: "already exists",
			customRoleCore: &MockedCustomRoleCore{
				CreateCustomRoleFunc: func(_ context.Context, _ mdl.CreateCustomRole) (mdl.CustomRole, error) {
					return mdl.CustomRole{}, mdl.ErrAlreadyExists
				},
			},
			in:   &pb.CreateRoleRequest{Role: &pb.Role{Name: "role manager"}},
			want: invalidArgWithViolation("role.name", "a role with this name already exists"),
		},
		{
			name: "invalid role",
			customRoleCore: &MockedCustomRoleCore{
				CreateCustomRoleFunc: func(_ context.Context, _ mdl.CreateCustomRole) (mdl.CustomRole, error) {
					return mdl.CustomRole{}, mdl.ErrValidation
				},
			},
			in:   &pb.CreateRoleRequest{Role: &pb.Role{Name: "role manager"}},
			want: status.New(codes.InvalidArgument, "invalid role"),
		},
		{
			name: "organization or permission not found",
			customRoleCore: &MockedCustomRoleCore{
				CreateCustomRoleFunc: func(_ context.Context, _ mdl.CreateCustomRole) (mdl.CustomRole, error) {
					return mdl.CustomRole{}, mdl.ErrNotFound
				},
			},
			in:   &pb.CreateRoleRequest{Role: &pb.Role{Name: "role manager"}},
			want: status.New(codes.NotFound, "organization or permission not found"),
		},
		{
			name: "core error",
			customRoleCore: &MockedCustomRoleCore{
				CreateCustomRoleFunc: func(_ context.Context, _ mdl.CreateCustomRole) (mdl.CustomRole, error) {
					return mdl.CustomRole{}, errors.New("boom")
				},
			},
			in:   &pb.CreateRoleRequest{Role: &pb.Role{Name: "role manager"}},
			want: status.New(codes.Internal, codes.Internal.String()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:            testingx.NewLogger(t),
				CustomRoleCore: tt.customRoleCore,
			})

			_, err := srvTest.customRoleServiceClient.CreateRole(authCtxForTestUser(t, t.Context()), tt.in)
			if err == nil {
				t.Fatal("CreateRole() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("CreateRole() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Proto(), tt.want.Proto(), defaultDiffOpts())
		})
	}
}

func TestRoleService_GetRole(t *testing.T) {
	mockedRole := mdl.CustomRole{
		ID:          uuid.New(),
		Name:        "role manager",
		Permissions: []mdl.Permission{mdl.PermissionCustomRoleRead},
		CreatedAt:   time.Now(),
		UpdatedAt:   new(time.Now().Add(time.Minute)),
		ETag:        uuid.NewString(),
	}
	customRoleCore := &MockedCustomRoleCore{
		CustomRoleByIDFunc: func(_ context.Context, _ uuid.UUID) (mdl.CustomRole, error) {
			return mockedRole, nil
		},
	}
	srvTest := NewServerTest(t, ServerConfig{
		Log:            testingx.NewLogger(t),
		CustomRoleCore: customRoleCore,
	})

	got, err := srvTest.customRoleServiceClient.GetRole(
		authCtxForTestUser(t, t.Context()),
		&pb.GetRoleRequest{Id: mockedRole.ID.String()},
	)
	if err != nil {
		t.Fatalf("GetRole() error = %v", err)
	}

	want := &pb.Role{
		Id:          mockedRole.ID.String(),
		Name:        mockedRole.Name,
		Permissions: []pb.Permission{pb.Permission_PERMISSION_CUSTOM_ROLE_READ},
		CreateTime:  timestamppb.New(mockedRole.CreatedAt),
		UpdateTime:  timestamppb.New(*mockedRole.UpdatedAt),
		Etag:        mockedRole.ETag,
	}

	testingx.AssertDiff(t, got, want, defaultDiffOpts())
}

func TestRoleService_GetRole_error(t *testing.T) {
	invalidArgWithViolation := func(field, desc string) *status.Status {
		st, err := status.New(codes.InvalidArgument, codes.InvalidArgument.String()).WithDetails(
			&errdetails.BadRequest{FieldViolations: []*errdetails.BadRequest_FieldViolation{
				{Field: field, Description: desc},
			}},
		)
		if err != nil {
			t.Fatalf("invalidArgWithViolation(%q, %q) build status error = %v", field, desc, err)
		}
		return st
	}

	roleID := uuid.New()

	tests := []struct {
		name           string
		customRoleCore CustomRoleCore
		in             *pb.GetRoleRequest
		want           *status.Status
	}{
		{
			name:           "validated request",
			customRoleCore: &MockedCustomRoleCore{},
			in:             &pb.GetRoleRequest{Id: "bad"},
			want:           invalidArgWithViolation("id", "must be a valid UUID"),
		},
		{
			name: "missing role",
			customRoleCore: &MockedCustomRoleCore{
				CustomRoleByIDFunc: func(_ context.Context, _ uuid.UUID) (mdl.CustomRole, error) {
					return mdl.CustomRole{}, mdl.ErrNotFound
				},
			},
			in:   &pb.GetRoleRequest{Id: roleID.String()},
			want: status.New(codes.NotFound, `role "`+roleID.String()+`" not found`),
		},
		{
			name: "core error",
			customRoleCore: &MockedCustomRoleCore{
				CustomRoleByIDFunc: func(_ context.Context, _ uuid.UUID) (mdl.CustomRole, error) {
					return mdl.CustomRole{}, errors.New("boom")
				},
			},
			in:   &pb.GetRoleRequest{Id: roleID.String()},
			want: status.New(codes.Internal, codes.Internal.String()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:            testingx.NewLogger(t),
				CustomRoleCore: tt.customRoleCore,
			})

			_, err := srvTest.customRoleServiceClient.GetRole(authCtxForTestUser(t, t.Context()), tt.in)
			if err == nil {
				t.Fatal("GetRole() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("GetRole() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Proto(), tt.want.Proto(), defaultDiffOpts())
		})
	}
}

func TestRoleService_ListRoles(t *testing.T) {
	diffOpts := defaultDiffOpts()

	now := time.Now()

	firstRole := mdl.CustomRole{
		ID:          uuid.New(),
		Name:        "role reader",
		Permissions: []mdl.Permission{mdl.PermissionCustomRoleRead},
		CreatedAt:   now.Add(-3 * time.Hour),
		ETag:        uuid.NewString(),
	}
	secondRole := mdl.CustomRole{
		ID:          uuid.New(),
		Name:        "role editor",
		Permissions: []mdl.Permission{mdl.PermissionCustomRoleRead, mdl.PermissionCustomRoleUpdate},
		CreatedAt:   now.Add(-2 * time.Hour),
		UpdatedAt:   new(now.Add(-time.Hour)),
		ETag:        uuid.NewString(),
	}
	thirdRole := mdl.CustomRole{
		ID:          uuid.New(),
		Name:        "role administrator",
		Permissions: []mdl.Permission{mdl.PermissionCustomRoleCreate, mdl.PermissionCustomRoleDelete},
		CreatedAt:   now,
		ETag:        uuid.NewString(),
	}

	pbFirstRole := &pb.Role{
		Id:          firstRole.ID.String(),
		Name:        firstRole.Name,
		Permissions: []pb.Permission{pb.Permission_PERMISSION_CUSTOM_ROLE_READ},
		CreateTime:  timestamppb.New(firstRole.CreatedAt),
		Etag:        firstRole.ETag,
	}
	pbSecondRole := &pb.Role{
		Id:          secondRole.ID.String(),
		Name:        secondRole.Name,
		Permissions: []pb.Permission{pb.Permission_PERMISSION_CUSTOM_ROLE_READ, pb.Permission_PERMISSION_CUSTOM_ROLE_UPDATE},
		CreateTime:  timestamppb.New(secondRole.CreatedAt),
		UpdateTime:  timestamppb.New(*secondRole.UpdatedAt),
		Etag:        secondRole.ETag,
	}
	pbThirdRole := &pb.Role{
		Id:          thirdRole.ID.String(),
		Name:        thirdRole.Name,
		Permissions: []pb.Permission{pb.Permission_PERMISSION_CUSTOM_ROLE_CREATE, pb.Permission_PERMISSION_CUSTOM_ROLE_DELETE},
		CreateTime:  timestamppb.New(thirdRole.CreatedAt),
		Etag:        thirdRole.ETag,
	}

	tests := []struct {
		name           string
		customRoleCore CustomRoleCore
		in             *pb.ListRolesRequest
		want           *pb.ListRolesResponse
	}{
		{
			name: "empty request",
			customRoleCore: &MockedCustomRoleCore{
				CustomRolesFunc: func(_ context.Context, _, _ int) ([]mdl.CustomRole, int, error) {
					return []mdl.CustomRole{firstRole, secondRole, thirdRole}, 15, nil
				},
			},
			in: &pb.ListRolesRequest{},
			want: &pb.ListRolesResponse{
				Roles:     []*pb.Role{pbFirstRole, pbSecondRole, pbThirdRole},
				TotalSize: 15,
			},
		},
		{
			name: "empty result",
			customRoleCore: &MockedCustomRoleCore{
				CustomRolesFunc: func(_ context.Context, _, _ int) ([]mdl.CustomRole, int, error) {
					return nil, 0, nil
				},
			},
			in:   &pb.ListRolesRequest{},
			want: &pb.ListRolesResponse{},
		},
		{
			name: "first page returns next_page_token when more results exist",
			customRoleCore: &MockedCustomRoleCore{
				CustomRolesFunc: func(_ context.Context, _, _ int) ([]mdl.CustomRole, int, error) {
					return []mdl.CustomRole{firstRole, secondRole}, 5, nil
				},
			},
			in: &pb.ListRolesRequest{PageSize: 2},
			want: &pb.ListRolesResponse{
				Roles:         []*pb.Role{pbFirstRole, pbSecondRole},
				TotalSize:     5,
				NextPageToken: "eyJvIjoyfQ==",
			},
		},
		{
			name: "single page returns no next_page_token",
			customRoleCore: &MockedCustomRoleCore{
				CustomRolesFunc: func(_ context.Context, _, _ int) ([]mdl.CustomRole, int, error) {
					return []mdl.CustomRole{firstRole, secondRole, thirdRole}, 3, nil
				},
			},
			in: &pb.ListRolesRequest{PageSize: 10},
			want: &pb.ListRolesResponse{
				Roles:     []*pb.Role{pbFirstRole, pbSecondRole, pbThirdRole},
				TotalSize: 3,
			},
		},
		{
			name: "page_token offset is honored",
			customRoleCore: &MockedCustomRoleCore{
				CustomRolesFunc: func(_ context.Context, _, _ int) ([]mdl.CustomRole, int, error) {
					return []mdl.CustomRole{thirdRole}, 10, nil
				},
			},
			in: &pb.ListRolesRequest{
				PageSize:  2,
				PageToken: "eyJvIjoyfQ==",
			},
			want: &pb.ListRolesResponse{
				Roles:         []*pb.Role{pbThirdRole},
				TotalSize:     10,
				NextPageToken: "eyJvIjo0fQ==",
			},
		},
		{
			name: "last page exactly fills page size",
			customRoleCore: &MockedCustomRoleCore{
				CustomRolesFunc: func(_ context.Context, _, _ int) ([]mdl.CustomRole, int, error) {
					return []mdl.CustomRole{firstRole, secondRole, thirdRole}, 3, nil
				},
			},
			in: &pb.ListRolesRequest{PageSize: 3},
			want: &pb.ListRolesResponse{
				Roles:     []*pb.Role{pbFirstRole, pbSecondRole, pbThirdRole},
				TotalSize: 3,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:            testingx.NewLogger(t),
				CustomRoleCore: tt.customRoleCore,
			})

			got, err := srvTest.customRoleServiceClient.ListRoles(authCtxForTestUser(t, t.Context()), tt.in)
			if err != nil {
				t.Fatalf("ListRoles() error = %q, want no error", err)
			}

			testingx.AssertDiff(t, got.GetRoles(), tt.want.GetRoles(), diffOpts)

			if got.GetTotalSize() != tt.want.GetTotalSize() {
				t.Errorf("ListRoles() total_size = %d, want %d", got.GetTotalSize(), tt.want.GetTotalSize())
			}

			if got.GetNextPageToken() != tt.want.GetNextPageToken() {
				t.Errorf("ListRoles() next_page_token = %q, want %q", got.GetNextPageToken(), tt.want.GetNextPageToken())
			}
		})
	}
}

func TestRoleService_ListRoles_error(t *testing.T) {
	tests := []struct {
		name           string
		customRoleCore CustomRoleCore
		in             *pb.ListRolesRequest
		want           *status.Status
	}{
		{
			name:           "validated request",
			customRoleCore: &MockedCustomRoleCore{},
			in:             &pb.ListRolesRequest{PageToken: "bad"},
			want:           status.New(codes.InvalidArgument, "invalid page_token"),
		},
		{
			name: "core error",
			customRoleCore: &MockedCustomRoleCore{
				CustomRolesFunc: func(_ context.Context, _, _ int) ([]mdl.CustomRole, int, error) {
					return nil, 0, errors.New("boom")
				},
			},
			in:   &pb.ListRolesRequest{},
			want: status.New(codes.Internal, codes.Internal.String()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:            testingx.NewLogger(t),
				CustomRoleCore: tt.customRoleCore,
			})

			_, err := srvTest.customRoleServiceClient.ListRoles(authCtxForTestUser(t, t.Context()), tt.in)
			if err == nil {
				t.Fatal("ListRoles() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("ListRoles() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Proto(), tt.want.Proto(), defaultDiffOpts())
		})
	}
}

func TestRoleService_ListProjectRoleAssignments(t *testing.T) {
	mockedRole := mdl.CustomRole{
		ID:          uuid.New(),
		Name:        "project reader",
		Permissions: []mdl.Permission{mdl.PermissionCustomRoleRead},
		CreatedAt:   time.Now(),
		UpdatedAt:   new(time.Now().Add(time.Minute)),
		ETag:        uuid.NewString(),
	}
	customRoleCore := &MockedCustomRoleCore{
		UserProjectCustomRolesFunc: func(_ context.Context, _ uuid.UUID, _, _ int) ([]mdl.CustomRole, int, error) {
			return []mdl.CustomRole{mockedRole}, 2, nil
		},
	}
	srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), CustomRoleCore: customRoleCore})

	got, err := srvTest.customRoleServiceClient.ListProjectRoleAssignments(
		authCtxForTestUser(t, t.Context()),
		&pb.ListProjectRoleAssignmentsRequest{UserId: uuid.NewString(), PageSize: 1},
	)
	if err != nil {
		t.Fatalf("ListProjectRoleAssignments() error = %v", err)
	}

	want := &pb.ListProjectRoleAssignmentsResponse{
		Roles: []*pb.Role{{
			Id:          mockedRole.ID.String(),
			Name:        mockedRole.Name,
			Permissions: []pb.Permission{pb.Permission_PERMISSION_CUSTOM_ROLE_READ},
			CreateTime:  timestamppb.New(mockedRole.CreatedAt),
			UpdateTime:  timestamppb.New(*mockedRole.UpdatedAt),
			Etag:        mockedRole.ETag,
		}},
		TotalSize:     2,
		NextPageToken: "eyJvIjoxfQ==",
	}

	testingx.AssertDiff(t, got, want, defaultDiffOpts())
}

func TestRoleService_ListProjectRoleAssignments_error(t *testing.T) {
	tests := []struct {
		name           string
		customRoleCore CustomRoleCore
		in             *pb.ListProjectRoleAssignmentsRequest
		want           *status.Status
	}{
		{
			name:           "validated request",
			customRoleCore: &MockedCustomRoleCore{},
			in:             &pb.ListProjectRoleAssignmentsRequest{UserId: "bad"},
			want: status.Convert(invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
				{Field: "user_id", Description: "must be a valid UUID"},
			})),
		},
		{
			name: "not found",
			customRoleCore: &MockedCustomRoleCore{
				UserProjectCustomRolesFunc: func(_ context.Context, _ uuid.UUID, _, _ int) ([]mdl.CustomRole, int, error) {
					return nil, 0, mdl.ErrNotFound
				},
			},
			in:   &pb.ListProjectRoleAssignmentsRequest{UserId: uuid.NewString()},
			want: status.New(codes.NotFound, "user, project, or organization membership not found"),
		},
		{
			name: "core error",
			customRoleCore: &MockedCustomRoleCore{
				UserProjectCustomRolesFunc: func(_ context.Context, _ uuid.UUID, _, _ int) ([]mdl.CustomRole, int, error) {
					return nil, 0, errors.New("boom")
				},
			},
			in:   &pb.ListProjectRoleAssignmentsRequest{UserId: uuid.NewString()},
			want: status.New(codes.Internal, codes.Internal.String()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), CustomRoleCore: tt.customRoleCore})

			_, err := srvTest.customRoleServiceClient.ListProjectRoleAssignments(authCtxForTestUser(t, t.Context()), tt.in)
			if err == nil {
				t.Fatal("ListProjectRoleAssignments() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("ListProjectRoleAssignments() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Proto(), tt.want.Proto(), defaultDiffOpts())
		})
	}
}

func TestRoleService_ListOrganizationRoleAssignments(t *testing.T) {
	mockedRole := mdl.CustomRole{
		ID:          uuid.New(),
		Name:        "organization reader",
		Permissions: []mdl.Permission{mdl.PermissionCustomRoleRead},
		CreatedAt:   time.Now(),
		UpdatedAt:   new(time.Now().Add(time.Minute)),
		ETag:        uuid.NewString(),
	}
	customRoleCore := &MockedCustomRoleCore{
		UserOrgCustomRolesFunc: func(_ context.Context, _ uuid.UUID, _, _ int) ([]mdl.CustomRole, int, error) {
			return []mdl.CustomRole{mockedRole}, 2, nil
		},
	}
	srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), CustomRoleCore: customRoleCore})

	got, err := srvTest.customRoleServiceClient.ListOrganizationRoleAssignments(
		authCtxForTestUser(t, t.Context()),
		&pb.ListOrganizationRoleAssignmentsRequest{UserId: uuid.NewString(), PageSize: 1},
	)
	if err != nil {
		t.Fatalf("ListOrganizationRoleAssignments() error = %v", err)
	}

	want := &pb.ListOrganizationRoleAssignmentsResponse{
		Roles: []*pb.Role{{
			Id:          mockedRole.ID.String(),
			Name:        mockedRole.Name,
			Permissions: []pb.Permission{pb.Permission_PERMISSION_CUSTOM_ROLE_READ},
			CreateTime:  timestamppb.New(mockedRole.CreatedAt),
			UpdateTime:  timestamppb.New(*mockedRole.UpdatedAt),
			Etag:        mockedRole.ETag,
		}},
		TotalSize:     2,
		NextPageToken: "eyJvIjoxfQ==",
	}

	testingx.AssertDiff(t, got, want, defaultDiffOpts())
}

func TestRoleService_ListOrganizationRoleAssignments_error(t *testing.T) {
	tests := []struct {
		name           string
		customRoleCore CustomRoleCore
		in             *pb.ListOrganizationRoleAssignmentsRequest
		want           *status.Status
	}{
		{
			name:           "validated request",
			customRoleCore: &MockedCustomRoleCore{},
			in:             &pb.ListOrganizationRoleAssignmentsRequest{UserId: "bad"},
			want: status.Convert(invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
				{Field: "user_id", Description: "must be a valid UUID"},
			})),
		},
		{
			name: "not found",
			customRoleCore: &MockedCustomRoleCore{
				UserOrgCustomRolesFunc: func(_ context.Context, _ uuid.UUID, _, _ int) ([]mdl.CustomRole, int, error) {
					return nil, 0, mdl.ErrNotFound
				},
			},
			in:   &pb.ListOrganizationRoleAssignmentsRequest{UserId: uuid.NewString()},
			want: status.New(codes.NotFound, "user or organization membership not found"),
		},
		{
			name: "core error",
			customRoleCore: &MockedCustomRoleCore{
				UserOrgCustomRolesFunc: func(_ context.Context, _ uuid.UUID, _, _ int) ([]mdl.CustomRole, int, error) {
					return nil, 0, errors.New("boom")
				},
			},
			in:   &pb.ListOrganizationRoleAssignmentsRequest{UserId: uuid.NewString()},
			want: status.New(codes.Internal, codes.Internal.String()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), CustomRoleCore: tt.customRoleCore})

			_, err := srvTest.customRoleServiceClient.ListOrganizationRoleAssignments(authCtxForTestUser(t, t.Context()), tt.in)
			if err == nil {
				t.Fatal("ListOrganizationRoleAssignments() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("ListOrganizationRoleAssignments() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Proto(), tt.want.Proto(), defaultDiffOpts())
		})
	}
}

func TestRoleService_UpdateRole(t *testing.T) {
	mockedRole := mdl.CustomRole{
		ID:          uuid.New(),
		Name:        "updated role",
		Permissions: []mdl.Permission{mdl.PermissionCustomRoleUpdate},
		CreatedAt:   time.Now(),
		UpdatedAt:   new(time.Now().Add(time.Minute)),
		ETag:        uuid.NewString(),
	}
	customRoleCore := &MockedCustomRoleCore{
		UpdateCustomRoleFunc: func(_ context.Context, _ mdl.UpdateCustomRole) (mdl.CustomRole, error) {
			return mockedRole, nil
		},
	}
	srvTest := NewServerTest(t, ServerConfig{
		Log:            testingx.NewLogger(t),
		CustomRoleCore: customRoleCore,
	})

	got, err := srvTest.customRoleServiceClient.UpdateRole(
		authCtxForTestUser(t, t.Context()),
		&pb.UpdateRoleRequest{
			Role:       &pb.Role{Id: mockedRole.ID.String(), Name: mockedRole.Name},
			UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
		},
	)
	if err != nil {
		t.Fatalf("UpdateRole() error = %v", err)
	}

	want := &pb.Role{
		Id:          mockedRole.ID.String(),
		Name:        mockedRole.Name,
		Permissions: []pb.Permission{pb.Permission_PERMISSION_CUSTOM_ROLE_UPDATE},
		CreateTime:  timestamppb.New(mockedRole.CreatedAt),
		UpdateTime:  timestamppb.New(*mockedRole.UpdatedAt),
		Etag:        mockedRole.ETag,
	}

	testingx.AssertDiff(t, got, want, defaultDiffOpts())
}

func TestRoleService_UpdateRole_error(t *testing.T) {
	invalidArgWithViolation := func(field, desc string) *status.Status {
		st, err := status.New(codes.InvalidArgument, codes.InvalidArgument.String()).WithDetails(
			&errdetails.BadRequest{FieldViolations: []*errdetails.BadRequest_FieldViolation{
				{Field: field, Description: desc},
			}},
		)
		if err != nil {
			t.Fatalf("invalidArgWithViolation(%q, %q) build status error = %v", field, desc, err)
		}
		return st
	}

	roleID := uuid.New()

	tests := []struct {
		name           string
		customRoleCore CustomRoleCore
		in             *pb.UpdateRoleRequest
		want           *status.Status
	}{
		{
			name:           "validated request",
			customRoleCore: &MockedCustomRoleCore{},
			in:             &pb.UpdateRoleRequest{},
			want:           invalidArgWithViolation("role", "required"),
		},
		{
			name: "role not found",
			customRoleCore: &MockedCustomRoleCore{
				UpdateCustomRoleFunc: func(_ context.Context, _ mdl.UpdateCustomRole) (mdl.CustomRole, error) {
					return mdl.CustomRole{}, mdl.ErrNotFound
				},
			},
			in: &pb.UpdateRoleRequest{
				Role:       &pb.Role{Id: roleID.String(), Name: "updated role"},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
			},
			want: status.New(codes.NotFound, `role "`+roleID.String()+`" not found`),
		},
		{
			name: "invalid role",
			customRoleCore: &MockedCustomRoleCore{
				UpdateCustomRoleFunc: func(_ context.Context, _ mdl.UpdateCustomRole) (mdl.CustomRole, error) {
					return mdl.CustomRole{}, mdl.ErrValidation
				},
			},
			in: &pb.UpdateRoleRequest{
				Role:       &pb.Role{Id: roleID.String(), Name: "updated role"},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
			},
			want: status.New(codes.InvalidArgument, "invalid role"),
		},
		{
			name: "already exists",
			customRoleCore: &MockedCustomRoleCore{
				UpdateCustomRoleFunc: func(_ context.Context, _ mdl.UpdateCustomRole) (mdl.CustomRole, error) {
					return mdl.CustomRole{}, mdl.ErrAlreadyExists
				},
			},
			in: &pb.UpdateRoleRequest{
				Role:       &pb.Role{Id: roleID.String(), Name: "updated role"},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
			},
			want: invalidArgWithViolation("role.name", "a role with this name already exists"),
		},
		{
			name: "core error",
			customRoleCore: &MockedCustomRoleCore{
				UpdateCustomRoleFunc: func(_ context.Context, _ mdl.UpdateCustomRole) (mdl.CustomRole, error) {
					return mdl.CustomRole{}, errors.New("boom")
				},
			},
			in: &pb.UpdateRoleRequest{
				Role:       &pb.Role{Id: roleID.String(), Name: "updated role"},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
			},
			want: status.New(codes.Internal, codes.Internal.String()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:            testingx.NewLogger(t),
				CustomRoleCore: tt.customRoleCore,
			})

			_, err := srvTest.customRoleServiceClient.UpdateRole(authCtxForTestUser(t, t.Context()), tt.in)
			if err == nil {
				t.Fatal("UpdateRole() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("UpdateRole() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Proto(), tt.want.Proto(), defaultDiffOpts())
		})
	}
}

func TestRoleService_ModifyRolePermissions(t *testing.T) {
	mockedRole := mdl.CustomRole{
		ID:          uuid.New(),
		Name:        "role manager",
		Permissions: []mdl.Permission{mdl.PermissionCustomRoleUpdate},
		CreatedAt:   time.Now(),
		UpdatedAt:   new(time.Now().Add(time.Minute)),
		ETag:        uuid.NewString(),
	}
	customRoleCore := &MockedCustomRoleCore{
		ModifyCustomRolePermissionsFunc: func(_ context.Context, _ mdl.ModifyCustomRolePermissions) (mdl.CustomRole, error) {
			return mockedRole, nil
		},
	}
	srvTest := NewServerTest(t, ServerConfig{
		Log:            testingx.NewLogger(t),
		CustomRoleCore: customRoleCore,
	})

	got, err := srvTest.customRoleServiceClient.ModifyRolePermissions(
		authCtxForTestUser(t, t.Context()),
		&pb.ModifyRolePermissionsRequest{
			Id:             mockedRole.ID.String(),
			AddPermissions: []pb.Permission{pb.Permission_PERMISSION_CUSTOM_ROLE_UPDATE},
		},
	)
	if err != nil {
		t.Fatalf("ModifyRolePermissions() error = %v", err)
	}

	want := &pb.Role{
		Id:          mockedRole.ID.String(),
		Name:        mockedRole.Name,
		Permissions: []pb.Permission{pb.Permission_PERMISSION_CUSTOM_ROLE_UPDATE},
		CreateTime:  timestamppb.New(mockedRole.CreatedAt),
		UpdateTime:  timestamppb.New(*mockedRole.UpdatedAt),
		Etag:        mockedRole.ETag,
	}

	testingx.AssertDiff(t, got, want, defaultDiffOpts())
}

func TestRoleService_ModifyRolePermissions_error(t *testing.T) {
	invalidArgWithViolation := func(field, desc string) *status.Status {
		st, err := status.New(codes.InvalidArgument, codes.InvalidArgument.String()).WithDetails(
			&errdetails.BadRequest{FieldViolations: []*errdetails.BadRequest_FieldViolation{
				{Field: field, Description: desc},
			}},
		)
		if err != nil {
			t.Fatalf("invalidArgWithViolation(%q, %q) build status error = %v", field, desc, err)
		}
		return st
	}

	roleID := uuid.New()

	tests := []struct {
		name           string
		customRoleCore CustomRoleCore
		in             *pb.ModifyRolePermissionsRequest
		want           *status.Status
	}{
		{
			name:           "validated request",
			customRoleCore: &MockedCustomRoleCore{},
			in:             &pb.ModifyRolePermissionsRequest{Id: "bad"},
			want:           invalidArgWithViolation("id", "must be a valid UUID"),
		},
		{
			name: "missing role",
			customRoleCore: &MockedCustomRoleCore{
				ModifyCustomRolePermissionsFunc: func(_ context.Context, _ mdl.ModifyCustomRolePermissions) (mdl.CustomRole, error) {
					return mdl.CustomRole{}, mdl.ErrNotFound
				},
			},
			in:   &pb.ModifyRolePermissionsRequest{Id: roleID.String()},
			want: status.New(codes.NotFound, `role "`+roleID.String()+`" or permission not found`),
		},
		{
			name: "invalid permission changes",
			customRoleCore: &MockedCustomRoleCore{
				ModifyCustomRolePermissionsFunc: func(_ context.Context, _ mdl.ModifyCustomRolePermissions) (mdl.CustomRole, error) {
					return mdl.CustomRole{}, mdl.ErrValidation
				},
			},
			in:   &pb.ModifyRolePermissionsRequest{Id: roleID.String()},
			want: status.New(codes.InvalidArgument, "invalid permission changes"),
		},
		{
			name: "core error",
			customRoleCore: &MockedCustomRoleCore{
				ModifyCustomRolePermissionsFunc: func(_ context.Context, _ mdl.ModifyCustomRolePermissions) (mdl.CustomRole, error) {
					return mdl.CustomRole{}, errors.New("boom")
				},
			},
			in:   &pb.ModifyRolePermissionsRequest{Id: roleID.String()},
			want: status.New(codes.Internal, codes.Internal.String()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:            testingx.NewLogger(t),
				CustomRoleCore: tt.customRoleCore,
			})

			_, err := srvTest.customRoleServiceClient.ModifyRolePermissions(authCtxForTestUser(t, t.Context()), tt.in)
			if err == nil {
				t.Fatal("ModifyRolePermissions() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("ModifyRolePermissions() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Proto(), tt.want.Proto(), defaultDiffOpts())
		})
	}
}

func TestRoleService_DeleteRole(t *testing.T) {
	customRoleCore := &MockedCustomRoleCore{
		DeleteCustomRoleFunc: func(_ context.Context, _ uuid.UUID) error {
			return nil
		},
	}
	srvTest := NewServerTest(t, ServerConfig{
		Log:            testingx.NewLogger(t),
		CustomRoleCore: customRoleCore,
	})

	got, err := srvTest.customRoleServiceClient.DeleteRole(
		authCtxForTestUser(t, t.Context()),
		&pb.DeleteRoleRequest{Id: uuid.NewString()},
	)
	if err != nil {
		t.Fatalf("DeleteRole() error = %v", err)
	}

	want := &pb.DeleteRoleResponse{}

	testingx.AssertDiff(t, got, want, defaultDiffOpts())
}

func TestRoleService_DeleteRole_error(t *testing.T) {
	invalidArgWithViolation := func(field, desc string) *status.Status {
		st, err := status.New(codes.InvalidArgument, codes.InvalidArgument.String()).WithDetails(
			&errdetails.BadRequest{FieldViolations: []*errdetails.BadRequest_FieldViolation{
				{Field: field, Description: desc},
			}},
		)
		if err != nil {
			t.Fatalf("invalidArgWithViolation(%q, %q) build status error = %v", field, desc, err)
		}
		return st
	}

	roleID := uuid.New()

	tests := []struct {
		name           string
		customRoleCore CustomRoleCore
		in             *pb.DeleteRoleRequest
		want           *status.Status
	}{
		{
			name:           "validated request",
			customRoleCore: &MockedCustomRoleCore{},
			in:             &pb.DeleteRoleRequest{Id: "bad"},
			want:           invalidArgWithViolation("id", "must be a valid UUID"),
		},
		{
			name: "missing role",
			customRoleCore: &MockedCustomRoleCore{
				DeleteCustomRoleFunc: func(_ context.Context, _ uuid.UUID) error {
					return mdl.ErrNotFound
				},
			},
			in:   &pb.DeleteRoleRequest{Id: roleID.String()},
			want: status.New(codes.NotFound, `role "`+roleID.String()+`" not found`),
		},
		{
			name: "core error",
			customRoleCore: &MockedCustomRoleCore{
				DeleteCustomRoleFunc: func(_ context.Context, _ uuid.UUID) error {
					return errors.New("boom")
				},
			},
			in:   &pb.DeleteRoleRequest{Id: roleID.String()},
			want: status.New(codes.Internal, codes.Internal.String()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:            testingx.NewLogger(t),
				CustomRoleCore: tt.customRoleCore,
			})

			_, err := srvTest.customRoleServiceClient.DeleteRole(authCtxForTestUser(t, t.Context()), tt.in)
			if err == nil {
				t.Fatal("DeleteRole() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("DeleteRole() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Proto(), tt.want.Proto(), defaultDiffOpts())
		})
	}
}

func TestRoleService_AssignRoleToProject(t *testing.T) {
	customRoleCore := &MockedCustomRoleCore{
		AssignCustomRoleToProjectFunc: func(_ context.Context, _, _ uuid.UUID) error { return nil },
	}
	srvTest := NewServerTest(t, ServerConfig{
		Log:            testingx.NewLogger(t),
		CustomRoleCore: customRoleCore,
	})

	got, err := srvTest.customRoleServiceClient.AssignRoleToProject(
		authCtxForTestUser(t, t.Context()),
		&pb.AssignRoleToProjectRequest{RoleId: uuid.NewString(), UserId: uuid.NewString()},
	)
	if err != nil {
		t.Fatalf("AssignRoleToProject() error = %v", err)
	}

	want := &pb.AssignRoleToProjectResponse{}

	testingx.AssertDiff(t, got, want, defaultDiffOpts())
}

func TestRoleService_AssignRoleToProject_error(t *testing.T) {
	tests := []struct {
		name           string
		customRoleCore CustomRoleCore
		in             *pb.AssignRoleToProjectRequest
		want           *status.Status
	}{
		{
			name:           "validated request",
			customRoleCore: &MockedCustomRoleCore{},
			in:             &pb.AssignRoleToProjectRequest{RoleId: "not-a-uuid", UserId: uuid.NewString()},
			want: status.Convert(invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
				{Field: "role_id", Description: "must be a valid UUID"},
			})),
		},
		{
			name: "not found",
			customRoleCore: &MockedCustomRoleCore{
				AssignCustomRoleToProjectFunc: func(_ context.Context, _, _ uuid.UUID) error {
					return mdl.ErrNotFound
				},
			},
			in:   &pb.AssignRoleToProjectRequest{RoleId: uuid.NewString(), UserId: uuid.NewString()},
			want: status.New(codes.NotFound, "user, role, project, or organization membership not found"),
		},
		{
			name: "already assigned",
			customRoleCore: &MockedCustomRoleCore{
				AssignCustomRoleToProjectFunc: func(_ context.Context, _, _ uuid.UUID) error {
					return mdl.ErrAlreadyExists
				},
			},
			in:   &pb.AssignRoleToProjectRequest{RoleId: uuid.NewString(), UserId: uuid.NewString()},
			want: status.New(codes.AlreadyExists, "user already has role in project"),
		},
		{
			name: "core error",
			customRoleCore: &MockedCustomRoleCore{
				AssignCustomRoleToProjectFunc: func(_ context.Context, _, _ uuid.UUID) error {
					return errors.New("boom")
				},
			},
			in:   &pb.AssignRoleToProjectRequest{RoleId: uuid.NewString(), UserId: uuid.NewString()},
			want: status.New(codes.Internal, codes.Internal.String()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:            testingx.NewLogger(t),
				CustomRoleCore: tt.customRoleCore,
			})

			_, err := srvTest.customRoleServiceClient.AssignRoleToProject(authCtxForTestUser(t, t.Context()), tt.in)
			if err == nil {
				t.Fatal("AssignRoleToProject() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("AssignRoleToProject() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Proto(), tt.want.Proto(), defaultDiffOpts())
		})
	}
}

func TestRoleService_UnassignRoleFromProject(t *testing.T) {
	customRoleCore := &MockedCustomRoleCore{
		UnassignCustomRoleFromProjectFunc: func(_ context.Context, _, _ uuid.UUID) error { return nil },
	}
	srvTest := NewServerTest(t, ServerConfig{
		Log:            testingx.NewLogger(t),
		CustomRoleCore: customRoleCore,
	})

	got, err := srvTest.customRoleServiceClient.UnassignRoleFromProject(
		authCtxForTestUser(t, t.Context()),
		&pb.UnassignRoleFromProjectRequest{RoleId: uuid.NewString(), UserId: uuid.NewString()},
	)
	if err != nil {
		t.Fatalf("UnassignRoleFromProject() error = %v", err)
	}

	want := &pb.UnassignRoleFromProjectResponse{}

	testingx.AssertDiff(t, got, want, defaultDiffOpts())
}

func TestRoleService_UnassignRoleFromProject_error(t *testing.T) {
	tests := []struct {
		name           string
		customRoleCore CustomRoleCore
		in             *pb.UnassignRoleFromProjectRequest
		want           *status.Status
	}{
		{
			name:           "validated request",
			customRoleCore: &MockedCustomRoleCore{},
			in:             &pb.UnassignRoleFromProjectRequest{RoleId: "not-a-uuid", UserId: uuid.NewString()},
			want: status.Convert(invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
				{Field: "role_id", Description: "must be a valid UUID"},
			})),
		},
		{
			name: "not found",
			customRoleCore: &MockedCustomRoleCore{
				UnassignCustomRoleFromProjectFunc: func(_ context.Context, _, _ uuid.UUID) error {
					return mdl.ErrNotFound
				},
			},
			in:   &pb.UnassignRoleFromProjectRequest{RoleId: uuid.NewString(), UserId: uuid.NewString()},
			want: status.New(codes.NotFound, "project role assignment not found"),
		},
		{
			name: "core error",
			customRoleCore: &MockedCustomRoleCore{
				UnassignCustomRoleFromProjectFunc: func(_ context.Context, _, _ uuid.UUID) error {
					return errors.New("boom")
				},
			},
			in:   &pb.UnassignRoleFromProjectRequest{RoleId: uuid.NewString(), UserId: uuid.NewString()},
			want: status.New(codes.Internal, codes.Internal.String()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:            testingx.NewLogger(t),
				CustomRoleCore: tt.customRoleCore,
			})

			_, err := srvTest.customRoleServiceClient.UnassignRoleFromProject(authCtxForTestUser(t, t.Context()), tt.in)
			if err == nil {
				t.Fatal("UnassignRoleFromProject() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("UnassignRoleFromProject() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Proto(), tt.want.Proto(), defaultDiffOpts())
		})
	}
}

func TestRoleService_AssignRoleToOrganization(t *testing.T) {
	customRoleCore := &MockedCustomRoleCore{
		AssignCustomRoleToOrgFunc: func(_ context.Context, _, _ uuid.UUID) error { return nil },
	}
	srvTest := NewServerTest(t, ServerConfig{
		Log:            testingx.NewLogger(t),
		CustomRoleCore: customRoleCore,
	})

	got, err := srvTest.customRoleServiceClient.AssignRoleToOrganization(
		authCtxForTestUser(t, t.Context()),
		&pb.AssignRoleToOrganizationRequest{RoleId: uuid.NewString(), UserId: uuid.NewString()},
	)
	if err != nil {
		t.Fatalf("AssignRoleToOrganization() error = %v", err)
	}

	want := &pb.AssignRoleToOrganizationResponse{}

	testingx.AssertDiff(t, got, want, defaultDiffOpts())
}

func TestRoleService_AssignRoleToOrganization_error(t *testing.T) {
	tests := []struct {
		name           string
		customRoleCore CustomRoleCore
		in             *pb.AssignRoleToOrganizationRequest
		want           *status.Status
	}{
		{
			name:           "validated request",
			customRoleCore: &MockedCustomRoleCore{},
			in:             &pb.AssignRoleToOrganizationRequest{RoleId: "not-a-uuid", UserId: uuid.NewString()},
			want: status.Convert(invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
				{Field: "role_id", Description: "must be a valid UUID"},
			})),
		},
		{
			name: "not found",
			customRoleCore: &MockedCustomRoleCore{
				AssignCustomRoleToOrgFunc: func(_ context.Context, _, _ uuid.UUID) error {
					return mdl.ErrNotFound
				},
			},
			in:   &pb.AssignRoleToOrganizationRequest{RoleId: uuid.NewString(), UserId: uuid.NewString()},
			want: status.New(codes.NotFound, "user, role, organization, or organization membership not found"),
		},
		{
			name: "already assigned",
			customRoleCore: &MockedCustomRoleCore{
				AssignCustomRoleToOrgFunc: func(_ context.Context, _, _ uuid.UUID) error {
					return mdl.ErrAlreadyExists
				},
			},
			in:   &pb.AssignRoleToOrganizationRequest{RoleId: uuid.NewString(), UserId: uuid.NewString()},
			want: status.New(codes.AlreadyExists, "user already has role in organization"),
		},
		{
			name: "core error",
			customRoleCore: &MockedCustomRoleCore{
				AssignCustomRoleToOrgFunc: func(_ context.Context, _, _ uuid.UUID) error {
					return errors.New("boom")
				},
			},
			in:   &pb.AssignRoleToOrganizationRequest{RoleId: uuid.NewString(), UserId: uuid.NewString()},
			want: status.New(codes.Internal, codes.Internal.String()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:            testingx.NewLogger(t),
				CustomRoleCore: tt.customRoleCore,
			})

			_, err := srvTest.customRoleServiceClient.AssignRoleToOrganization(authCtxForTestUser(t, t.Context()), tt.in)
			if err == nil {
				t.Fatal("AssignRoleToOrganization() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("AssignRoleToOrganization() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Proto(), tt.want.Proto(), defaultDiffOpts())
		})
	}
}

func TestRoleService_UnassignRoleFromOrganization(t *testing.T) {
	customRoleCore := &MockedCustomRoleCore{
		UnassignCustomRoleFromOrgFunc: func(_ context.Context, _, _ uuid.UUID) error { return nil },
	}
	srvTest := NewServerTest(t, ServerConfig{
		Log:            testingx.NewLogger(t),
		CustomRoleCore: customRoleCore,
	})

	got, err := srvTest.customRoleServiceClient.UnassignRoleFromOrganization(
		authCtxForTestUser(t, t.Context()),
		&pb.UnassignRoleFromOrganizationRequest{RoleId: uuid.NewString(), UserId: uuid.NewString()},
	)
	if err != nil {
		t.Fatalf("UnassignRoleFromOrganization() error = %v", err)
	}

	want := &pb.UnassignRoleFromOrganizationResponse{}

	testingx.AssertDiff(t, got, want, defaultDiffOpts())
}

func TestRoleService_UnassignRoleFromOrganization_error(t *testing.T) {
	tests := []struct {
		name           string
		customRoleCore CustomRoleCore
		in             *pb.UnassignRoleFromOrganizationRequest
		want           *status.Status
	}{
		{
			name:           "validated request",
			customRoleCore: &MockedCustomRoleCore{},
			in:             &pb.UnassignRoleFromOrganizationRequest{RoleId: "not-a-uuid", UserId: uuid.NewString()},
			want: status.Convert(invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
				{Field: "role_id", Description: "must be a valid UUID"},
			})),
		},
		{
			name: "not found",
			customRoleCore: &MockedCustomRoleCore{
				UnassignCustomRoleFromOrgFunc: func(_ context.Context, _, _ uuid.UUID) error {
					return mdl.ErrNotFound
				},
			},
			in:   &pb.UnassignRoleFromOrganizationRequest{RoleId: uuid.NewString(), UserId: uuid.NewString()},
			want: status.New(codes.NotFound, "organization role assignment not found"),
		},
		{
			name: "core error",
			customRoleCore: &MockedCustomRoleCore{
				UnassignCustomRoleFromOrgFunc: func(_ context.Context, _, _ uuid.UUID) error {
					return errors.New("boom")
				},
			},
			in:   &pb.UnassignRoleFromOrganizationRequest{RoleId: uuid.NewString(), UserId: uuid.NewString()},
			want: status.New(codes.Internal, codes.Internal.String()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:            testingx.NewLogger(t),
				CustomRoleCore: tt.customRoleCore,
			})

			_, err := srvTest.customRoleServiceClient.UnassignRoleFromOrganization(authCtxForTestUser(t, t.Context()), tt.in)
			if err == nil {
				t.Fatal("UnassignRoleFromOrganization() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("UnassignRoleFromOrganization() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Proto(), tt.want.Proto(), defaultDiffOpts())
		})
	}
}
