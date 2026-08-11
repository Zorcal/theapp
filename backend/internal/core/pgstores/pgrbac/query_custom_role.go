package pgrbac

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

func createCustomRoleQuery(cr CreateCustomRole, managedKey *string) pgdb.TypedQuery[CustomRole] {
	params := pgx.NamedArgs{
		"org_id":           cr.OrgID,
		"name":             cr.Name,
		"managed_key":      managedKey,
		"permission_names": cr.PermissionNames,
	}

	// Execution returns sql.ErrNoRows when the organization or any requested permission does not
	// exist, and pgdb.ErrAlreadyExists when the organization already has the role name.
	const sql = `
		WITH
			resolved_permissions AS (
				SELECT permission_ids, permission_names
				FROM rbac.resolve_permissions(@permission_names::text[])
			),
			new_role AS (
				INSERT INTO rbac.custom_roles (external_id, org_id, name, managed_key, created_at, etag)
				SELECT gen_random_uuid(), organization.id, @name, @managed_key, NOW(), gen_random_uuid()
				FROM org.organizations AS organization
				CROSS JOIN resolved_permissions
				WHERE organization.id = @org_id
				RETURNING id, external_id, name, managed_key, created_at, updated_at, etag
			),
			inserted_permissions AS (
				INSERT INTO rbac.custom_role_permissions (role_id, permission_id)
				SELECT new_role.id, permission.id
				FROM new_role
				CROSS JOIN resolved_permissions
				CROSS JOIN LATERAL unnest(resolved_permissions.permission_ids) AS permission(id)
				RETURNING permission_id
			)
		SELECT
			new_role.id,
			new_role.external_id,
			new_role.name,
			new_role.managed_key,
			resolved_permissions.permission_names,
			new_role.created_at,
			new_role.updated_at,
			new_role.etag
		FROM new_role
		CROSS JOIN resolved_permissions`

	return pgdb.TypedQuery[CustomRole]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[CustomRole],
		Expect: pgdb.ExpectOne,
	}
}

// updateCustomRoleQuery updates fields stored on the custom role row. Permission replacement is
// gated on resolving every requested permission name; its caller replaces the join-table rows with
// separate queries in the same transaction.
func updateCustomRoleQuery(ur UpdateCustomRole) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{
		"org_id":  ur.OrgID,
		"role_id": ur.ExternalID,
		"etag":    ur.ETag,
	}

	var (
		setClauses []string
		// fromClause gates the update on resolving the complete permission set when replacement is
		// selected. It remains empty for updates that do not touch permissions.
		fromClause string
	)
	if ur.Fields.Name {
		setClauses = append(setClauses, "name = @name")
		params["name"] = ur.Name
	}
	if ur.Fields.PermissionNames {
		params["permission_names"] = ur.PermissionNames
		fromClause = "FROM rbac.resolve_permissions(@permission_names::text[]) AS resolved_permissions"
	}

	setClauses = append(setClauses, "updated_at = NOW()", "etag = gen_random_uuid()")

	// Execution returns sql.ErrNoRows when the organization does not own the role or any requested
	// permission does not exist, pgdb.ErrETagMismatch when the role has changed, and
	// pgdb.ErrAlreadyExists when the organization already has the role name.
	sql := fmt.Sprintf(
		`WITH updated AS (
			UPDATE rbac.custom_roles
			SET %[1]s
			%[2]s
			WHERE org_id = @org_id
				AND external_id = @role_id
				AND etag = @etag
			RETURNING id
		)
		SELECT
			id AS role_id,
			TRUE AS is_updated,
			TRUE AS etag_matches
		FROM updated

		UNION ALL

		SELECT
			role.id AS role_id,
			FALSE AS is_updated,
			role.etag = @etag AS etag_matches
		FROM rbac.custom_roles AS role
		WHERE role.org_id = @org_id
			AND role.external_id = @role_id
			AND NOT EXISTS (SELECT 1 FROM updated)`,
		strings.Join(setClauses, ", "),
		fromClause,
	)

	return pgdb.TypedQuery[int]{
		SQL:  sql,
		Args: params,
		Scan: func(row pgx.CollectableRow) (int, error) {
			var (
				id            int
				isUpdated     bool
				isETagMatched bool
			)
			if err := row.Scan(&id, &isUpdated, &isETagMatched); err != nil {
				return 0, err
			}
			if !isETagMatched {
				return 0, pgdb.ErrETagMismatch
			}
			if !isUpdated {
				return 0, pgx.ErrNoRows
			}
			return id, nil
		},
		Expect: pgdb.ExpectOne,
	}
}

