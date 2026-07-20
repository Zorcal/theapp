package grpc

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestRoleService_CreateRole(t *testing.T) {
	diffOpts := defaultDiffOpts()

	id, now := uuid.New(), time.Now()

	roleCore := &MockedRoleCore{
		CreateRoleFunc: func(_ context.Context, orgID int, cr mdl.CreateRole) (mdl.RoleCustom, error) {
			if orgID != testOrgID {
				t.Errorf("CreateRole() orgID = %d, want %d", orgID, testOrgID)
			}
			return mdl.RoleCustom{
				ID:          id,
				Name:        cr.Name,
				Permissions: cr.Permissions,
				CreatedAt:   now,
			}, nil
		},
	}

	srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), RoleCore: roleCore})

	got, err := srvTest.roleServiceClient.CreateRole(authCtxForTestUser(t, t.Context()), &pb.CreateRoleRequest{
		Role: &pb.Role{Name: "viewer", Permissions: []string{"user:read"}},
	})
	if err != nil {
		t.Fatalf("CreateRole() error = %q, want no error", err)
	}

	want := &pb.Role{
		Id:         id.String(),
		Name:       "viewer",
		CreateTime: timestamppb.New(now),
	}

	testingx.AssertDiff(t, got, want, diffOpts)
}

func TestRoleService_CreateRole_error(t *testing.T) {
	tests := []struct {
		name     string
		roleCore RoleCore
		in       *pb.CreateRoleRequest
		want     *status.Status
	}{
		{
			name:     "missing role",
			roleCore: &MockedRoleCore{},
			in:       &pb.CreateRoleRequest{},
			want:     status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name: "validation error",
			roleCore: &MockedRoleCore{
				CreateRoleFunc: func(_ context.Context, _ int, _ mdl.CreateRole) (mdl.RoleCustom, error) {
					return mdl.RoleCustom{}, mdl.ErrValidation
				},
			},
			in:   &pb.CreateRoleRequest{Role: &pb.Role{Name: "viewer", Permissions: []string{"user:read"}}},
			want: status.New(codes.InvalidArgument, "invalid request"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), RoleCore: tt.roleCore})

			_, err := srvTest.roleServiceClient.CreateRole(authCtxForTestUser(t, t.Context()), tt.in)

			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("CreateRole() error = %v, want grpc status", err)
			}
			if st.Code() != tt.want.Code() {
				t.Errorf("CreateRole() code = %v, want %v", st.Code(), tt.want.Code())
			}
		})
	}
}

func TestRoleService_UpdateRole(t *testing.T) {
	diffOpts := defaultDiffOpts()

	id, now := uuid.New(), time.Now()

	roleCore := &MockedRoleCore{
		UpdateRoleFunc: func(_ context.Context, orgID int, ur mdl.UpdateRole) (mdl.RoleCustom, error) {
			if orgID != testOrgID {
				t.Errorf("UpdateRole() orgID = %d, want %d", orgID, testOrgID)
			}
			if !ur.Fields.Name || !ur.Fields.Permissions {
				t.Errorf("UpdateRole() Fields = %+v, want Name and Permissions set", ur.Fields)
			}
			return mdl.RoleCustom{
				ID:          ur.ID,
				Name:        ur.Name,
				Permissions: ur.Permissions,
				UpdatedAt:   &now,
			}, nil
		},
	}

	srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), RoleCore: roleCore})

	got, err := srvTest.roleServiceClient.UpdateRole(authCtxForTestUser(t, t.Context()), &pb.UpdateRoleRequest{
		Role:       &pb.Role{Id: id.String(), Name: "viewer-renamed", Permissions: []string{"role:read"}},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name", "permissions"}},
	})
	if err != nil {
		t.Fatalf("UpdateRole() error = %q, want no error", err)
	}

	want := &pb.Role{
		Id:         id.String(),
		Name:       "viewer-renamed",
		CreateTime: timestamppb.New(time.Time{}),
		UpdateTime: timestamppb.New(now),
	}

	testingx.AssertDiff(t, got, want, diffOpts)
}

