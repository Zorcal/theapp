package conv

import (
	"fmt"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/pkg/x/slicesx"
)

var permissionToPB = map[mdl.Permission]pb.Permission{
	mdl.PermissionUserRead:                         pb.Permission_PERMISSION_USER_READ,
	mdl.PermissionUserCreate:                       pb.Permission_PERMISSION_USER_CREATE,
	mdl.PermissionUserUpdate:                       pb.Permission_PERMISSION_USER_UPDATE,
	mdl.PermissionUserDelete:                       pb.Permission_PERMISSION_USER_DELETE,
	mdl.PermissionUserRestore:                      pb.Permission_PERMISSION_USER_RESTORE,
	mdl.PermissionSystemRoleRead:                   pb.Permission_PERMISSION_SYSTEM_ROLE_READ,
	mdl.PermissionSystemRoleAssign:                 pb.Permission_PERMISSION_SYSTEM_ROLE_ASSIGN,
	mdl.PermissionSystemRoleUnassign:               pb.Permission_PERMISSION_SYSTEM_ROLE_UNASSIGN,
	mdl.PermissionProjectDiscoverAll:               pb.Permission_PERMISSION_PROJECT_DISCOVER_ALL,
	mdl.PermissionOrgCreate:                        pb.Permission_PERMISSION_ORGANIZATION_CREATE,
	mdl.PermissionProjectCreate:                    pb.Permission_PERMISSION_PROJECT_CREATE,
	mdl.PermissionOrgUserCreate:                    pb.Permission_PERMISSION_ORGANIZATION_USER_CREATE,
	mdl.PermissionOrgUserRead:                      pb.Permission_PERMISSION_ORGANIZATION_USER_READ,
	mdl.PermissionOrgUserRemove:                    pb.Permission_PERMISSION_ORGANIZATION_USER_REMOVE,
	mdl.PermissionCustomRoleCreate:                 pb.Permission_PERMISSION_CUSTOM_ROLE_CREATE,
	mdl.PermissionCustomRoleRead:                   pb.Permission_PERMISSION_CUSTOM_ROLE_READ,
	mdl.PermissionCustomRoleUpdate:                 pb.Permission_PERMISSION_CUSTOM_ROLE_UPDATE,
	mdl.PermissionCustomRoleDelete:                 pb.Permission_PERMISSION_CUSTOM_ROLE_DELETE,
	mdl.PermissionCustomRoleAssignProject:          pb.Permission_PERMISSION_CUSTOM_ROLE_ASSIGN_PROJECT,
	mdl.PermissionCustomRoleUnassignProject:        pb.Permission_PERMISSION_CUSTOM_ROLE_UNASSIGN_PROJECT,
	mdl.PermissionCustomRoleAssignOrg:              pb.Permission_PERMISSION_CUSTOM_ROLE_ASSIGN_ORGANIZATION,
	mdl.PermissionCustomRoleUnassignOrg:            pb.Permission_PERMISSION_CUSTOM_ROLE_UNASSIGN_ORGANIZATION,
	mdl.PermissionCustomRoleReadProjectAssignments: pb.Permission_PERMISSION_CUSTOM_ROLE_READ_PROJECT_ASSIGNMENTS,
	mdl.PermissionCustomRoleReadOrgAssignments:     pb.Permission_PERMISSION_CUSTOM_ROLE_READ_ORGANIZATION_ASSIGNMENTS,
}

var permissionFromPB = func() map[pb.Permission]mdl.Permission {
	result := make(map[pb.Permission]mdl.Permission, len(permissionToPB))
	for mdlPerm, pbPerm := range permissionToPB {
		if _, ok := result[pbPerm]; ok {
			panic("duplicate protobuf permission mapping: " + pbPerm.String())
		}
		result[pbPerm] = mdlPerm
	}
	return result
}()

func PermissionToPB(permission mdl.Permission) (pb.Permission, bool) {
	result, ok := permissionToPB[permission]
	return result, ok
}

func PermissionFromPB(permission pb.Permission) (mdl.Permission, bool) {
	result, ok := permissionFromPB[permission]
	return result, ok
}

func PermissionsToPB(permissions []mdl.Permission) []pb.Permission {
	return slicesx.Map(permissions, mustPermissionToPB)
}

func PermissionsFromPB(permissions []pb.Permission) ([]mdl.Permission, bool) {
	result := make([]mdl.Permission, 0, len(permissions))
	for _, permission := range permissions {
		modelPermission, ok := PermissionFromPB(permission)
		if !ok {
			return nil, false
		}
		result = append(result, modelPermission)
	}
	return result, true
}

func AssignmentScopeToPB(scope mdl.AssignmentScope) pb.AssignmentScope {
	switch scope {
	case mdl.AssignmentScopeProject:
		return pb.AssignmentScope_ASSIGNMENT_SCOPE_PROJECT
	case mdl.AssignmentScopeOrganization:
		return pb.AssignmentScope_ASSIGNMENT_SCOPE_ORGANIZATION
	case mdl.AssignmentScopeSystem:
		return pb.AssignmentScope_ASSIGNMENT_SCOPE_SYSTEM
	default:
		// Assignment scopes are derived from the backend's closed permission registry. Reaching
		// this branch means a model value was added without updating the API conversion.
		panic(fmt.Sprintf("unsupported assignment scope: %d", scope))
	}
}

func PermissionDescriptorsToPB(descriptors []mdl.PermissionDescriptor) []*pb.PermissionDescriptor {
	return slicesx.Map(descriptors, func(desc mdl.PermissionDescriptor) *pb.PermissionDescriptor {
		return &pb.PermissionDescriptor{
			Permission:             mustPermissionToPB(desc.Permission),
			MinimumAssignmentScope: AssignmentScopeToPB(desc.MinimumAssignmentScope),
		}
	})
}

func mustPermissionToPB(permission mdl.Permission) pb.Permission {
	result, ok := PermissionToPB(permission)
	if !ok {
		// Model permissions come from the backend's closed permission registry. Reaching this
		// branch means a model value was added without updating the API conversion.
		panic(fmt.Sprintf("unsupported permission: %q", permission))
	}
	return result
}
