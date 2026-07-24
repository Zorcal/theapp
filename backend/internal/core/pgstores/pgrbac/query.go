package pgrbac

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

func createCustomRoleQuery(cr CreateCustomRole) pgdb.TypedQuery[CustomRole] {
	params := pgx.NamedArgs{
		"org_id":           cr.OrgID,
		"name":             cr.Name,
		"permission_names": cr.PermissionNames,
	}

	// Execution returns sql.ErrNoRows when the organization or any requested permission does not
	// exist, and pgdb.ErrAlreadyExists when the organization already has the role name.
	const sql = `
		WITH
			-- Convert the input array into a deduplicated set of permission names.
			requested_permissions AS (
				SELECT DISTINCT name
				FROM unnest(@permission_names::text[]) AS requested(name)
			),
			-- Resolve requested names to the permission IDs used by the join table.
			valid_permissions AS (
				SELECT p.id, p.name
				FROM rbac.permissions AS p
				JOIN requested_permissions AS requested ON requested.name = p.name
			),
			-- Insert only when the organization and every requested permission exist.
			new_role AS (
				INSERT INTO rbac.custom_roles (external_id, org_id, name, created_at, etag)
				SELECT gen_random_uuid(), o.id, @name, NOW(), gen_random_uuid()
				FROM org.organizations AS o
				WHERE o.id = @org_id
					AND NOT EXISTS (
						SELECT 1
						FROM requested_permissions AS requested
						LEFT JOIN valid_permissions AS valid ON valid.name = requested.name
						WHERE valid.id IS NULL
					)
				RETURNING id, external_id, org_id, name, created_at, updated_at, etag
			),
			-- Grant every validated permission to the new role.
			inserted_permissions AS (
				INSERT INTO rbac.custom_role_permissions (role_id, permission_id)
				SELECT new_role.id, valid_permissions.id
				FROM new_role
				CROSS JOIN valid_permissions
				RETURNING permission_id
			)
		-- Return the role with the permission rows that were actually inserted.
		SELECT
			new_role.id,
			new_role.external_id,
			new_role.name,
			COALESCE(
				(
					SELECT array_agg(valid_permissions.name ORDER BY valid_permissions.name)
					FROM inserted_permissions
					JOIN valid_permissions
						ON valid_permissions.id = inserted_permissions.permission_id
				),
				'{}'
			) AS permission_names,
			new_role.created_at,
			new_role.updated_at,
			new_role.etag
		FROM new_role`

	return pgdb.TypedQuery[CustomRole]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[CustomRole],
		Expect: pgdb.ExpectOne,
	}
}

func customRolesQuery(orgID, pageSize, pageOffset int) pgdb.TypedQuery[CustomRole] {
	params := pgx.NamedArgs{
		"org_id":      orgID,
		"page_size":   pageSize,
		"page_offset": pageOffset,
	}
	const sql = `
		SELECT
			r.id,
			r.external_id,
			r.name,
			COALESCE(array_agg(p.name ORDER BY p.name) FILTER (WHERE p.name IS NOT NULL), '{}') AS permission_names,
			r.created_at,
			r.updated_at,
			r.etag
		FROM rbac.custom_roles AS r
		LEFT JOIN rbac.custom_role_permissions AS rp ON rp.role_id = r.id
		LEFT JOIN rbac.permissions AS p ON p.id = rp.permission_id
		WHERE r.org_id = @org_id
		GROUP BY r.id
		ORDER BY r.name
		LIMIT @page_size OFFSET @page_offset`

	return pgdb.TypedQuery[CustomRole]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[CustomRole],
		Expect: pgdb.ExpectMany,
	}
}

func customRoleByExternalIDQuery(orgID int, roleID uuid.UUID) pgdb.TypedQuery[CustomRole] {
	params := pgx.NamedArgs{"org_id": orgID, "role_id": roleID}
	const sql = `
		SELECT
			r.id,
			r.external_id,
			r.name,
			COALESCE(array_agg(p.name ORDER BY p.name) FILTER (WHERE p.name IS NOT NULL), '{}') AS permission_names,
			r.created_at,
			r.updated_at,
			r.etag
		FROM rbac.custom_roles AS r
		LEFT JOIN rbac.custom_role_permissions AS rp ON rp.role_id = r.id
		LEFT JOIN rbac.permissions AS p ON p.id = rp.permission_id
		WHERE r.org_id = @org_id AND r.external_id = @role_id
		GROUP BY r.id`

	return pgdb.TypedQuery[CustomRole]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[CustomRole],
		Expect: pgdb.ExpectOne,
	}
}

func customRoleCountQuery(orgID int) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{"org_id": orgID}
	const sql = `SELECT COUNT(*) FROM rbac.custom_roles WHERE org_id = @org_id`

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

func userSystemPermissionsByExternalIDQuery(userID uuid.UUID) pgdb.TypedQuery[string] {
	params := pgx.NamedArgs{"user_id": userID}
	const sql = `
		SELECT DISTINCT p.name
		FROM useraccess.users AS u
		JOIN rbac.system_role_assignments AS sra ON sra.user_id = u.id
		JOIN rbac.system_role_permissions AS rp ON rp.role_id = sra.role_id
		JOIN rbac.permissions AS p ON p.id = rp.permission_id
		WHERE u.external_id = @user_id
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

func systemPermissionsRemainAfterUnassignQuery(
	userID uuid.UUID,
	roleName string,
	permissionNames []string,
) pgdb.TypedQuery[bool] {
	params := pgx.NamedArgs{
		"user_id":          userID,
		"role_name":        roleName,
		"permission_names": permissionNames,
	}
	const sql = `
		WITH excluded_assignment AS (
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
