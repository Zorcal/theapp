package rbac

import (
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
	"github.com/zorcal/theapp/backend/pkg/x/slicesx"
)

func staticRoleFromPg(r pgrbac.RoleStatic) mdl.RoleStatic {
	return mdl.RoleStatic{
		Name:        r.Name,
		Permissions: permissionsFromPg(r.PermissionNames),
	}
}

func staticRolesFromPg(rs []pgrbac.RoleStatic) []mdl.RoleStatic {
	return slicesx.Map(rs, staticRoleFromPg)
}

func permissionsFromPg(names []string) []mdl.Permission {
	return slicesx.Map(names, func(n string) mdl.Permission { return mdl.Permission(n) })
}
