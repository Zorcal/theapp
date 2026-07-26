package conv

import (
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/pkg/x/slicesx"
)

var permissionToPB = map[mdl.Permission]pb.Permission{
	mdl.PermissionUserRead:                         pb.Permission_PERMISSION_USER_READ,
	mdl.PermissionUserCreate:                       pb.Permission_PERMISSION_USER_CREATE,
	mdl.PermissionUserUpdate:                       pb.Permission_PERMISSION_USER_UPDATE,
	mdl.PermissionSystemRoleRead:                   pb.Permission_PERMISSION_SYSTEM_ROLE_READ,
	mdl.PermissionSystemRoleAssign:                 pb.Permission_PERMISSION_SYSTEM_ROLE_ASSIGN,
	mdl.PermissionSystemRoleUnassign:               pb.Permission_PERMISSION_SYSTEM_ROLE_UNASSIGN,
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
	return slicesx.Map(permissions, func(permission mdl.Permission) pb.Permission {
		result, _ := PermissionToPB(permission)
		return result
	})
}

func PermissionsFromPB(permissions []pb.Permission) []mdl.Permission {
	return slicesx.Map(permissions, func(permission pb.Permission) mdl.Permission {
		result, _ := PermissionFromPB(permission)
		return result
	})
}

func permsFromPB(perms []pb.Permission) []mdl.Permission {
	return PermissionsFromPB(perms)
}