func TestRoleService_UpdateRole_partialMask(t *testing.T) {
	diffOpts := defaultDiffOpts()

	id, now := uuid.New(), time.Now()

	roleCore := &MockedRoleCore{
		UpdateRoleFunc: func(_ context.Context, _ int, ur mdl.UpdateRole) (mdl.RoleCustom, error) {
			if !ur.Fields.Name || ur.Fields.Permissions {
				t.Errorf("UpdateRole() Fields = %+v, want only Name set", ur.Fields)
			}
			return mdl.RoleCustom{
				ID:          ur.ID,
				Name:        ur.Name,
				Permissions: []mdl.Permission{mdl.PermissionRoleRead}, // carried over from the existing role
				UpdatedAt:   &now,
			}, nil
		},
	}

	srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), RoleCore: roleCore})

	got, err := srvTest.roleServiceClient.UpdateRole(authCtxForTestUser(t, t.Context()), &pb.UpdateRoleRequest{
		Role:       &pb.Role{Id: id.String(), Name: "viewer-renamed"},
		UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
	})
	if err != nil {
		t.Fatalf("UpdateRole() error = %q, want no error", err)
	}

	want := &pb.Role{
		Id:         id.String(),
		Name:       "viewer-renamed",
		CreateTime: timestamppb.New(time.Time{}),
		UpdateTime: timestamppb.New(now),
	}

	testingx.AssertDiff(t, got, want, diffOpts)
}

func TestRoleService_UpdateRole_error(t *testing.T) {
	id := uuid.New().String()

	tests := []struct {
		name     string
		roleCore RoleCore
		in       *pb.UpdateRoleRequest
		want     *status.Status
	}{
		{
			name:     "missing role",
			roleCore: &MockedRoleCore{},
			in:       &pb.UpdateRoleRequest{},
			want:     status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name:     "invalid id",
			roleCore: &MockedRoleCore{},
			in:       &pb.UpdateRoleRequest{Role: &pb.Role{Id: "not-a-uuid", Name: "viewer"}},
			want:     status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name:     "missing update_mask",
			roleCore: &MockedRoleCore{},
			in:       &pb.UpdateRoleRequest{Role: &pb.Role{Id: id, Name: "viewer"}},
			want:     status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name:     "invalid update_mask field",
			roleCore: &MockedRoleCore{},
			in: &pb.UpdateRoleRequest{
				Role:       &pb.Role{Id: id, Name: "viewer", Permissions: []string{"role:read"}},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name", "permissions", "is_static"}},
			},
			want: status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name:     "permissions in mask but empty",
			roleCore: &MockedRoleCore{},
			in: &pb.UpdateRoleRequest{
				Role:       &pb.Role{Id: id, Name: "viewer"},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"permissions"}},
			},
			want: status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name: "not found",
			roleCore: &MockedRoleCore{
				UpdateRoleFunc: func(_ context.Context, _ int, _ mdl.UpdateRole) (mdl.RoleCustom, error) {
					return mdl.RoleCustom{}, mdl.ErrNotFound
				},
			},
			in: &pb.UpdateRoleRequest{
				Role:       &pb.Role{Id: id, Name: "viewer", Permissions: []string{"role:read"}},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name", "permissions"}},
			},
			want: status.New(codes.NotFound, "not found"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), RoleCore: tt.roleCore})

			_, err := srvTest.roleServiceClient.UpdateRole(authCtxForTestUser(t, t.Context()), tt.in)

			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("UpdateRole() error = %v, want grpc status", err)
			}
			if st.Code() != tt.want.Code() {
				t.Errorf("UpdateRole() code = %v, want %v", st.Code(), tt.want.Code())
			}
		})
	}
}

