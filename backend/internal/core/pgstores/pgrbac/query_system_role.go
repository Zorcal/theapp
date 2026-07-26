package pgrbac

import (
	"github.com/google/uuid"
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

func systemRoleByNameQuery(name string) pgdb.TypedQuery[SystemRole] {
	params := pgx.NamedArgs{"name": name}
	const sql = `
		SELECT
			r.name,
			COALESCE(array_agg(p.name ORDER BY p.name) FILTER (WHERE p.name IS NOT NULL), '{}') AS permission_names
		FROM rbac.system_roles AS r
		LEFT JOIN rbac.system_role_permissions AS rp ON rp.role_id = r.id
		LEFT JOIN rbac.permissions AS p ON p.id = rp.permission_id
		WHERE r.name = @name
		GROUP BY r.id`

	return pgdb.TypedQuery[SystemRole]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[SystemRole],
		Expect: pgdb.ExpectOne,
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

func userSystemRolesByExternalIDQuery(userID uuid.UUID, pageSize, pageOffset int) pgdb.TypedQuery[SystemRole] {
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
		JOIN useraccess.users AS u ON u.id = sra.user_id
		JOIN rbac.system_roles AS r ON r.id = sra.role_id
		LEFT JOIN rbac.system_role_permissions AS rp ON rp.role_id = r.id
		LEFT JOIN rbac.permissions AS p ON p.id = rp.permission_id
		WHERE u.external_id = @user_id
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

func userSystemRoleCountByExternalIDQuery(userID uuid.UUID) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{"user_id": userID}
	// Anchor on the user so no assignments returns zero while an unknown user returns no row.
	const sql = `
		SELECT COUNT(sra.role_id)
		FROM useraccess.users AS u
		LEFT JOIN rbac.system_role_assignments AS sra ON sra.user_id = u.id
		WHERE u.external_id = @user_id
		GROUP BY u.id`

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

func userSystemPermissionsByExternalIDQuery(userID uuid.UUID) pgdb.TypedQuery[[]string] {
	params := pgx.NamedArgs{"user_id": userID}
	const sql = `
		SELECT COALESCE(array_agg(DISTINCT p.name ORDER BY p.name) FILTER (WHERE p.name IS NOT NULL), '{}')
		FROM useraccess.users AS u
		LEFT JOIN rbac.system_role_assignments AS sra ON sra.user_id = u.id
		LEFT JOIN rbac.system_role_permissions AS rp ON rp.role_id = sra.role_id
		LEFT JOIN rbac.permissions AS p ON p.id = rp.permission_id
		WHERE u.external_id = @user_id
		GROUP BY u.id`

	return pgdb.TypedQuery[[]string]{
		SQL:  sql,
		Args: params,
		Scan: func(row pgx.CollectableRow) ([]string, error) {
			var names []string
			return names, row.Scan(&names)
		},
		Expect: pgdb.ExpectOne,
	}
}

func systemPermissionsRemainAfterUnassignQuery(userID uuid.UUID, roleName string, permNames []string) pgdb.TypedQuery[bool] {
	params := pgx.NamedArgs{
		"user_id":          userID,
		"role_name":        roleName,
		"permission_names": permNames,
	}
	const sql = `
		WITH
			-- Identify the exact assignment being considered for removal. Anchoring the final
			-- result on this row makes a missing user, role, or assignment return sql.ErrNoRows.
			excluded_assignment AS (
			SELECT sra.user_id, sra.role_id
			FROM rbac.system_role_assignments AS sra
			JOIN useraccess.users AS u ON u.id = sra.user_id
			JOIN rbac.system_roles AS r ON r.id = sra.role_id
			WHERE u.external_id = @user_id
				AND r.name = @role_name
		)
		SELECT NOT EXISTS (
			SELECT 1
			FROM unnest(@permission_names::text[]) AS required(name)
			WHERE NOT EXISTS (
				SELECT 1
				FROM rbac.system_role_assignments AS sra
				JOIN rbac.system_role_permissions AS rp ON rp.role_id = sra.role_id
				JOIN rbac.permissions AS p ON p.id = rp.permission_id
				WHERE p.name = required.name
					AND (sra.user_id, sra.role_id) != (
						excluded_assignment.user_id,
						excluded_assignment.role_id
					)
			)
		)
		FROM excluded_assignment`

	return pgdb.TypedQuery[bool]{
		SQL:  sql,
		Args: params,
		Scan: func(row pgx.CollectableRow) (bool, error) {
			var remain bool
			return remain, row.Scan(&remain)
		},
		Expect: pgdb.ExpectOne,
	}
}

func assignSystemRoleQuery(userID uuid.UUID, roleName string) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{"user_id": userID, "role_name": roleName}
	const sql = `
		INSERT INTO rbac.system_role_assignments (user_id, role_id)
		SELECT u.id, r.id
		FROM (
			SELECT id
			FROM useraccess.users
			WHERE external_id = @user_id
		) AS u
		CROSS JOIN (
			SELECT id
			FROM rbac.system_roles
			WHERE name = @role_name
		) AS r
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

func unassignSystemRoleQuery(userID uuid.UUID, roleName string) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{"user_id": userID, "role_name": roleName}
	const sql = `
		DELETE FROM rbac.system_role_assignments
		WHERE user_id = (
			SELECT id
			FROM useraccess.users
			WHERE external_id = @user_id
		)
			AND role_id = (
				SELECT id
				FROM rbac.system_roles
				WHERE name = @role_name
			)
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
