package grpc

import (
	"testing"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestPermissionService_ListPermissions(t *testing.T) {
	srvTest := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t)})

	got, err := srvTest.permissionServiceClient.ListPermissions(
		authCtxForTestUser(t, t.Context()),
		&pb.ListPermissionsRequest{},
	)
	if err != nil {
		t.Fatalf("ListPermissions() error = %v", err)
	}

	want := &pb.ListPermissionsResponse{
		Permissions: []pb.Permission{
			pb.Permission_PERMISSION_CUSTOM_ROLE_CREATE,
			pb.Permission_PERMISSION_CUSTOM_ROLE_READ,
			pb.Permission_PERMISSION_CUSTOM_ROLE_UPDATE,
			pb.Permission_PERMISSION_CUSTOM_ROLE_DELETE,
			pb.Permission_PERMISSION_CUSTOM_ROLE_ASSIGN_PROJECT,
			pb.Permission_PERMISSION_CUSTOM_ROLE_UNASSIGN_PROJECT,
			pb.Permission_PERMISSION_CUSTOM_ROLE_ASSIGN_ORGANIZATION,
			pb.Permission_PERMISSION_CUSTOM_ROLE_UNASSIGN_ORGANIZATION,
		},
	}

	testingx.AssertDiff(t, got, want, defaultDiffOpts())
}
