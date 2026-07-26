package pgrbac

import (
	"fmt"
	"strings"

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
			-- Pair the new role with every validated permission. The CROSS JOIN produces no rows
			-- when new_role is empty, preserving the organization and permission validation gate.
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

// updateCustomRoleQuery updates fields stored on the custom role row. Its caller must validate and
// replace selected permission names with separate queries in the same transaction.
func updateCustomRoleQuery(ur UpdateCustomRole) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{
		"org_id":  ur.OrgID,
		"role_id": ur.ExternalID,
	}
	var setClauses []string

	if ur.Fields.Name {
		setClauses = append(setClauses, "name = @name")
		params["name"] = ur.Name
	}

	setClauses = append(setClauses, "updated_at = NOW()", "etag = gen_random_uuid()")

	// Execution returns sql.ErrNoRows when the organization does not own the role and
	// pgdb.ErrAlreadyExists when the organization already has the role name.
	sql := fmt.Sprintf(
		`UPDATE rbac.custom_roles
		SET %[1]s
		WHERE org_id = @org_id
			AND external_id = @role_id
		RETURNING id`,
		strings.Join(setClauses, ", "),
	)

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

func validateCustomRolePermsQuery(orgID int, roleID uuid.UUID, permNames []string) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{
		"org_id":           orgID,
		"role_id":          roleID,
		"permission_names": permNames,
	}

	// Execution returns sql.ErrNoRows when the organization does not own the role or any requested
	// permission does not exist.
	const sql = `
		WITH
			-- Deduplicate the complete replacement permission set before validating it.
			requested_permissions AS (
				SELECT DISTINCT name
				FROM unnest(@permission_names::text[]) AS requested(name)
			),
			-- Resolve every requested permission name against the permission registry.
			valid_permissions AS (
				SELECT p.name
				FROM rbac.permissions AS p
				JOIN requested_permissions AS requested ON requested.name = p.name
			)
		-- Return the role id only when it belongs to the organization and every permission exists.
		SELECT r.id
		FROM rbac.custom_roles AS r
		WHERE r.org_id = @org_id
			AND r.external_id = @role_id
			AND NOT EXISTS (
				SELECT 1
				FROM requested_permissions AS requested
				LEFT JOIN valid_permissions AS valid ON valid.name = requested.name
				WHERE valid.name IS NULL
			)`

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

func insertCustomRolePermissionsQuery(roleID uuid.UUID, permNames []string) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{"role_id": roleID, "permission_names": permNames}
	const sql = `
		INSERT INTO rbac.custom_role_permissions (role_id, permission_id)
		SELECT r.id, p.id
		FROM rbac.custom_roles AS r
		CROSS JOIN rbac.permissions AS p
		WHERE r.external_id = @role_id
			AND p.name = ANY(@permission_names::text[])
		RETURNING permission_id`

	return pgdb.TypedQuery[int]{
		SQL:  sql,
		Args: params,
		Scan: func(row pgx.CollectableRow) (int, error) {
			var id int
			return id, row.Scan(&id)
		},
		Expect: pgdb.ExpectMany,
	}
}