func TestRoleService_ModifyRolePermissions(t *testing.T) {
	diffOpts := defaultDiffOpts()

	id, now := uuid.New(), time.Now()

	roleCore := &MockedRoleCore{
		ModifyRolePermissionsFunc: func(_ context.Context, orgID int, m mdl.ModifyRolePermissions) (mdl.RoleCustom, error) {
			if orgID != testOrgID {
				t.Errorf("ModifyRolePermissions() orgID = %d, want %d", orgID, testOrgID)
			}
			if m.ID != id {
				t.Errorf("ModifyRolePermissions() ID = %v, want %v", m.ID, id)
			}
			return mdl.RoleCustom{
				ID:          m.ID,
				Name:        "viewer",
				Permissions: []mdl.Permission{mdl.PermissionRoleRead},
				UpdatedAt:   &now,
			}, nil
		},
	}

	srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), RoleCore: roleCore})

	got, err := srvTest.roleServiceClient.ModifyRolePermissions(authCtxForTestUser(t, t.Context()), &pb.ModifyRolePermissionsRequest{
		Id:                id.String(),
		AddPermissions:    []string{"role:read"},
		RemovePermissions: []string{"role:create"},
	})
	if err != nil {
		t.Fatalf("ModifyRolePermissions() error = %q, want no error", err)
	}

	want := &pb.Role{
		Id:         id.String(),
		Name:       "viewer",
		CreateTime: timestamppb.New(time.Time{}),
		UpdateTime: timestamppb.New(now),
	}

	testingx.AssertDiff(t, got, want, diffOpts)
}

func TestRoleService_ModifyRolePermissions_error(t *testing.T) {
	id := uuid.New().String()

	tests := []struct {
		name     string
		roleCore RoleCore
		in       *pb.ModifyRolePermissionsRequest
		want     *status.Status
	}{
		{
			name:     "invalid id",
			roleCore: &MockedRoleCore{},
			in:       &pb.ModifyRolePermissionsRequest{Id: "not-a-uuid"},
			want:     status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name: "not found",
			roleCore: &MockedRoleCore{
				ModifyRolePermissionsFunc: func(_ context.Context, _ int, _ mdl.ModifyRolePermissions) (mdl.RoleCustom, error) {
					return mdl.RoleCustom{}, mdl.ErrNotFound
				},
			},
			in:   &pb.ModifyRolePermissionsRequest{Id: id, AddPermissions: []string{"role:read"}},
			want: status.New(codes.NotFound, "not found"),
		},
		{
			name: "validation error",
			roleCore: &MockedRoleCore{
				ModifyRolePermissionsFunc: func(_ context.Context, _ int, _ mdl.ModifyRolePermissions) (mdl.RoleCustom, error) {
					return mdl.RoleCustom{}, mdl.ErrValidation
				},
			},
			in:   &pb.ModifyRolePermissionsRequest{Id: id, AddPermissions: []string{"role:read"}},
			want: status.New(codes.InvalidArgument, "invalid request"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), RoleCore: tt.roleCore})

			_, err := srvTest.roleServiceClient.ModifyRolePermissions(authCtxForTestUser(t, t.Context()), tt.in)

			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("ModifyRolePermissions() error = %v, want grpc status", err)
			}
			if st.Code() != tt.want.Code() {
				t.Errorf("ModifyRolePermissions() code = %v, want %v", st.Code(), tt.want.Code())
			}
		})
	}
}

func TestRoleService_DeleteRole(t *testing.T) {
	id := uuid.New()

	roleCore := &MockedRoleCore{
		DeleteRoleFunc: func(_ context.Context, orgID int, gotID uuid.UUID) error {
			if orgID != testOrgID {
				t.Errorf("DeleteRole() orgID = %d, want %d", orgID, testOrgID)
			}
			if gotID != id {
				t.Errorf("DeleteRole() id = %v, want %v", gotID, id)
			}
			return nil
		},
	}

	srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), RoleCore: roleCore})

	got, err := srvTest.roleServiceClient.DeleteRole(authCtxForTestUser(t, t.Context()), &pb.DeleteRoleRequest{Id: id.String()})
	if err != nil {
		t.Fatalf("DeleteRole() error = %q, want no error", err)
	}

	testingx.AssertDiff(t, got, &emptypb.Empty{}, defaultDiffOpts())
}

