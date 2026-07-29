package conv

import (
	"fmt"
	"slices"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/pkg/x/slicesx"
)

func CustomRolesToPB(customRoles []mdl.CustomRole) []*pb.Role {
	return slicesx.Map(customRoles, CustomRoleToPB)
}

func CustomRoleToPB(customRole mdl.CustomRole) *pb.Role {
	return &pb.Role{
		Id:                     customRole.ID.String(),
		Name:                   customRole.Name,
		Permissions:            PermissionsToPB(customRole.Permissions),
		Kind:                   roleKindToPB(customRole.Kind),
		MinimumAssignmentScope: AssignmentScopeToPB(customRole.MinimumAssignmentScope()),
		CreateTime:             timestamppb.New(customRole.CreatedAt),
		UpdateTime:             maybeNewTimestamppb(customRole.UpdatedAt),
		Etag:                   customRole.ETag,
	}
}

func roleKindToPB(kind mdl.RoleKind) pb.RoleKind {
	switch kind {
	case mdl.RoleKindCustom:
		return pb.RoleKind_ROLE_KIND_CUSTOM
	case mdl.RoleKindOrganizationAdmin:
		return pb.RoleKind_ROLE_KIND_ORGANIZATION_ADMIN
	default:
		// Role kinds come from the backend's closed managed-role registry. Reaching this branch
		// means a model value was added without updating the API conversion.
		panic(fmt.Sprintf("unsupported role kind: %d", kind))
	}
}

func CreateCustomRoleFromPB(customRole *pb.Role) (mdl.CreateCustomRole, bool) {
	permissions, ok := PermissionsFromPB(customRole.GetPermissions())
	if !ok {
		return mdl.CreateCustomRole{}, false
	}
	return mdl.CreateCustomRole{
		Name:        customRole.GetName(),
		Permissions: permissions,
	}, true
}

func UpdateCustomRoleFromPB(req *pb.UpdateRoleRequest, customRoleID uuid.UUID) (mdl.UpdateCustomRole, bool) {
	permissions, ok := PermissionsFromPB(req.GetRole().GetPermissions())
	if !ok {
		return mdl.UpdateCustomRole{}, false
	}
	paths := req.GetUpdateMask().GetPaths()
	return mdl.UpdateCustomRole{
		ID:          customRoleID,
		Name:        req.GetRole().GetName(),
		Permissions: permissions,
		Fields: mdl.CustomRoleUpdateFields{
			Name:        slices.Contains(paths, "name"),
			Permissions: slices.Contains(paths, "permissions"),
		},
	}, true
}

func ModifyCustomRolePermissionsFromPB(req *pb.ModifyRolePermissionsRequest, customRoleID uuid.UUID) (mdl.ModifyCustomRolePermissions, bool) {
	addPermissions, addOK := PermissionsFromPB(req.GetAddPermissions())
	removePermissions, removeOK := PermissionsFromPB(req.GetRemovePermissions())
	if !addOK || !removeOK {
		return mdl.ModifyCustomRolePermissions{}, false
	}
	return mdl.ModifyCustomRolePermissions{
		ID:                customRoleID,
		AddPermissions:    addPermissions,
		RemovePermissions: removePermissions,
	}, true
}
