package pgrbac

import (
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

// projectPermissionsQuery resolves projectID's org and the distinct union of permissions userID
// holds for it. Project- and org-scoped assignments contribute only while the user is a current
// member of the project's organization; system-scoped assignments contribute unconditionally.
// Anchoring on both the user and project means a nonexistent ID yields zero rows.
func projectPermissionsQuery(userID uuid.UUID, projectID int) pgdb.TypedQuery[ProjectPermissions] {
	params := pgx.NamedArgs{"user_id": userID, "project_id": projectID}
	const sql = `
		SELECT
			proj.org_id,
			COALESCE(array_agg(p.name ORDER BY p.name) FILTER (WHERE p.name IS NOT NULL), '{}') AS permission_names
		FROM org.projects AS proj
		JOIN useraccess.users AS u ON u.external_id = @user_id
		LEFT JOIN LATERAL (
			SELECT rp.permission_id
			FROM rbac.project_role_assignments AS pra
			JOIN org.org_membership AS m
				ON m.user_id = pra.user_id
				AND m.org_id = proj.org_id
			JOIN rbac.custom_role_permissions AS rp ON rp.role_id = pra.role_id
			WHERE pra.user_id = u.id AND pra.project_id = proj.id

			UNION

			SELECT rp.permission_id
			FROM rbac.org_role_assignments AS ora
			JOIN org.org_membership AS m
				ON m.user_id = ora.user_id
				AND m.org_id = ora.org_id
			JOIN rbac.custom_role_permissions AS rp ON rp.role_id = ora.role_id
			WHERE ora.user_id = u.id AND ora.org_id = proj.org_id

			UNION

			SELECT rp.permission_id
			FROM rbac.system_role_assignments AS sra
			JOIN rbac.system_role_permissions AS rp ON rp.role_id = sra.role_id
			WHERE sra.user_id = u.id
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

// orgPermissionsByProjectIDQuery resolves the distinct union of organization- and system-scoped
// permissions userID holds for projectID's organization. Project-scoped assignments are omitted.
// Anchoring on both the user and project means a nonexistent ID yields zero rows.
func orgPermissionsByProjectIDQuery(userID uuid.UUID, projectID int) pgdb.TypedQuery[OrgPermissions] {
	params := pgx.NamedArgs{"user_id": userID, "project_id": projectID}
	const sql = `
		SELECT
			proj.org_id,
			COALESCE(array_agg(p.name ORDER BY p.name) FILTER (WHERE p.name IS NOT NULL), '{}') AS permission_names
		FROM org.projects AS proj
		JOIN useraccess.users AS u ON u.external_id = @user_id
		LEFT JOIN LATERAL (
			SELECT rp.permission_id
			FROM rbac.org_role_assignments AS ora
			JOIN org.org_membership AS m
				ON m.user_id = ora.user_id
				AND m.org_id = ora.org_id
			JOIN rbac.custom_role_permissions AS rp ON rp.role_id = ora.role_id
			WHERE ora.user_id = u.id AND ora.org_id = proj.org_id

			UNION

			SELECT rp.permission_id
			FROM rbac.system_role_assignments AS sra
			JOIN rbac.system_role_permissions AS rp ON rp.role_id = sra.role_id
			WHERE sra.user_id = u.id
		) AS granted ON true
		LEFT JOIN rbac.permissions AS p ON p.id = granted.permission_id
		WHERE proj.id = @project_id
		GROUP BY proj.org_id`

	return pgdb.TypedQuery[OrgPermissions]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[OrgPermissions],
		Expect: pgdb.ExpectOne,
	}
}