func TestRoleService_DeleteRole_error(t *testing.T) {
	id := uuid.New().String()

	tests := []struct {
		name     string
		roleCore RoleCore
		in       *pb.DeleteRoleRequest
		want     *status.Status
	}{
		{
			name:     "invalid id",
			roleCore: &MockedRoleCore{},
			in:       &pb.DeleteRoleRequest{Id: "not-a-uuid"},
			want:     status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name: "not found",
			roleCore: &MockedRoleCore{
				DeleteRoleFunc: func(_ context.Context, _ int, _ uuid.UUID) error {
					return mdl.ErrNotFound
				},
			},
			in:   &pb.DeleteRoleRequest{Id: id},
			want: status.New(codes.NotFound, "not found"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), RoleCore: tt.roleCore})

			_, err := srvTest.roleServiceClient.DeleteRole(authCtxForTestUser(t, t.Context()), tt.in)

			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("DeleteRole() error = %v, want grpc status", err)
			}
			if st.Code() != tt.want.Code() {
				t.Errorf("DeleteRole() code = %v, want %v", st.Code(), tt.want.Code())
			}
		})
	}
}

func TestRoleService_ListRoles(t *testing.T) {
	diffOpts := defaultDiffOpts()

	id1, id2 := uuid.New(), uuid.New()

	roleCore := &MockedRoleCore{
		OrgRolesFunc: func(_ context.Context, orgID, pageSize, pageOffset int) ([]mdl.RoleCustom, int, error) {
			if orgID != testOrgID {
				t.Errorf("OrgRoles() orgID = %d, want %d", orgID, testOrgID)
			}
			roles := []mdl.RoleCustom{
				{ID: id1, Name: "viewer", Permissions: []mdl.Permission{mdl.PermissionRoleRead}},
				{ID: id2, Name: "editor", Permissions: []mdl.Permission{mdl.PermissionRoleRead, mdl.PermissionRoleUpdate}},
			}
			end := min(pageOffset+pageSize, len(roles))
			if pageOffset >= len(roles) {
				return nil, len(roles), nil
			}
			return roles[pageOffset:end], len(roles), nil
		},
	}

	srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), RoleCore: roleCore})

	got, err := srvTest.roleServiceClient.ListRoles(authCtxForTestUser(t, t.Context()), &pb.ListRolesRequest{})
	if err != nil {
		t.Fatalf("ListRoles() error = %q, want no error", err)
	}

	zeroTime := timestamppb.New(time.Time{})
	want := &pb.ListRolesResponse{
		Roles: []*pb.Role{
			{Id: id1.String(), Name: "viewer", CreateTime: zeroTime},
			{Id: id2.String(), Name: "editor", CreateTime: zeroTime},
		},
		TotalSize: 2,
	}

	testingx.AssertDiff(t, got, want, diffOpts)
}

func TestRoleService_ListRoles_pagination(t *testing.T) {
	diffOpts := defaultDiffOpts()

	id1, id2, id3 := uuid.New(), uuid.New(), uuid.New()

	roleCore := &MockedRoleCore{
		OrgRolesFunc: func(_ context.Context, _, pageSize, pageOffset int) ([]mdl.RoleCustom, int, error) {
			roles := []mdl.RoleCustom{
				{ID: id1, Name: "role-1", Permissions: []mdl.Permission{mdl.PermissionRoleRead}},
				{ID: id2, Name: "role-2", Permissions: []mdl.Permission{mdl.PermissionRoleRead}},
				{ID: id3, Name: "role-3", Permissions: []mdl.Permission{mdl.PermissionRoleRead}},
			}
			end := min(pageOffset+pageSize, len(roles))
			if pageOffset >= len(roles) {
				return nil, len(roles), nil
			}
			return roles[pageOffset:end], len(roles), nil
		},
	}

	srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), RoleCore: roleCore})

	page1, err := srvTest.roleServiceClient.ListRoles(authCtxForTestUser(t, t.Context()), &pb.ListRolesRequest{PageSize: 2})
	if err != nil {
		t.Fatalf("ListRoles() error = %q, want no error", err)
	}
	if len(page1.GetRoles()) != 2 {
		t.Fatalf("ListRoles() page 1 len = %d, want 2", len(page1.GetRoles()))
	}
	if page1.GetNextPageToken() == "" {
		t.Fatalf("ListRoles() page 1 next_page_token = empty, want non-empty")
	}

	page2, err := srvTest.roleServiceClient.ListRoles(authCtxForTestUser(t, t.Context()), &pb.ListRolesRequest{
		PageSize:  2,
		PageToken: page1.GetNextPageToken(),
	})
	if err != nil {
		t.Fatalf("ListRoles() page 2 error = %q, want no error", err)
	}

	want := &pb.ListRolesResponse{
		Roles:     []*pb.Role{{Id: id3.String(), Name: "role-3", CreateTime: timestamppb.New(time.Time{})}},
		TotalSize: 3,
	}
	testingx.AssertDiff(t, page2, want, diffOpts)
}

