package conv

import (
	"errors"
	"fmt"
	"slices"
	"uuid"

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
		Etag:                   customRole.ETag.String(),
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

func UpdateCustomRoleFromPB(req *pb.UpdateRoleRequest, customRoleID uuid.UUID) (mdl.UpdateCustomRole, error) {
	permissions, ok := PermissionsFromPB(req.GetRole().GetPermissions())
	if !ok {
		return mdl.UpdateCustomRole{}, errors.New("unknown permission")
	}
	etag, err := uuid.Parse(req.GetRole().GetEtag())
	if err != nil {
		return mdl.UpdateCustomRole{}, fmt.Errorf("parse etag: %w", err)
	}
	paths := req.GetUpdateMask().GetPaths()
	return mdl.UpdateCustomRole{
		ID:          customRoleID,
		ETag:        etag,
		Name:        req.GetRole().GetName(),
		Permissions: permissions,
		Fields: mdl.CustomRoleUpdateFields{
			Name:        slices.Contains(paths, "name"),
			Permissions: slices.Contains(paths, "permissions"),
		},
	}, nil
}

func ModifyCustomRolePermissionsFromPB(req *pb.ModifyRolePermissionsRequest, customRoleID uuid.UUID) (mdl.ModifyCustomRolePermissions, error) {
	addPermissions, addOK := PermissionsFromPB(req.GetAddPermissions())
	removePermissions, removeOK := PermissionsFromPB(req.GetRemovePermissions())
	if !addOK || !removeOK {
		return mdl.ModifyCustomRolePermissions{}, errors.New("unknown permission")
	}
	etag, err := uuid.Parse(req.GetEtag())
	if err != nil {
		return mdl.ModifyCustomRolePermissions{}, fmt.Errorf("parse etag: %w", err)
	}
	return mdl.ModifyCustomRolePermissions{
		ID:                customRoleID,
		ETag:              etag,
		AddPermissions:    addPermissions,
		RemovePermissions: removePermissions,
	}, nil
}
