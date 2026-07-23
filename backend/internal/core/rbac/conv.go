package rbac

import (
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
	"github.com/zorcal/theapp/backend/pkg/x/slicesx"
)

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
