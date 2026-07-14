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

func systemPermissionsQuery(userID int) pgdb.TypedQuery[string] {
	params := pgx.NamedArgs{"user_id": userID}
	const sql = `
		SELECT DISTINCT p.name
		FROM rbac.system_role_assignments sra
		JOIN rbac.role_permissions rp ON rp.role_id = sra.role_id
		JOIN rbac.permissions p ON p.id = rp.permission_id
		WHERE sra.user_id = @user_id
		ORDER BY p.name`

	return pgdb.TypedQuery[string]{
		SQL:  sql,
		Args: params,
		Scan: func(row pgx.CollectableRow) (string, error) {
			var name string
			return name, row.Scan(&name)
		},
		Expect: pgdb.ExpectMany,
	}
}

func assignSystemRoleQuery(userID int, roleName string) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{"user_id": userID, "role_name": roleName}
	const sql = `
		INSERT INTO rbac.system_role_assignments (user_id, role_id)
		SELECT @user_id, r.id
		FROM rbac.roles r
		WHERE r.name = @role_name
		RETURNING role_id`

	return pgdb.TypedQuery[int]{
		SQL:  sql,
		Args: params,
		Scan: func(row pgx.CollectableRow) (int, error) {
			var id int
			return id, row.Scan(&id)
		},
		Expect: pgdb.ExpectOne,
	}
}
