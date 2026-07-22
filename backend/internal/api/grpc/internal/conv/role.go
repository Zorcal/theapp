package conv

import (
	"slices"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/pkg/mustconv"
	"github.com/zorcal/theapp/backend/pkg/x/slicesx"
)

// RoleToPB converts r to its protobuf representation. Permissions is never populated — it's
// input-only on pb.Role, accepted by CreateRole/UpdateRole but never returned; see
// ListRolePermissions for reading a role's current permission set.
func RoleToPB(r mdl.RoleCustom) *pb.Role {
	return &pb.Role{
		Id:         r.ID.String(),
		Name:       r.Name,
		CreateTime: timestamppb.New(r.CreatedAt),
		UpdateTime: maybeNewTimestamppb(r.UpdatedAt),
		Etag:       r.ETag,
	}
}

func RolesToPB(rs []mdl.RoleCustom) []*pb.Role {
	return slicesx.Map(rs, RoleToPB)
}

func CreateRoleFromPB(r *pb.Role) mdl.CreateRole {
	return mdl.CreateRole{
		Name:        r.GetName(),
		Permissions: permissionsFromPB(r.GetPermissions()),
	}
}

func UpdateRoleFromPB(req *pb.UpdateRoleRequest, id uuid.UUID) mdl.UpdateRole {
	paths := req.GetUpdateMask().GetPaths()
	return mdl.UpdateRole{
		ID:          id,
		Name:        req.GetRole().GetName(),
		Permissions: permissionsFromPB(req.GetRole().GetPermissions()),
		Fields: mdl.RoleUpdateFields{
			Name:        slices.Contains(paths, "name"),
			Permissions: slices.Contains(paths, "permissions"),
		},
	}
}

func ModifyRolePermissionsFromPB(req *pb.ModifyRolePermissionsRequest, id uuid.UUID) mdl.ModifyRolePermissions {
	return mdl.ModifyRolePermissions{
		ID:                id,
		AddPermissions:    permissionsFromPB(req.GetAddPermissions()),
		RemovePermissions: permissionsFromPB(req.GetRemovePermissions()),
	}
}

func AssignRoleFromPB(req *pb.AssignRoleRequest, roleID, userID uuid.UUID) mdl.AssignRole {
	return mdl.AssignRole{
		RoleID: roleID,
		UserID: userID,
		Scope:  assignRoleScopeFromPB(req),
	}
}

func UnassignRoleFromPB(req *pb.UnassignRoleRequest, roleID, userID uuid.UUID) mdl.UnassignRole {
	return mdl.UnassignRole{
		RoleID: roleID,
		UserID: userID,
		Scope:  unassignRoleScopeFromPB(req),
	}
}

func assignRoleScopeFromPB(req *pb.AssignRoleRequest) mdl.RoleScope {
	switch v := req.GetScope().(type) {
	case *pb.AssignRoleRequest_ProjectId:
		id := int(v.ProjectId)
		return mdl.RoleScope{ProjectID: &id}
	case *pb.AssignRoleRequest_OrgId:
		id := int(v.OrgId)
		return mdl.RoleScope{OrgID: &id}
	default:
		return mdl.RoleScope{}
	}
}

func unassignRoleScopeFromPB(req *pb.UnassignRoleRequest) mdl.RoleScope {
	switch v := req.GetScope().(type) {
	case *pb.UnassignRoleRequest_ProjectId:
		id := int(v.ProjectId)
		return mdl.RoleScope{ProjectID: &id}
	case *pb.UnassignRoleRequest_OrgId:
		id := int(v.OrgId)
		return mdl.RoleScope{OrgID: &id}
	default:
		return mdl.RoleScope{}
	}
}

func RoleAssignmentToPB(ra mdl.RoleAssignment) *pb.RoleAssignment {
	out := &pb.RoleAssignment{
		RoleId:   ra.RoleID.String(),
		RoleName: ra.RoleName,
	}
	switch {
	case ra.Scope.ProjectID != nil:
		out.Scope = &pb.RoleAssignment_ProjectId{ProjectId: mustconv.Int32(*ra.Scope.ProjectID)}
	case ra.Scope.OrgID != nil:
		out.Scope = &pb.RoleAssignment_OrgId{OrgId: mustconv.Int32(*ra.Scope.OrgID)}
	}
	return out
}

func RoleAssignmentsToPB(ras []mdl.RoleAssignment) []*pb.RoleAssignment {
	return slicesx.Map(ras, RoleAssignmentToPB)
}

func PermissionsToPB(perms []mdl.Permission) []string {
	return slicesx.Map(perms, func(p mdl.Permission) string { return string(p) })
}

func permissionsFromPB(names []string) []mdl.Permission {
	return slicesx.Map(names, func(n string) mdl.Permission { return mdl.Permission(n) })
}