func TestRoleService_AssignRole(t *testing.T) {
	roleID, userID := uuid.New(), uuid.New()

	roleCore := &MockedRoleCore{
		AssignRoleFunc: func(_ context.Context, orgID int, in mdl.AssignRole) error {
			if orgID != testOrgID {
				t.Errorf("AssignRole() orgID = %d, want %d", orgID, testOrgID)
			}
			if in.RoleID != roleID {
				t.Errorf("AssignRole() RoleID = %v, want %v", in.RoleID, roleID)
			}
			if in.UserID != userID {
				t.Errorf("AssignRole() UserID = %v, want %v", in.UserID, userID)
			}
			if in.Scope.ProjectID == nil || *in.Scope.ProjectID != 42 {
				t.Errorf("AssignRole() Scope.ProjectID = %v, want 42", in.Scope.ProjectID)
			}
			return nil
		},
	}

	srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), RoleCore: roleCore})

	got, err := srvTest.roleServiceClient.AssignRole(authCtxForTestUser(t, t.Context()), &pb.AssignRoleRequest{
		RoleId: roleID.String(),
		UserId: userID.String(),
		Scope:  &pb.AssignRoleRequest_ProjectId{ProjectId: 42},
	})
	if err != nil {
		t.Fatalf("AssignRole() error = %q, want no error", err)
	}

	testingx.AssertDiff(t, got, &emptypb.Empty{}, defaultDiffOpts())
}

func TestRoleService_AssignRole_error(t *testing.T) {
	roleID, userID := uuid.New().String(), uuid.New().String()

	tests := []struct {
		name     string
		roleCore RoleCore
		in       *pb.AssignRoleRequest
		want     *status.Status
	}{
		{
			name:     "invalid role id",
			roleCore: &MockedRoleCore{},
			in:       &pb.AssignRoleRequest{RoleId: "not-a-uuid", UserId: userID, Scope: &pb.AssignRoleRequest_ProjectId{ProjectId: 42}},
			want:     status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name:     "invalid user id",
			roleCore: &MockedRoleCore{},
			in:       &pb.AssignRoleRequest{RoleId: roleID, UserId: "not-a-uuid", Scope: &pb.AssignRoleRequest_ProjectId{ProjectId: 42}},
			want:     status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name: "not found",
			roleCore: &MockedRoleCore{
				AssignRoleFunc: func(_ context.Context, _ int, _ mdl.AssignRole) error {
					return mdl.ErrNotFound
				},
			},
			in:   &pb.AssignRoleRequest{RoleId: roleID, UserId: userID, Scope: &pb.AssignRoleRequest_ProjectId{ProjectId: 42}},
			want: status.New(codes.NotFound, "role or user not found"),
		},
		{
			name: "role scope conflict",
			roleCore: &MockedRoleCore{
				AssignRoleFunc: func(_ context.Context, _ int, _ mdl.AssignRole) error {
					return mdl.ErrRoleScopeConflict
				},
			},
			in:   &pb.AssignRoleRequest{RoleId: roleID, UserId: userID, Scope: &pb.AssignRoleRequest_ProjectId{ProjectId: 42}},
			want: status.New(codes.FailedPrecondition, "role already assigned at org scope"),
		},
		{
			name: "not org member",
			roleCore: &MockedRoleCore{
				AssignRoleFunc: func(_ context.Context, _ int, _ mdl.AssignRole) error {
					return mdl.ErrNotOrgMember
				},
			},
			in:   &pb.AssignRoleRequest{RoleId: roleID, UserId: userID, Scope: &pb.AssignRoleRequest_OrgId{OrgId: 42}},
			want: status.New(codes.FailedPrecondition, "user is not a member of the organization"),
		},
		{
			name: "validation error",
			roleCore: &MockedRoleCore{
				AssignRoleFunc: func(_ context.Context, _ int, _ mdl.AssignRole) error {
					return mdl.ErrValidation
				},
			},
			in:   &pb.AssignRoleRequest{RoleId: roleID, UserId: userID, Scope: &pb.AssignRoleRequest_ProjectId{ProjectId: 42}},
			want: status.New(codes.InvalidArgument, "invalid request"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), RoleCore: tt.roleCore})

			_, err := srvTest.roleServiceClient.AssignRole(authCtxForTestUser(t, t.Context()), tt.in)

			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("AssignRole() error = %v, want grpc status", err)
			}
			if st.Code() != tt.want.Code() {
				t.Errorf("AssignRole() code = %v, want %v", st.Code(), tt.want.Code())
			}
		})
	}
}

