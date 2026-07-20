package pgrbac

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

// staticRolesQuery resolves every static role and the names of the permissions currently granted
// to it.
func staticRolesQuery() pgdb.TypedQuery[RoleStatic] {
	const sql = `
		SELECT
			r.id,
			r.external_id,
			r.name,
			r.created_at,
			r.updated_at,
			COALESCE(array_agg(p.name ORDER BY p.name) FILTER (WHERE p.name IS NOT NULL), '{}') AS permission_names
		FROM rbac.static_roles AS r
		LEFT JOIN rbac.static_role_permissions AS rp ON rp.role_id = r.id
		LEFT JOIN rbac.permissions AS p ON p.id = rp.permission_id
		GROUP BY r.id
		ORDER BY r.name`

	return pgdb.TypedQuery[RoleStatic]{
		SQL:    sql,
		Scan:   pgx.RowToStructByName[RoleStatic],
		Expect: pgdb.ExpectMany,
	}
}

// staticRoleByNameQuery resolves a single static role by name.
func staticRoleByNameQuery(name string) pgdb.TypedQuery[RoleStatic] {
	params := pgx.NamedArgs{"name": name}
	const sql = `
		SELECT
			r.id,
			r.external_id,
			r.name,
			r.created_at,
			r.updated_at,
			COALESCE(array_agg(p.name ORDER BY p.name) FILTER (WHERE p.name IS NOT NULL), '{}') AS permission_names
		FROM rbac.static_roles AS r
		LEFT JOIN rbac.static_role_permissions AS rp ON rp.role_id = r.id
		LEFT JOIN rbac.permissions AS p ON p.id = rp.permission_id
		WHERE r.name = @name
		GROUP BY r.id`

	return pgdb.TypedQuery[RoleStatic]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[RoleStatic],
		Expect: pgdb.ExpectOne,
	}
}

// orgRolesQuery resolves orgID's own custom roles.
func orgRolesQuery(orgID int) pgdb.TypedQuery[RoleCustom] {
	params := pgx.NamedArgs{"org_id": orgID}
	const sql = `
		SELECT
			r.id,
			r.external_id,
			r.name,
			r.org_id,
			r.created_at,
			r.updated_at,
			r.etag,
			COALESCE(array_agg(p.name ORDER BY p.name) FILTER (WHERE p.name IS NOT NULL), '{}') AS permission_names
		FROM rbac.custom_roles AS r
		LEFT JOIN rbac.custom_role_permissions AS rp ON rp.role_id = r.id
		LEFT JOIN rbac.permissions AS p ON p.id = rp.permission_id
		WHERE r.org_id = @org_id
		GROUP BY r.id
		ORDER BY r.name`

	return pgdb.TypedQuery[RoleCustom]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[RoleCustom],
		Expect: pgdb.ExpectMany,
	}
}

// roleByExternalIDQuery resolves a single custom role by its external ID, regardless of its
// owning organization — ownership is checked by the caller. A static role's external ID never
// matches here, since static roles live in a separate table entirely.
func roleByExternalIDQuery(externalID uuid.UUID) pgdb.TypedQuery[RoleCustom] {
	params := pgx.NamedArgs{"external_id": externalID}
	const sql = `
		SELECT
			r.id,
			r.external_id,
			r.name,
			r.org_id,
			r.created_at,
			r.updated_at,
			r.etag,
			COALESCE(array_agg(p.name ORDER BY p.name) FILTER (WHERE p.name IS NOT NULL), '{}') AS permission_names
		FROM rbac.custom_roles AS r
		LEFT JOIN rbac.custom_role_permissions AS rp ON rp.role_id = r.id
		LEFT JOIN rbac.permissions AS p ON p.id = rp.permission_id
		WHERE r.external_id = @external_id
		GROUP BY r.id`

	return pgdb.TypedQuery[RoleCustom]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[RoleCustom],
		Expect: pgdb.ExpectOne,
	}
}

