package pgrbac

import (
	"github.com/jackc/pgx/v5"

	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

func rolesQuery() pgdb.TypedQuery[Role] {
	const sql = `
		SELECT
			r.name,
			r.is_static,
			COALESCE(array_agg(p.name ORDER BY p.name) FILTER (WHERE p.name IS NOT NULL), '{}') AS permission_names
		FROM rbac.roles r
		LEFT JOIN rbac.role_permissions rp ON rp.role_id = r.id
		LEFT JOIN rbac.permissions p ON p.id = rp.permission_id
		GROUP BY r.id
		ORDER BY r.name`

	return pgdb.TypedQuery[Role]{
		SQL:    sql,
		Scan:   pgx.RowToStructByName[Role],
		Expect: pgdb.ExpectMany,
	}
}
