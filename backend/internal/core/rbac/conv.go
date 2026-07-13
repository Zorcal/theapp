package rbac

import (
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
	"github.com/zorcal/theapp/backend/pkg/x/slicesx"
)

func roleFromPg(r pgrbac.Role) mdl.Role {
	return mdl.Role{
		Name:        r.Name,
		IsStatic:    r.IsStatic,
		Permissions: permissionsFromPg(r.PermissionNames),
	}
}

func rolesFromPg(rs []pgrbac.Role) []mdl.Role {
	return slicesx.Map(rs, roleFromPg)
}

func permissionsFromPg(names []string) []mdl.Permission {
	return slicesx.Map(names, func(n string) mdl.Permission { return mdl.Permission(n) })
}