func TestRoleService_UnassignRole(t *testing.T) {
	roleID, userID := uuid.New(), uuid.New()

	roleCore := &MockedRoleCore{
		UnassignRoleFunc: func(_ context.Context, orgID int, in mdl.UnassignRole) error {
			if orgID != testOrgID {
				t.Errorf("UnassignRole() orgID = %d, want %d", orgID, testOrgID)
			}
			if in.RoleID != roleID {
				t.Errorf("UnassignRole() RoleID = %v, want %v", in.RoleID, roleID)
			}
			if in.UserID != userID {
				t.Errorf("UnassignRole() UserID = %v, want %v", in.UserID, userID)
			}
			if in.Scope.OrgID == nil || *in.Scope.OrgID != 7 {
				t.Errorf("UnassignRole() Scope.OrgID = %v, want 7", in.Scope.OrgID)
			}
			return nil
		},
	}

	srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), RoleCore: roleCore})

	got, err := srvTest.roleServiceClient.UnassignRole(authCtxForTestUser(t, t.Context()), &pb.UnassignRoleRequest{
		RoleId: roleID.String(),
		UserId: userID.String(),
		Scope:  &pb.UnassignRoleRequest_OrgId{OrgId: 7},
	})
	if err != nil {
		t.Fatalf("UnassignRole() error = %q, want no error", err)
	}

	testingx.AssertDiff(t, got, &emptypb.Empty{}, defaultDiffOpts())
}

func TestRoleService_UnassignRole_error(t *testing.T) {
	roleID, userID := uuid.New().String(), uuid.New().String()

	tests := []struct {
		name     string
		roleCore RoleCore
		in       *pb.UnassignRoleRequest
		want     *status.Status
	}{
		{
			name:     "invalid role id",
			roleCore: &MockedRoleCore{},
			in:       &pb.UnassignRoleRequest{RoleId: "not-a-uuid", UserId: userID, Scope: &pb.UnassignRoleRequest_ProjectId{ProjectId: 42}},
			want:     status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name: "not found",
			roleCore: &MockedRoleCore{
				UnassignRoleFunc: func(_ context.Context, _ int, _ mdl.UnassignRole) error {
					return mdl.ErrNotFound
				},
			},
			in:   &pb.UnassignRoleRequest{RoleId: roleID, UserId: userID, Scope: &pb.UnassignRoleRequest_ProjectId{ProjectId: 42}},
			want: status.New(codes.NotFound, "role or user not found"),
		},
		{
			name: "validation error",
			roleCore: &MockedRoleCore{
				UnassignRoleFunc: func(_ context.Context, _ int, _ mdl.UnassignRole) error {
					return mdl.ErrValidation
				},
			},
			in:   &pb.UnassignRoleRequest{RoleId: roleID, UserId: userID, Scope: &pb.UnassignRoleRequest_ProjectId{ProjectId: 42}},
			want: status.New(codes.InvalidArgument, "invalid request"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), RoleCore: tt.roleCore})

			_, err := srvTest.roleServiceClient.UnassignRole(authCtxForTestUser(t, t.Context()), tt.in)

			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("UnassignRole() error = %v, want grpc status", err)
			}
			if st.Code() != tt.want.Code() {
				t.Errorf("UnassignRole() code = %v, want %v", st.Code(), tt.want.Code())
			}
		})
	}
}

