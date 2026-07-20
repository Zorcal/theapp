package rbac

import (
	"slices"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
	"github.com/zorcal/theapp/backend/pkg/x/slicesx"
)

func roleCustomFromPg(r pgrbac.RoleCustom) mdl.RoleCustom {
	return mdl.RoleCustom{
		ID:          r.ExternalID,
		Name:        r.Name,
		OrgID:       r.OrgID,
		Permissions: permissionsFromPg(r.PermissionNames),
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
		ETag:        r.ETag.String(),
	}
}

func rolesCustomFromPg(rs []pgrbac.RoleCustom) []mdl.RoleCustom {
	return slicesx.Map(rs, roleCustomFromPg)
}

func staticRoleFromPg(r pgrbac.RoleStatic) mdl.RoleStatic {
	return mdl.RoleStatic{
		ID:          r.ExternalID,
		Name:        r.Name,
		Permissions: permissionsFromPg(r.PermissionNames),
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

func staticRolesFromPg(rs []pgrbac.RoleStatic) []mdl.RoleStatic {
	return slicesx.Map(rs, staticRoleFromPg)
}

func permissionsFromPg(names []string) []mdl.Permission {
	return slicesx.Map(names, func(n string) mdl.Permission { return mdl.Permission(n) })
}

func permissionNamesFromMdl(perms []mdl.Permission) []string {
	return slicesx.Map(perms, func(p mdl.Permission) string { return string(p) })
}

func createRoleToPg(orgID int, cr mdl.CreateRole) pgrbac.CreateRole {
	return pgrbac.CreateRole{
		OrgID:           orgID,
		Name:            cr.Name,
		PermissionNames: permissionNamesFromMdl(cr.Permissions),
	}
}

func roleAssignmentFromPg(ra pgrbac.RoleAssignment) mdl.RoleAssignment {
	return mdl.RoleAssignment{
		RoleID:   ra.RoleExternalID,
		RoleName: ra.RoleName,
		Scope: mdl.RoleScope{
			ProjectID: ra.ProjectID,
			OrgID:     ra.OrgID,
		},
	}
}

func roleAssignmentsFromPg(ras []pgrbac.RoleAssignment) []mdl.RoleAssignment {
	return slicesx.Map(ras, roleAssignmentFromPg)
}

// mergePermissionNames computes (existing ∪ add) \ remove, deduplicated and sorted. Adding a
// name already in existing, or removing one that isn't, is a no-op — the set operations already
// behave that way without any special-casing.
func mergePermissionNames(existing []string, add, remove []mdl.Permission) []string {
	set := make(map[string]struct{}, len(existing)+len(add))
	for _, n := range existing {
		set[n] = struct{}{}
	}
	for _, p := range add {
		set[string(p)] = struct{}{}
	}
	for _, p := range remove {
		delete(set, string(p))
	}

	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}