// createRoleQuery inserts a new custom role owned by cr.OrgID, along with a custom_role_permissions
// row for every name in cr.PermissionNames, in a single round trip.
func createRoleQuery(cr CreateRole) pgdb.TypedQuery[RoleCustom] {
	params := pgx.NamedArgs{
		"org_id":           cr.OrgID,
		"name":             cr.Name,
		"permission_names": cr.PermissionNames,
	}
	const sql = `
		WITH new_custom_role AS (
			INSERT INTO rbac.custom_roles (external_id, org_id, name, created_at, etag)
			VALUES (gen_random_uuid(), @org_id, @name, NOW(), gen_random_uuid())
			RETURNING id, external_id, name, org_id, created_at, updated_at, etag
		), inserted_perms AS (
			INSERT INTO rbac.custom_role_permissions (role_id, permission_id)
			SELECT new_custom_role.id, p.id
			FROM new_custom_role, rbac.permissions AS p
			WHERE p.name = ANY(@permission_names)
		)
		SELECT
			new_custom_role.id,
			new_custom_role.external_id,
			new_custom_role.name,
			new_custom_role.org_id,
			new_custom_role.created_at,
			new_custom_role.updated_at,
			new_custom_role.etag,
			(SELECT array_agg(x ORDER BY x) FROM unnest(@permission_names::text[]) AS x) AS permission_names
		FROM new_custom_role`

	return pgdb.TypedQuery[RoleCustom]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[RoleCustom],
		Expect: pgdb.ExpectOne,
	}
}

// updateRoleQuery replaces the name and permission set of the custom role identified by ur.ID
// and owned by ur.OrgID. Matching on org_id, not just id, means a role owned by a different
// organization simply doesn't match, returning sql.ErrNoRows rather than updating a role outside
// the caller's org.
func updateRoleQuery(ur UpdateRole) pgdb.TypedQuery[RoleCustom] {
	params := pgx.NamedArgs{
		"id":               ur.ID,
		"org_id":           ur.OrgID,
		"name":             ur.Name,
		"permission_names": ur.PermissionNames,
	}
	const sql = `
		WITH updated_role AS (
			UPDATE rbac.custom_roles
			SET name = @name, updated_at = NOW(), etag = gen_random_uuid()
			WHERE id = @id AND org_id = @org_id
			RETURNING id, external_id, name, org_id, created_at, updated_at, etag
		), deleted_perms AS (
			-- Only removes permissions absent from the new set, never one being kept: a sibling
			-- CTE that deleted and reinserted the very same (role_id, permission_id) row would
			-- collide, since both run against the same snapshot and neither sees the other's
			-- write within one statement.
			DELETE FROM rbac.custom_role_permissions
			WHERE role_id IN (SELECT id FROM updated_role)
			AND permission_id NOT IN (SELECT id FROM rbac.permissions WHERE name = ANY(@permission_names))
		), inserted_perms AS (
			INSERT INTO rbac.custom_role_permissions (role_id, permission_id)
			SELECT updated_role.id, p.id
			FROM updated_role, rbac.permissions AS p
			WHERE p.name = ANY(@permission_names)
			ON CONFLICT DO NOTHING
		)
		SELECT
			updated_role.id,
			updated_role.external_id,
			updated_role.name,
			updated_role.org_id,
			updated_role.created_at,
			updated_role.updated_at,
			updated_role.etag,
			(SELECT array_agg(x ORDER BY x) FROM unnest(@permission_names::text[]) AS x) AS permission_names
		FROM updated_role`

	return pgdb.TypedQuery[RoleCustom]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[RoleCustom],
		Expect: pgdb.ExpectOne,
	}
}

// deleteRoleQuery deletes the custom role identified by id and owned by orgID, along with its
// custom_role_permissions rows. Matching on org_id, not just id, means a role owned by a
// different organization simply doesn't match, returning sql.ErrNoRows rather than deleting a
// role outside the caller's org.
func deleteRoleQuery(id, orgID int) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{"id": id, "org_id": orgID}
	const sql = `
		WITH deleted_role AS (
			DELETE FROM rbac.custom_roles
			WHERE id = @id AND org_id = @org_id
			RETURNING id
		), deleted_perms AS (
			DELETE FROM rbac.custom_role_permissions
			WHERE role_id IN (SELECT id FROM deleted_role)
		)
		SELECT id FROM deleted_role`

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