func modifyCustomRolePermissionsQuery(mp ModifyCustomRolePermissions) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{
		"org_id":                  mp.OrgID,
		"role_id":                 mp.ExternalID,
		"add_permission_names":    mp.AddPermissionNames,
		"remove_permission_names": mp.RemovePermissionNames,
	}

	// Resolving and validating permission names in this statement keeps validation and mutation
	// atomic: an unknown name prevents every requested change instead of allowing a partial update.
	// Execution returns sql.ErrNoRows when the organization does not own the role or any requested
	// permission does not exist.
	const sql = `
		WITH
			-- Combine the add/remove arrays solely to validate every requested permission name.
			-- This is not a replacement permission set for the role.
			requested_permissions AS (
				SELECT DISTINCT name
				FROM unnest(
					@add_permission_names::text[] || @remove_permission_names::text[]
				) AS requested(name)
			),
			-- Resolve every requested permission name to the permission ID used by the join table.
			valid_permissions AS (
				SELECT p.id, p.name
				FROM rbac.permissions AS p
				JOIN requested_permissions AS requested ON requested.name = p.name
			),
			-- Gate every mutation behind role ownership and permission validation. An empty
			-- target_role prevents all subsequent permission and metadata changes.
			target_role AS (
				SELECT r.id
				FROM rbac.custom_roles AS r
				WHERE r.org_id = @org_id
					AND r.external_id = @role_id
					AND NOT EXISTS (
						SELECT 1
						FROM requested_permissions AS requested
						LEFT JOIN valid_permissions AS valid ON valid.name = requested.name
						WHERE valid.id IS NULL
					)
			),
			-- Removing a permission the role does not hold produces no row.
			removed_permissions AS (
				DELETE FROM rbac.custom_role_permissions
				WHERE role_id = (SELECT id FROM target_role)
					AND permission_id IN (
						SELECT id
						FROM valid_permissions
						WHERE name = ANY(@remove_permission_names::text[])
					)
				RETURNING permission_id
			),
			-- Pair the single target role with every permission to add. The CROSS JOIN produces
			-- no rows when target_role is empty, preserving the ownership and validation gate.
			-- ON CONFLICT makes adding an existing permission a no-op.
			added_permissions AS (
				INSERT INTO rbac.custom_role_permissions (role_id, permission_id)
				SELECT target_role.id, valid_permissions.id
				FROM target_role
				CROSS JOIN valid_permissions
				WHERE valid_permissions.name = ANY(@add_permission_names::text[])
				ON CONFLICT DO NOTHING
				RETURNING permission_id
			),
			-- Change role metadata only when the permission set changed.
			updated_role AS (
				UPDATE rbac.custom_roles
				SET updated_at = NOW(), etag = gen_random_uuid()
				WHERE id = (SELECT id FROM target_role)
					AND (
						EXISTS (SELECT 1 FROM removed_permissions)
						OR EXISTS (SELECT 1 FROM added_permissions)
				)
				RETURNING id
			)
		-- Returning the target ID makes a missing role or permission surface as sql.ErrNoRows.
		SELECT id
		FROM target_role`

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

func deleteCustomRoleQuery(orgID int, roleID uuid.UUID) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{"org_id": orgID, "role_id": roleID}
	const sql = `
		DELETE FROM rbac.custom_roles
		WHERE org_id = @org_id AND external_id = @role_id
		RETURNING id`

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

func deleteCustomRolePermissionsQuery(orgID int, roleID uuid.UUID) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{"org_id": orgID, "role_id": roleID}
	const sql = `
		DELETE FROM rbac.custom_role_permissions
		WHERE role_id = (
			SELECT id
			FROM rbac.custom_roles
			WHERE org_id = @org_id AND external_id = @role_id
		)
		RETURNING permission_id`

	return pgdb.TypedQuery[int]{
		SQL:  sql,
		Args: params,
		Scan: func(row pgx.CollectableRow) (int, error) {
			var id int
			return id, row.Scan(&id)
		},
		Expect: pgdb.ExpectMany,
	}
}

func deleteCustomRoleProjectAssignmentsQuery(orgID int, roleID uuid.UUID) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{"org_id": orgID, "role_id": roleID}
	const sql = `
		DELETE FROM rbac.project_role_assignments
		WHERE role_id = (
			SELECT id
			FROM rbac.custom_roles
			WHERE org_id = @org_id AND external_id = @role_id
		)
		RETURNING user_id`

	return pgdb.TypedQuery[int]{
		SQL:  sql,
		Args: params,
		Scan: func(row pgx.CollectableRow) (int, error) {
			var id int
			return id, row.Scan(&id)
		},
		Expect: pgdb.ExpectMany,
	}
}