func TestRoleService_ListRoleAssignments(t *testing.T) {
	diffOpts := defaultDiffOpts()

	userID := uuid.New()
	roleID1, roleID2 := uuid.New(), uuid.New()
	projectID := 42

	roleCore := &MockedRoleCore{
		ListRoleAssignmentsFunc: func(_ context.Context, orgID int, gotUserID uuid.UUID, pageSize, pageOffset int) ([]mdl.RoleAssignment, int, error) {
			if orgID != testOrgID {
				t.Errorf("ListRoleAssignments() orgID = %d, want %d", orgID, testOrgID)
			}
			if gotUserID != userID {
				t.Errorf("ListRoleAssignments() userID = %v, want %v", gotUserID, userID)
			}
			assignments := []mdl.RoleAssignment{
				{RoleID: roleID1, RoleName: "viewer", Scope: mdl.RoleScope{ProjectID: &projectID}},
				{RoleID: roleID2, RoleName: "admin", Scope: mdl.RoleScope{OrgID: &orgID}},
			}
			end := min(pageOffset+pageSize, len(assignments))
			if pageOffset >= len(assignments) {
				return nil, len(assignments), nil
			}
			return assignments[pageOffset:end], len(assignments), nil
		},
	}

	srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), RoleCore: roleCore})

	got, err := srvTest.roleServiceClient.ListRoleAssignments(authCtxForTestUser(t, t.Context()), &pb.ListRoleAssignmentsRequest{UserId: userID.String()})
	if err != nil {
		t.Fatalf("ListRoleAssignments() error = %q, want no error", err)
	}

	want := &pb.ListRoleAssignmentsResponse{
		Assignments: []*pb.RoleAssignment{
			{RoleId: roleID1.String(), RoleName: "viewer", Scope: &pb.RoleAssignment_ProjectId{ProjectId: 42}},
			{RoleId: roleID2.String(), RoleName: "admin", Scope: &pb.RoleAssignment_OrgId{OrgId: int32(testOrgID)}},
		},
		TotalSize: 2,
	}

	testingx.AssertDiff(t, got, want, diffOpts)
}

func TestRoleService_ListRoleAssignments_error(t *testing.T) {
	tests := []struct {
		name     string
		roleCore RoleCore
		in       *pb.ListRoleAssignmentsRequest
		want     *status.Status
	}{
		{
			name:     "invalid user id",
			roleCore: &MockedRoleCore{},
			in:       &pb.ListRoleAssignmentsRequest{UserId: "not-a-uuid"},
			want:     status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name: "user not found",
			roleCore: &MockedRoleCore{
				ListRoleAssignmentsFunc: func(_ context.Context, _ int, _ uuid.UUID, _, _ int) ([]mdl.RoleAssignment, int, error) {
					return nil, 0, mdl.ErrNotFound
				},
			},
			in:   &pb.ListRoleAssignmentsRequest{UserId: uuid.New().String()},
			want: status.New(codes.NotFound, codes.NotFound.String()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), RoleCore: tt.roleCore})

			_, err := srvTest.roleServiceClient.ListRoleAssignments(authCtxForTestUser(t, t.Context()), tt.in)

			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("ListRoleAssignments() error = %v, want grpc status", err)
			}
			if st.Code() != tt.want.Code() {
				t.Errorf("ListRoleAssignments() code = %v, want %v", st.Code(), tt.want.Code())
			}
		})
	}
}