// systemPermissionsQuery resolves the permissions userID holds through system-scope role
// assignments only.
func systemPermissionsQuery(userID int) pgdb.TypedQuery[string] {
	params := pgx.NamedArgs{"user_id": userID}
	const sql = `
		SELECT DISTINCT p.name
		FROM rbac.system_role_assignments AS sra
		JOIN rbac.static_role_permissions AS rp ON rp.role_id = sra.role_id
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
			JOIN rbac.static_role_permissions AS rp ON rp.role_id = sra.role_id
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

// assignProjectRoleSQL grants userID roleID for projectID. Assigning a grant the user already
// holds is a no-op, the same reasoning as updateRoleQuery's permission inserts.
const assignProjectRoleSQL = `
	INSERT INTO rbac.project_role_assignments (user_id, role_id, project_id)
	VALUES ($1, $2, $3)
	ON CONFLICT DO NOTHING`

// assignOrgRoleSQL grants userID roleID for orgID. Assigning a grant the user already holds is a
// no-op, the same reasoning as updateRoleQuery's permission inserts.
const assignOrgRoleSQL = `
	INSERT INTO rbac.org_role_assignments (user_id, role_id, org_id)
	VALUES ($1, $2, $3)
	ON CONFLICT DO NOTHING`

const unassignProjectRoleSQL = `
	DELETE FROM rbac.project_role_assignments
	WHERE user_id = $1 AND role_id = $2 AND project_id = $3`

const unassignOrgRoleSQL = `
	DELETE FROM rbac.org_role_assignments
	WHERE user_id = $1 AND role_id = $2 AND org_id = $3`

// deleteProjectAssignmentsForOrgSQL deletes every project-scope assignment of roleID held by
// userID across every project under orgID — the cleanup performed when that same user/role is
// promoted to an org-scope assignment for orgID.
const deleteProjectAssignmentsForOrgSQL = `
	DELETE FROM rbac.project_role_assignments
	WHERE user_id = $1 AND role_id = $2
	AND project_id IN (SELECT id FROM org.projects WHERE org_id = $3)`

func orgAssignmentExistsQuery(userID, roleID, orgID int) pgdb.TypedQuery[bool] {
	params := pgx.NamedArgs{"user_id": userID, "role_id": roleID, "org_id": orgID}
	const sql = `
		SELECT EXISTS (
			SELECT 1 FROM rbac.org_role_assignments
			WHERE user_id = @user_id AND role_id = @role_id AND org_id = @org_id
		) AS exists`

	return pgdb.TypedQuery[bool]{
		SQL:  sql,
		Args: params,
		Scan: func(row pgx.CollectableRow) (bool, error) {
			var exists bool
			return exists, row.Scan(&exists)
		},
		Expect: pgdb.ExpectOne,
	}
}

// roleAssignmentsForUserQuery resolves every custom role userID holds within orgID: project-scope
// rows for projects under orgID, unioned with the org-scope row for orgID itself.
func roleAssignmentsForUserQuery(userID, orgID int) pgdb.TypedQuery[RoleAssignment] {
	params := pgx.NamedArgs{"user_id": userID, "org_id": orgID}
	const sql = `
		SELECT r.external_id AS role_external_id, r.name AS role_name, pra.project_id, NULL::int AS org_id
		FROM rbac.project_role_assignments AS pra
		JOIN rbac.custom_roles AS r ON r.id = pra.role_id
		JOIN org.projects AS p ON p.id = pra.project_id
		WHERE pra.user_id = @user_id AND p.org_id = @org_id

		UNION ALL

		SELECT r.external_id AS role_external_id, r.name AS role_name, NULL::int AS project_id, ora.org_id
		FROM rbac.org_role_assignments AS ora
		JOIN rbac.custom_roles AS r ON r.id = ora.role_id
		WHERE ora.user_id = @user_id AND ora.org_id = @org_id

		ORDER BY role_name`

	return pgdb.TypedQuery[RoleAssignment]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[RoleAssignment],
		Expect: pgdb.ExpectMany,
	}
}

// systemRoleAssignmentsForUserQuery resolves the names of every static role userID holds at
// system scope.
func systemRoleAssignmentsForUserQuery(userID int) pgdb.TypedQuery[string] {
	params := pgx.NamedArgs{"user_id": userID}
	const sql = `
		SELECT r.name
		FROM rbac.system_role_assignments AS sra
		JOIN rbac.static_roles AS r ON r.id = sra.role_id
		WHERE sra.user_id = @user_id
		ORDER BY r.name`

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

// assignSystemRoleQuery grants userID the static role named roleName at system scope. role_id
// resolves against rbac.static_roles specifically, so a name that only matches a custom role
// yields zero rows rather than assigning the wrong kind of role at system scope.
func assignSystemRoleQuery(userID int, roleName string) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{"user_id": userID, "role_name": roleName}
	const sql = `
		INSERT INTO rbac.system_role_assignments (user_id, role_id)
		SELECT @user_id, r.id
		FROM rbac.static_roles AS r
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
