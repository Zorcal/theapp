package pgrbac

import (
	"github.com/jackc/pgx/v5"

	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

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
