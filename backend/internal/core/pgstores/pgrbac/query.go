package pgrbac

import (
	"github.com/jackc/pgx/v5"

	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

func systemRolesQuery(pageSize, pageOffset int) pgdb.TypedQuery[SystemRole] {
	params := pgx.NamedArgs{"page_size": pageSize, "page_offset": pageOffset}
	const sql = `
		SELECT
			r.name,
			COALESCE(array_agg(p.name ORDER BY p.name) FILTER (WHERE p.name IS NOT NULL), '{}') AS permission_names
		FROM rbac.system_roles AS r
		LEFT JOIN rbac.system_role_permissions AS rp ON rp.role_id = r.id
		LEFT JOIN rbac.permissions AS p ON p.id = rp.permission_id
		GROUP BY r.id
		ORDER BY r.name
		LIMIT @page_size OFFSET @page_offset`

	return pgdb.TypedQuery[SystemRole]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[SystemRole],
		Expect: pgdb.ExpectMany,
	}
}

func systemRoleCountQuery() pgdb.TypedQuery[int] {
	const sql = `SELECT COUNT(*) FROM rbac.system_roles`

	return pgdb.TypedQuery[int]{
		SQL: sql,
		Scan: func(row pgx.CollectableRow) (int, error) {
			var count int
			return count, row.Scan(&count)
		},
		Expect: pgdb.ExpectOne,
	}
}

func userSystemRolesQuery(userID, pageSize, pageOffset int) pgdb.TypedQuery[SystemRole] {
	params := pgx.NamedArgs{
		"user_id":     userID,
		"page_size":   pageSize,
		"page_offset": pageOffset,
	}
	const sql = `
		SELECT
			r.name,
			COALESCE(array_agg(p.name ORDER BY p.name) FILTER (WHERE p.name IS NOT NULL), '{}') AS permission_names
		FROM rbac.system_role_assignments AS sra
		JOIN rbac.system_roles AS r ON r.id = sra.role_id
		LEFT JOIN rbac.system_role_permissions AS rp ON rp.role_id = r.id
		LEFT JOIN rbac.permissions AS p ON p.id = rp.permission_id
		WHERE sra.user_id = @user_id
		GROUP BY r.id
		ORDER BY r.name
		LIMIT @page_size OFFSET @page_offset`

	return pgdb.TypedQuery[SystemRole]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[SystemRole],
		Expect: pgdb.ExpectMany,
	}
}

func userSystemRoleCountQuery(userID int) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{"user_id": userID}
	const sql = `
		SELECT COUNT(*)
		FROM rbac.system_role_assignments
		WHERE user_id = @user_id`

	return pgdb.TypedQuery[int]{
		SQL:  sql,
		Args: params,
		Scan: func(row pgx.CollectableRow) (int, error) {
			var count int
			return count, row.Scan(&count)
		},
		Expect: pgdb.ExpectOne,
	}
}

func systemPermissionsQuery(userID int) pgdb.TypedQuery[string] {
	params := pgx.NamedArgs{"user_id": userID}
	const sql = `
		SELECT DISTINCT p.name
		FROM rbac.system_role_assignments AS sra
		JOIN rbac.system_role_permissions AS rp ON rp.role_id = sra.role_id
		JOIN rbac.permissions AS p ON p.id = rp.permission_id
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
		FROM rbac.system_roles AS r
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

func unassignSystemRoleQuery(userID int, roleName string) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{"user_id": userID, "role_name": roleName}
	const sql = `
		DELETE FROM rbac.system_role_assignments AS sra
		USING rbac.system_roles AS r
		WHERE sra.user_id = @user_id
			AND sra.role_id = r.id
			AND r.name = @role_name
		RETURNING sra.role_id`

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

// projectPermissionsQuery resolves projectID's org and the distinct union of permissions userID
// holds for it, across all three assignment scopes: project_role_assignments filtered by
// projectID directly, org_role_assignments filtered by projectID's org, and
// system_role_assignments, unconditionally. Anchoring on org.projects in a single round trip
// means a nonexistent projectID yields zero rows rather than a permission set that only reflects
// system-scope grants.
func projectPermissionsQuery(userID, projectID int) pgdb.TypedQuery[ProjectPermissions] {
	params := pgx.NamedArgs{"user_id": userID, "project_id": projectID}
	const sql = `
		SELECT
			proj.org_id,
			COALESCE(array_agg(p.name ORDER BY p.name) FILTER (WHERE p.name IS NOT NULL), '{}') AS permission_names
		FROM org.projects AS proj
		LEFT JOIN LATERAL (
			SELECT rp.permission_id
			FROM rbac.project_role_assignments AS pra
			JOIN rbac.custom_role_permissions AS rp ON rp.role_id = pra.role_id
			WHERE pra.user_id = @user_id AND pra.project_id = proj.id

			UNION

			SELECT rp.permission_id
			FROM rbac.org_role_assignments AS ora
			JOIN rbac.custom_role_permissions AS rp ON rp.role_id = ora.role_id
			WHERE ora.user_id = @user_id AND ora.org_id = proj.org_id

			UNION

			SELECT rp.permission_id
			FROM rbac.system_role_assignments AS sra
			JOIN rbac.system_role_permissions AS rp ON rp.role_id = sra.role_id
			WHERE sra.user_id = @user_id
		) AS granted ON true
		LEFT JOIN rbac.permissions AS p ON p.id = granted.permission_id
		WHERE proj.id = @project_id
		GROUP BY proj.org_id`

	return pgdb.TypedQuery[ProjectPermissions]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[ProjectPermissions],
		Expect: pgdb.ExpectOne,
	}
}