func TestRoleService_ListRolePermissions(t *testing.T) {
	diffOpts := defaultDiffOpts()

	roleID := uuid.New()

	roleCore := &MockedRoleCore{
		RolePermissionsFunc: func(_ context.Context, orgID int, gotRoleID uuid.UUID, pageSize, pageOffset int) ([]mdl.Permission, int, error) {
			if orgID != testOrgID {
				t.Errorf("RolePermissions() orgID = %d, want %d", orgID, testOrgID)
			}
			if gotRoleID != roleID {
				t.Errorf("RolePermissions() roleID = %v, want %v", gotRoleID, roleID)
			}
			permissions := []mdl.Permission{mdl.PermissionRoleRead, mdl.PermissionRoleUpdate}
			end := min(pageOffset+pageSize, len(permissions))
			if pageOffset >= len(permissions) {
				return nil, len(permissions), nil
			}
			return permissions[pageOffset:end], len(permissions), nil
		},
	}

	srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), RoleCore: roleCore})

	got, err := srvTest.roleServiceClient.ListRolePermissions(authCtxForTestUser(t, t.Context()), &pb.ListRolePermissionsRequest{RoleId: roleID.String()})
	if err != nil {
		t.Fatalf("ListRolePermissions() error = %q, want no error", err)
	}

	want := &pb.ListRolePermissionsResponse{
		Permissions: []string{"role:read", "role:update"},
		TotalSize:   2,
	}

	testingx.AssertDiff(t, got, want, diffOpts)
}

func TestRoleService_ListRolePermissions_error(t *testing.T) {
	tests := []struct {
		name     string
		roleCore RoleCore
		in       *pb.ListRolePermissionsRequest
		want     *status.Status
	}{
		{
			name:     "invalid role id",
			roleCore: &MockedRoleCore{},
			in:       &pb.ListRolePermissionsRequest{RoleId: "not-a-uuid"},
			want:     status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name: "not found",
			roleCore: &MockedRoleCore{
				RolePermissionsFunc: func(_ context.Context, _ int, _ uuid.UUID, _, _ int) ([]mdl.Permission, int, error) {
					return nil, 0, mdl.ErrNotFound
				},
			},
			in:   &pb.ListRolePermissionsRequest{RoleId: uuid.New().String()},
			want: status.New(codes.NotFound, codes.NotFound.String()),
		},
		{
			// A role_id that identifies a static role, rather than a custom one, surfaces as the
			// same mdl.ErrNotFound as any other nonexistent ID — RolePermissions only ever resolves
			// custom roles, and static roles live in a separate table it never queries. A more
			// specific "that's a static role" error would require an extra existence check against
			// the static role table once the primary lookup misses; not done here, since this path
			// only matters for a caller who already holds a static role's ID and mistakenly feeds it
			// to this RPC.
			name: "role id resolves to a static role, not a custom one",
			roleCore: &MockedRoleCore{
				RolePermissionsFunc: func(_ context.Context, _ int, _ uuid.UUID, _, _ int) ([]mdl.Permission, int, error) {
					return nil, 0, mdl.ErrNotFound
				},
			},
			in:   &pb.ListRolePermissionsRequest{RoleId: uuid.New().String()},
			want: status.New(codes.NotFound, codes.NotFound.String()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), RoleCore: tt.roleCore})

			_, err := srvTest.roleServiceClient.ListRolePermissions(authCtxForTestUser(t, t.Context()), tt.in)

			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("ListRolePermissions() error = %v, want grpc status", err)
			}
			if st.Code() != tt.want.Code() {
				t.Errorf("ListRolePermissions() code = %v, want %v", st.Code(), tt.want.Code())
			}
		})
	}
}

func TestRoleService_ListAssignablePermissions(t *testing.T) {
	srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), RoleCore: &MockedRoleCore{}})

	got, err := srvTest.roleServiceClient.ListAssignablePermissions(authCtxForTestUser(t, t.Context()), &pb.ListAssignablePermissionsRequest{})
	if err != nil {
		t.Fatalf("ListAssignablePermissions() error = %q, want no error", err)
	}

	want := &pb.ListAssignablePermissionsResponse{
		Permissions: []string{"role:read", "role:create", "role:update", "role:delete", "role:assign", "role:unassign"},
	}
	testingx.AssertDiff(t, got, want, defaultDiffOpts())
}