func deleteCustomRoleOrgAssignmentsQuery(orgID int, roleID uuid.UUID) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{"org_id": orgID, "role_id": roleID}
	const sql = `
		DELETE FROM rbac.org_role_assignments
		WHERE role_id = (
			SELECT id
			FROM rbac.custom_roles
			WHERE org_id = @org_id AND external_id = @role_id
		)
		RETURNING user_id`

	return pgdb.TypedQuery[int]{
		SQL:  sql,
		Args: params,
		Scan: func(row pgx.CollectableRow) (int, error) {
			var id int
			return id, row.Scan(&id)
		},
		Expect: pgdb.ExpectMany,
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

func assignCustomRoleToProjectQuery(userID, roleID uuid.UUID, projectID int) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{
		"user_id":    userID,
		"role_id":    roleID,
		"project_id": projectID,
	}
	const sql = `
		INSERT INTO rbac.project_role_assignments (user_id, role_id, project_id)
		SELECT u.id, r.id, p.id
		FROM (
			SELECT id
			FROM useraccess.users
			WHERE external_id = @user_id
		) AS u
		CROSS JOIN (
			SELECT id, org_id
			FROM rbac.custom_roles
			WHERE external_id = @role_id
		) AS r
		CROSS JOIN (
			SELECT id, org_id
			FROM org.projects
			WHERE id = @project_id
		) AS p
		WHERE r.org_id = p.org_id
			AND EXISTS (
				SELECT 1
				FROM org.org_membership AS m
				WHERE m.user_id = u.id
					AND m.org_id = p.org_id
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

func unassignCustomRoleFromProjectQuery(userID, roleID uuid.UUID, projectID int) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{
		"user_id":    userID,
		"role_id":    roleID,
		"project_id": projectID,
	}
	const sql = `
		DELETE FROM rbac.project_role_assignments
		WHERE user_id = (
			SELECT u.id
			FROM useraccess.users AS u
			JOIN org.org_membership AS m ON m.user_id = u.id
			JOIN org.projects AS p ON p.org_id = m.org_id
			WHERE u.external_id = @user_id
				AND p.id = @project_id
		)
			AND role_id = (
				SELECT r.id
				FROM rbac.custom_roles AS r
				JOIN org.projects AS p ON p.org_id = r.org_id
				WHERE r.external_id = @role_id
					AND p.id = @project_id
			)
			AND project_id = @project_id
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

func assignCustomRoleToOrgQuery(userID, roleID uuid.UUID, orgID int) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{
		"user_id": userID,
		"role_id": roleID,
		"org_id":  orgID,
	}
	const sql = `
		INSERT INTO rbac.org_role_assignments (user_id, role_id, org_id)
		SELECT u.id, r.id, m.org_id
		FROM useraccess.users AS u
		JOIN org.org_membership AS m
			ON m.user_id = u.id
			AND m.org_id = @org_id
		JOIN rbac.custom_roles AS r
			ON r.org_id = m.org_id
			AND r.external_id = @role_id
		WHERE u.external_id = @user_id
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

func unassignCustomRoleFromOrgQuery(userID, roleID uuid.UUID, orgID int) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{
		"user_id": userID,
		"role_id": roleID,
		"org_id":  orgID,
	}
	const sql = `
		DELETE FROM rbac.org_role_assignments
		WHERE user_id = (
			SELECT u.id
			FROM useraccess.users AS u
			JOIN org.org_membership AS m
				ON m.user_id = u.id
				AND m.org_id = @org_id
			WHERE u.external_id = @user_id
		)
			AND role_id = (
				SELECT id
				FROM rbac.custom_roles
				WHERE external_id = @role_id
					AND org_id = @org_id
			)
			AND org_id = @org_id
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
// holds for it. Project- and org-scoped assignments contribute only while the user is a current
// member of the project's organization; system-scoped assignments contribute unconditionally.
// Anchoring on org.projects means a nonexistent projectID yields zero rows rather than a
// permission set containing only system-scoped grants.
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
			JOIN org.org_membership AS m
				ON m.user_id = pra.user_id
				AND m.org_id = proj.org_id
			JOIN rbac.custom_role_permissions AS rp ON rp.role_id = pra.role_id
			WHERE pra.user_id = @user_id AND pra.project_id = proj.id

			UNION

			SELECT rp.permission_id
			FROM rbac.org_role_assignments AS ora
			JOIN org.org_membership AS m
				ON m.user_id = ora.user_id
				AND m.org_id = ora.org_id
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
