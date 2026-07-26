package conv

import (
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
		Id:          customRole.ID.String(),
		Name:        customRole.Name,
		Permissions: PermissionsToPB(customRole.Permissions),
		CreateTime:  timestamppb.New(customRole.CreatedAt),
		UpdateTime:  maybeNewTimestamppb(customRole.UpdatedAt),
		Etag:        customRole.ETag,
	}
}

func CreateCustomRoleFromPB(customRole *pb.Role) mdl.CreateCustomRole {
	return mdl.CreateCustomRole{
		Name:        customRole.GetName(),
		Permissions: permsFromPB(customRole.GetPermissions()),
	}
}

func UpdateCustomRoleFromPB(req *pb.UpdateRoleRequest, customRoleID uuid.UUID) mdl.UpdateCustomRole {
	paths := req.GetUpdateMask().GetPaths()
	return mdl.UpdateCustomRole{
		ID:          customRoleID,
		Name:        req.GetRole().GetName(),
		Permissions: permsFromPB(req.GetRole().GetPermissions()),
		Fields: mdl.CustomRoleUpdateFields{
			Name:        slices.Contains(paths, "name"),
			Permissions: slices.Contains(paths, "permissions"),
		},
	}
}

func ModifyCustomRolePermissionsFromPB(req *pb.ModifyRolePermissionsRequest, customRoleID uuid.UUID) mdl.ModifyCustomRolePermissions {
	return mdl.ModifyCustomRolePermissions{
		ID:                customRoleID,
		AddPermissions:    permsFromPB(req.GetAddPermissions()),
		RemovePermissions: permsFromPB(req.GetRemovePermissions()),
	}
}