func insertCustomRolePermissionsQuery(roleID uuid.UUID, permNames []string) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{"role_id": roleID, "permission_names": permNames}
	const sql = `
		INSERT INTO rbac.custom_role_permissions (role_id, permission_id)
		SELECT r.id, permission.id
		FROM rbac.custom_roles AS r
		CROSS JOIN rbac.resolve_permissions(@permission_names::text[]) AS resolved
		CROSS JOIN LATERAL unnest(resolved.permission_ids) AS permission(id)
		WHERE r.external_id = @role_id
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
		"etag":                    mp.ETag,
		"add_permission_names":    mp.AddPermissionNames,
		"remove_permission_names": mp.RemovePermissionNames,
	}

	// Resolving and validating permission names in this statement keeps validation and mutation
	// atomic: an unknown name prevents every requested change instead of allowing a partial update.
	// Execution returns sql.ErrNoRows when the organization does not own the role or any requested
	// permission does not exist.
	const sql = `
		WITH
			resolved_permissions AS (
				SELECT permission_ids, permission_names
				FROM rbac.resolve_permissions(
					@add_permission_names::text[] || @remove_permission_names::text[]
				)
			),
			valid_permissions AS (
				SELECT permission.id, permission.name
				FROM resolved_permissions
				CROSS JOIN LATERAL unnest(
					resolved_permissions.permission_ids,
					resolved_permissions.permission_names
				) AS permission(id, name)
			),
			target_role AS (
				SELECT r.id
				FROM rbac.custom_roles AS r
				CROSS JOIN resolved_permissions
				WHERE r.org_id = @org_id
					AND r.external_id = @role_id
					AND r.etag = @etag
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
		-- Return enough state to distinguish a missing role, stale ETag, and invalid permissions.
		SELECT
			(SELECT id FROM target_role) AS role_id,
			EXISTS (
				SELECT 1 FROM rbac.custom_roles
				WHERE org_id = @org_id AND external_id = @role_id
			) AS role_exists,
			EXISTS (SELECT 1 FROM resolved_permissions) AS permissions_valid`

	return pgdb.TypedQuery[int]{
		SQL:  sql,
		Args: params,
		Scan: func(row pgx.CollectableRow) (int, error) {
			var (
				id                  *int
				isExists            bool
				arePermissionsValid bool
			)
			if err := row.Scan(&id, &isExists, &arePermissionsValid); err != nil {
				return 0, err
			}
			if id == nil {
				if !arePermissionsValid {
					return 0, pgx.ErrNoRows
				}
				if isExists {
					return 0, pgdb.ErrETagMismatch
				}
				return 0, pgx.ErrNoRows
			}
			return *id, nil
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
			r.managed_key,
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
			r.managed_key,
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

func customRoleHasProjectAssignmentsQuery(roleID uuid.UUID) pgdb.TypedQuery[bool] {
	params := pgx.NamedArgs{"role_id": roleID}
	const sql = `
		SELECT EXISTS (
			SELECT 1
			FROM rbac.project_role_assignments AS assignment
			JOIN rbac.custom_roles AS role ON role.id = assignment.role_id
			WHERE role.external_id = @role_id
		)`

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

func assignCustomRoleToProjectQuery(userID, roleID uuid.UUID, projectID int) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{
		"user_id":    userID,
		"role_id":    roleID,
		"project_id": projectID,
	}
	const sql = `
		INSERT INTO rbac.project_role_assignments (user_id, project_id, role_id, org_id)
		SELECT u.id, p.id, r.id, p.org_id
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

func userProjectCustomRolesQuery(userID uuid.UUID, projectID, pageSize, pageOffset int) pgdb.TypedQuery[CustomRole] {
	params := pgx.NamedArgs{"user_id": userID, "project_id": projectID, "page_size": pageSize, "page_offset": pageOffset}
	const sql = `
		SELECT
			r.id,
			r.external_id,
			r.name,
			r.managed_key,
			COALESCE(array_agg(p.name ORDER BY p.name) FILTER (WHERE p.name IS NOT NULL), '{}'),
			r.created_at,
			r.updated_at,
			r.etag
		FROM useraccess.users AS u
		JOIN org.projects AS proj ON proj.id = @project_id
		JOIN org.org_membership AS m ON m.user_id = u.id AND m.org_id = proj.org_id
		JOIN rbac.project_role_assignments AS a ON a.user_id = u.id AND a.project_id = proj.id
		JOIN rbac.custom_roles AS r ON r.id = a.role_id AND r.org_id = proj.org_id
		LEFT JOIN rbac.custom_role_permissions AS rp ON rp.role_id = r.id
		LEFT JOIN rbac.permissions AS p ON p.id = rp.permission_id
		WHERE u.external_id = @user_id
		GROUP BY r.id
		ORDER BY r.name, r.id
		LIMIT @page_size OFFSET @page_offset`

	return pgdb.TypedQuery[CustomRole]{
		SQL:  sql,
		Args: params,
		Scan: func(row pgx.CollectableRow) (CustomRole, error) {
			var role CustomRole
			return role, row.Scan(&role.ID, &role.ExternalID, &role.Name, &role.ManagedKey, &role.PermissionNames, &role.CreatedAt, &role.UpdatedAt, &role.ETag)
		},
		Expect: pgdb.ExpectMany,
	}
}

func userProjectCustomRoleCountQuery(userID uuid.UUID, projectID int) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{"user_id": userID, "project_id": projectID}
	const sql = `
		SELECT COUNT(a.role_id)
		FROM useraccess.users AS u
		JOIN org.projects AS proj ON proj.id = @project_id
		JOIN org.org_membership AS m ON m.user_id = u.id AND m.org_id = proj.org_id
		LEFT JOIN rbac.project_role_assignments AS a ON a.user_id = u.id AND a.project_id = proj.id
		WHERE u.external_id = @user_id
		GROUP BY u.id, proj.id`

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

func userOrgCustomRolesQuery(userID uuid.UUID, orgID, pageSize, pageOffset int) pgdb.TypedQuery[CustomRole] {
	params := pgx.NamedArgs{"user_id": userID, "org_id": orgID, "page_size": pageSize, "page_offset": pageOffset}
	const sql = `
		SELECT
			r.id,
			r.external_id,
			r.name,
			r.managed_key,
			COALESCE(array_agg(p.name ORDER BY p.name) FILTER (WHERE p.name IS NOT NULL), '{}'),
			r.created_at,
			r.updated_at,
			r.etag
		FROM useraccess.users AS u
		JOIN org.org_membership AS m ON m.user_id = u.id AND m.org_id = @org_id
		JOIN rbac.org_role_assignments AS a ON a.user_id = u.id AND a.org_id = m.org_id
		JOIN rbac.custom_roles AS r ON r.id = a.role_id AND r.org_id = m.org_id
		LEFT JOIN rbac.custom_role_permissions AS rp ON rp.role_id = r.id
		LEFT JOIN rbac.permissions AS p ON p.id = rp.permission_id
		WHERE u.external_id = @user_id
		GROUP BY r.id
		ORDER BY r.name, r.id
		LIMIT @page_size OFFSET @page_offset`

	return pgdb.TypedQuery[CustomRole]{
		SQL:  sql,
		Args: params,
		Scan: func(row pgx.CollectableRow) (CustomRole, error) {
			var role CustomRole
			return role, row.Scan(&role.ID, &role.ExternalID, &role.Name, &role.ManagedKey, &role.PermissionNames, &role.CreatedAt, &role.UpdatedAt, &role.ETag)
		},
		Expect: pgdb.ExpectMany,
	}
}

func userOrgCustomRoleCountQuery(userID uuid.UUID, orgID int) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{"user_id": userID, "org_id": orgID}
	const sql = `
		SELECT COUNT(a.role_id)
		FROM useraccess.users AS u
		JOIN org.org_membership AS m ON m.user_id = u.id AND m.org_id = @org_id
		LEFT JOIN rbac.org_role_assignments AS a ON a.user_id = u.id AND a.org_id = m.org_id
		WHERE u.external_id = @user_id
		GROUP BY u.id, m.org_id`

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
