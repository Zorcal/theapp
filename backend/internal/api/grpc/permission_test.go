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
		Permissions: []*pb.PermissionDescriptor{
			{Permission: pb.Permission_PERMISSION_CUSTOM_ROLE_CREATE, MinimumAssignmentScope: pb.AssignmentScope_ASSIGNMENT_SCOPE_ORGANIZATION},
			{Permission: pb.Permission_PERMISSION_CUSTOM_ROLE_READ, MinimumAssignmentScope: pb.AssignmentScope_ASSIGNMENT_SCOPE_ORGANIZATION},
			{Permission: pb.Permission_PERMISSION_CUSTOM_ROLE_UPDATE, MinimumAssignmentScope: pb.AssignmentScope_ASSIGNMENT_SCOPE_ORGANIZATION},
			{Permission: pb.Permission_PERMISSION_CUSTOM_ROLE_DELETE, MinimumAssignmentScope: pb.AssignmentScope_ASSIGNMENT_SCOPE_ORGANIZATION},
			{Permission: pb.Permission_PERMISSION_CUSTOM_ROLE_ASSIGN_PROJECT, MinimumAssignmentScope: pb.AssignmentScope_ASSIGNMENT_SCOPE_PROJECT},
			{Permission: pb.Permission_PERMISSION_CUSTOM_ROLE_UNASSIGN_PROJECT, MinimumAssignmentScope: pb.AssignmentScope_ASSIGNMENT_SCOPE_PROJECT},
			{Permission: pb.Permission_PERMISSION_CUSTOM_ROLE_ASSIGN_ORGANIZATION, MinimumAssignmentScope: pb.AssignmentScope_ASSIGNMENT_SCOPE_ORGANIZATION},
			{Permission: pb.Permission_PERMISSION_CUSTOM_ROLE_UNASSIGN_ORGANIZATION, MinimumAssignmentScope: pb.AssignmentScope_ASSIGNMENT_SCOPE_ORGANIZATION},
			{Permission: pb.Permission_PERMISSION_CUSTOM_ROLE_READ_PROJECT_ASSIGNMENTS, MinimumAssignmentScope: pb.AssignmentScope_ASSIGNMENT_SCOPE_PROJECT},
			{Permission: pb.Permission_PERMISSION_CUSTOM_ROLE_READ_ORGANIZATION_ASSIGNMENTS, MinimumAssignmentScope: pb.AssignmentScope_ASSIGNMENT_SCOPE_ORGANIZATION},
			{Permission: pb.Permission_PERMISSION_ORGANIZATION_CREATE, MinimumAssignmentScope: pb.AssignmentScope_ASSIGNMENT_SCOPE_PROJECT},
			{Permission: pb.Permission_PERMISSION_PROJECT_CREATE, MinimumAssignmentScope: pb.AssignmentScope_ASSIGNMENT_SCOPE_ORGANIZATION},
			{Permission: pb.Permission_PERMISSION_ORGANIZATION_USER_CREATE, MinimumAssignmentScope: pb.AssignmentScope_ASSIGNMENT_SCOPE_ORGANIZATION},
			{Permission: pb.Permission_PERMISSION_ORGANIZATION_USER_READ, MinimumAssignmentScope: pb.AssignmentScope_ASSIGNMENT_SCOPE_ORGANIZATION},
		},
	}

	testingx.AssertDiff(t, got, want, defaultDiffOpts())
}
