package rbac

import (
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
	"github.com/zorcal/theapp/backend/pkg/x/slicesx"
)

func customRoleFromPg(r pgrbac.CustomRole) mdl.CustomRole {
	var kind mdl.RoleKind
	switch {
	case r.ManagedKey == nil:
		kind = mdl.RoleKindCustom
	case *r.ManagedKey == mdl.ManagedRoleKeyOrganizationAdmin:
		kind = mdl.RoleKindOrganizationAdmin
	default:
		// Managed keys are constrained by the database's closed managed-role registry. Reaching
		// this branch means the schema and core model no longer agree.
		panic("unsupported managed role key: " + *r.ManagedKey)
	}
	return mdl.CustomRole{
		ID:          r.ExternalID,
		Name:        r.Name,
		Kind:        kind,
		Permissions: permissionsFromPg(r.PermissionNames),
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
		ETag:        r.ETag.String(),
	}
}

func customRolesFromPg(rs []pgrbac.CustomRole) []mdl.CustomRole {
	return slicesx.Map(rs, customRoleFromPg)
}

func createCustomRoleToPg(cr mdl.CreateCustomRole, orgID int) pgrbac.CreateCustomRole {
	return pgrbac.CreateCustomRole{
		OrgID:           orgID,
		Name:            cr.Name,
		PermissionNames: permissionsToPg(cr.Permissions),
	}
}

func updateCustomRoleToPg(ur mdl.UpdateCustomRole, orgID int) pgrbac.UpdateCustomRole {
	return pgrbac.UpdateCustomRole{
		OrgID:      orgID,
		ExternalID: ur.ID,
		Fields: pgrbac.CustomRoleUpdateFields{
			Name:            ur.Fields.Name,
			PermissionNames: ur.Fields.Permissions,
		},
		Name:            ur.Name,
		PermissionNames: permissionsToPg(ur.Permissions),
	}
}

func modifyCustomRolePermissionsToPg(mrp mdl.ModifyCustomRolePermissions, orgID int) pgrbac.ModifyCustomRolePermissions {
	return pgrbac.ModifyCustomRolePermissions{
		OrgID:                 orgID,
		ExternalID:            mrp.ID,
		AddPermissionNames:    permissionsToPg(mrp.AddPermissions),
		RemovePermissionNames: permissionsToPg(mrp.RemovePermissions),
	}
}

func systemRoleFromPg(r pgrbac.SystemRole) mdl.SystemRole {
	return mdl.SystemRole{
		Name:        r.Name,
		Permissions: permissionsFromPg(r.PermissionNames),
	}
}

func systemRolesFromPg(rs []pgrbac.SystemRole) []mdl.SystemRole {
	return slicesx.Map(rs, systemRoleFromPg)
}

func permissionsFromPg(names []string) []mdl.Permission {
	return slicesx.Map(names, func(n string) mdl.Permission { return mdl.Permission(n) })
}

func permissionsToPg(permissions []mdl.Permission) []string {
	return slicesx.Map(permissions, func(permission mdl.Permission) string { return string(permission) })
}
