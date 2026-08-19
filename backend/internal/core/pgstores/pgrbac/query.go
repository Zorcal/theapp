package pgrbac

import (
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

// projectPermissionsQuery resolves projectID's org and the effective permissions userID holds for
// it. Anchoring on both the user and project means a nonexistent ID yields zero rows.
func projectPermissionsQuery(userID uuid.UUID, projectID int) pgdb.TypedQuery[ProjectPermissions] {
	params := pgx.NamedArgs{"user_id": userID, "project_id": projectID}
	const sql = `
		SELECT
			proj.org_id,
			COALESCE(array_agg(p.name ORDER BY p.name) FILTER (WHERE p.name IS NOT NULL), '{}') AS permission_names
		FROM org.projects AS proj
		JOIN useraccess.users AS u ON u.external_id = @user_id
		LEFT JOIN LATERAL rbac.project_permission_ids(u.id, proj.id) AS granted ON true
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

// orgPermissionsByProjectIDQuery resolves the effective organization permissions userID holds for
// projectID's organization. Project-scoped assignments are omitted. Anchoring on both the user and
// project means a nonexistent ID yields zero rows.
func orgPermissionsByProjectIDQuery(userID uuid.UUID, projectID int) pgdb.TypedQuery[OrgPermissions] {
	params := pgx.NamedArgs{"user_id": userID, "project_id": projectID}
	const sql = `
		SELECT
			proj.org_id,
			COALESCE(array_agg(p.name ORDER BY p.name) FILTER (WHERE p.name IS NOT NULL), '{}') AS permission_names
		FROM org.projects AS proj
		JOIN useraccess.users AS u ON u.external_id = @user_id
		LEFT JOIN LATERAL rbac.org_permission_ids(u.id, proj.org_id) AS granted ON true
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
