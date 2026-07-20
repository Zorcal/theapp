package grpc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestSystemRoleService_ListSystemRolePermissions(t *testing.T) {
	diffOpts := defaultDiffOpts()

	systemRoleCore := &MockedSystemRoleCore{
		StaticRolePermissionsFunc: func(_ context.Context, roleName string, pageSize, pageOffset int) ([]mdl.Permission, int, error) {
			if roleName != "superadmin" {
				t.Errorf("StaticRolePermissions() roleName = %q, want %q", roleName, "superadmin")
			}
			permissions := []mdl.Permission{mdl.PermissionRoleRead, mdl.PermissionRoleUpdate}
			end := min(pageOffset+pageSize, len(permissions))
			if pageOffset >= len(permissions) {
				return nil, len(permissions), nil
			}
			return permissions[pageOffset:end], len(permissions), nil
		},
	}

	srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), SystemRoleCore: systemRoleCore})

	got, err := srvTest.systemRoleServiceClient.ListSystemRolePermissions(authCtxForTestUser(t, t.Context()), &pb.ListSystemRolePermissionsRequest{RoleName: "superadmin"})
	if err != nil {
		t.Fatalf("ListSystemRolePermissions() error = %q, want no error", err)
	}

	want := &pb.ListSystemRolePermissionsResponse{
		Permissions: []string{"role:read", "role:update"},
		TotalSize:   2,
	}

	testingx.AssertDiff(t, got, want, diffOpts)
}

func TestSystemRoleService_ListSystemRolePermissions_error(t *testing.T) {
	tests := []struct {
		name           string
		systemRoleCore SystemRoleCore
		in             *pb.ListSystemRolePermissionsRequest
		want           *status.Status
	}{
		{
			name:           "missing role name",
			systemRoleCore: &MockedSystemRoleCore{},
			in:             &pb.ListSystemRolePermissionsRequest{},
			want:           status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name: "not found",
			systemRoleCore: &MockedSystemRoleCore{
				StaticRolePermissionsFunc: func(_ context.Context, _ string, _, _ int) ([]mdl.Permission, int, error) {
					return nil, 0, mdl.ErrNotFound
				},
			},
			in:   &pb.ListSystemRolePermissionsRequest{RoleName: "nonexistent"},
			want: status.New(codes.NotFound, codes.NotFound.String()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), SystemRoleCore: tt.systemRoleCore})

			_, err := srvTest.systemRoleServiceClient.ListSystemRolePermissions(authCtxForTestUser(t, t.Context()), tt.in)

			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("ListSystemRolePermissions() error = %v, want grpc status", err)
			}
			if st.Code() != tt.want.Code() {
				t.Errorf("ListSystemRolePermissions() code = %v, want %v", st.Code(), tt.want.Code())
			}
		})
	}
}

func TestSystemRoleService_ListSystemRoleAssignments(t *testing.T) {
	diffOpts := defaultDiffOpts()

	userID := uuid.New()

	systemRoleCore := &MockedSystemRoleCore{
		SystemRoleAssignmentsFunc: func(_ context.Context, gotUserID uuid.UUID, pageSize, pageOffset int) ([]string, int, error) {
			if gotUserID != userID {
				t.Errorf("SystemRoleAssignments() userID = %v, want %v", gotUserID, userID)
			}
			roleNames := []string{"superadmin", "support"}
			end := min(pageOffset+pageSize, len(roleNames))
			if pageOffset >= len(roleNames) {
				return nil, len(roleNames), nil
			}
			return roleNames[pageOffset:end], len(roleNames), nil
		},
	}

	srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), SystemRoleCore: systemRoleCore})

	got, err := srvTest.systemRoleServiceClient.ListSystemRoleAssignments(authCtxForTestUser(t, t.Context()), &pb.ListSystemRoleAssignmentsRequest{UserId: userID.String()})
	if err != nil {
		t.Fatalf("ListSystemRoleAssignments() error = %q, want no error", err)
	}

	want := &pb.ListSystemRoleAssignmentsResponse{
		RoleNames: []string{"superadmin", "support"},
		TotalSize: 2,
	}

	testingx.AssertDiff(t, got, want, diffOpts)
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
			want:           status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name: "user not found",
			systemRoleCore: &MockedSystemRoleCore{
				SystemRoleAssignmentsFunc: func(_ context.Context, _ uuid.UUID, _, _ int) ([]string, int, error) {
					return nil, 0, mdl.ErrNotFound
				},
			},
			in:   &pb.ListSystemRoleAssignmentsRequest{UserId: uuid.New().String()},
			want: status.New(codes.NotFound, codes.NotFound.String()),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), SystemRoleCore: tt.systemRoleCore})

			_, err := srvTest.systemRoleServiceClient.ListSystemRoleAssignments(authCtxForTestUser(t, t.Context()), tt.in)

			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("ListSystemRoleAssignments() error = %v, want grpc status", err)
			}
			if st.Code() != tt.want.Code() {
				t.Errorf("ListSystemRoleAssignments() code = %v, want %v", st.Code(), tt.want.Code())
			}
		})
	}
}
