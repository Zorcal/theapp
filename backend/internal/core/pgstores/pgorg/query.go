package pgorg

import (
	"fmt"
	"strings"
	"uuid"

	"github.com/jackc/pgx/v5"

	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

func createOrganizationQuery(co CreateOrganization) pgdb.TypedQuery[Organization] {
	params := pgx.NamedArgs{"name": co.Name, "control_project_name": co.ControlProjectName}

	// The control project is created in the same statement as the organization, so the two rows
	// are guaranteed to exist together.
	const sql = `
		WITH
			new_org AS (
				INSERT INTO org.organizations (name, created_at)
				VALUES (@name, NOW())
				RETURNING id, name, created_at, updated_at
			),
			new_control_project AS (
				INSERT INTO org.projects (org_id, name, is_control, created_at, etag)
				SELECT id, @control_project_name, true, NOW(), gen_random_uuid() FROM new_org
				RETURNING id, org_id
			)
		SELECT new_org.id, new_org.name, new_org.created_at, new_org.updated_at, new_control_project.id AS control_project_id
		FROM new_org
		JOIN new_control_project ON new_control_project.org_id = new_org.id`

	return pgdb.TypedQuery[Organization]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[Organization],
		Expect: pgdb.ExpectOne,
	}
}

func organizationUsersQuery(orgID int, filter OrganizationUserFilter, pageSize, pageOffset int) pgdb.TypedQuery[pguser.User] {
	params := pgx.NamedArgs{
		"org_id":      orgID,
		"page_size":   pageSize,
		"page_offset": pageOffset,
	}
	sql := fmt.Sprintf(`
		SELECT usr.id, usr.external_id, usr.email, usr.name, usr.email_verified_at,
		       usr.created_at, usr.updated_at, usr.etag
		FROM org.organizations AS organization
		JOIN org.org_membership AS membership ON membership.org_id = organization.id
		JOIN useraccess.users AS usr ON usr.id = membership.user_id
		%s
		ORDER BY usr.email, usr.id
		LIMIT @page_size OFFSET @page_offset`, organizationUsersWhereClause(filter, params))

	return pgdb.TypedQuery[pguser.User]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[pguser.User],
		Expect: pgdb.ExpectMany,
	}
}

func organizationUserCountQuery(orgID int, filter OrganizationUserFilter) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{"org_id": orgID}
	sql := fmt.Sprintf(`
		SELECT (
			SELECT COUNT(*)
			FROM org.organizations AS organization
			JOIN org.org_membership AS membership ON membership.org_id = organization.id
			JOIN useraccess.users AS usr ON usr.id = membership.user_id
			%s
		)
		FROM org.organizations AS organization_scope
		%s`, organizationUsersWhereClause(filter, params), organizationUsersScopeWhereClause(filter, params))

	return pgdb.TypedQuery[int]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowTo[int],
		Expect: pgdb.ExpectOne,
	}
}

// organizationUsersScopeWhereClause ensures the selected project belongs to the organization.
// Keeping this anchor outside the aggregate distinguishes an invalid project from a valid project
// without matching users.
func organizationUsersScopeWhereClause(filter OrganizationUserFilter, params pgx.NamedArgs) string {
	clauses := []string{
		"organization_scope.id = @org_id",
	}

	if filter.ProjectID != nil {
		params["project_id"] = *filter.ProjectID
		clauses = append(clauses, `EXISTS (
			SELECT FROM org.projects AS project
			WHERE project.id = @project_id AND project.org_id = organization_scope.id
		)`)
	}

	return "WHERE " + strings.Join(clauses, " AND ")
}

// organizationUsersWhereClause builds the organization anchor and optional project filter,
// adding any required named parameters to params as a side effect.
func organizationUsersWhereClause(filter OrganizationUserFilter, params pgx.NamedArgs) string {
	clauses := []string{
		"organization.id = @org_id",
	}

	if filter.ProjectID != nil {
		params["project_id"] = *filter.ProjectID
		clauses = append(
			clauses,
			`EXISTS (
SELECT FROM org.projects AS project
WHERE project.id = @project_id AND project.org_id = organization.id
		)`,
			`EXISTS (
SELECT FROM rbac.accessible_project_ids(usr.id) AS accessible
WHERE accessible.project_id = @project_id
)`,
		)
	}

	return "WHERE " + strings.Join(clauses, " AND ")
}

func organizationByNameQuery(name string) pgdb.TypedQuery[Organization] {
	params := pgx.NamedArgs{"name": name}
	const sql = `
		SELECT o.id, o.name, o.created_at, o.updated_at, p.id AS control_project_id
		FROM org.organizations AS o
		JOIN org.projects AS p ON p.org_id = o.id AND p.is_control
		WHERE o.name = @name`

	return pgdb.TypedQuery[Organization]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[Organization],
		Expect: pgdb.ExpectOne,
	}
}

func projectByNameQuery(orgID int, name string) pgdb.TypedQuery[Project] {
	params := pgx.NamedArgs{"org_id": orgID, "name": name}
	const sql = `
		SELECT id, org_id, name, is_control, created_at, updated_at, etag
		FROM org.projects
		WHERE org_id = @org_id AND lower(name) = lower(@name)`

	return pgdb.TypedQuery[Project]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[Project],
		Expect: pgdb.ExpectOne,
	}
}

func createProjectQuery(cp CreateProject) pgdb.TypedQuery[Project] {
	params := pgx.NamedArgs{"org_id": cp.OrgID, "name": cp.Name}

	// Resolve cp.OrgID via a join rather than depending on the org_id foreign key, so an unknown org yields zero rows
	// instead of a distinct constraint-violation error.
	const sql = `
		INSERT INTO org.projects (org_id, name, is_control, created_at, etag)
		SELECT o.id, @name, false, NOW(), gen_random_uuid()
		FROM org.organizations AS o
		WHERE o.id = @org_id
		RETURNING id, org_id, name, is_control, created_at, updated_at, etag`

	return pgdb.TypedQuery[Project]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[Project],
		Expect: pgdb.ExpectOne,
	}
}

func addOrganizationMemberQuery(userID uuid.UUID, orgID int) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{"user_id": userID, "org_id": orgID}
	const sql = `
		INSERT INTO org.org_membership (org_id, user_id)
		SELECT organization.id, usr.id
		FROM org.organizations AS organization
		CROSS JOIN useraccess.users AS usr
		WHERE organization.id = @org_id
			AND usr.external_id = @user_id
		RETURNING org_id`

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

func ensureOrganizationMemberQuery(userID uuid.UUID, orgID int) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{"user_id": userID, "org_id": orgID}
	const sql = `
		WITH
			target AS (
				SELECT organization.id AS org_id, usr.id AS user_id
				FROM org.organizations AS organization
				CROSS JOIN useraccess.users AS usr
				WHERE organization.id = @org_id AND usr.external_id = @user_id
			),
			inserted AS (
				INSERT INTO org.org_membership (org_id, user_id)
				SELECT org_id, user_id FROM target
				ON CONFLICT DO NOTHING
			)
		SELECT org_id FROM target`

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

func accessibleProjectsQuery(userID uuid.UUID, filter ProjectFilter, pageSize, pageOffset int) pgdb.TypedQuery[Project] {
	params := pgx.NamedArgs{
		"user_id":     userID,
		"page_size":   pageSize,
		"page_offset": pageOffset,
	}

	sql := fmt.Sprintf(`
		WITH
			target_user AS (
				SELECT id
				FROM useraccess.users
				WHERE external_id = @user_id
			)
		SELECT project.id, project.org_id, project.name, project.is_control, project.created_at, project.updated_at, project.etag
		FROM target_user AS target
		CROSS JOIN LATERAL rbac.accessible_project_ids(target.id) AS accessible
		JOIN org.projects AS project ON project.id = accessible.project_id
		%s
		ORDER BY project.org_id, project.name COLLATE org.project_name_natural
		LIMIT @page_size OFFSET @page_offset`, projectWhereClause(filter, params))

	return pgdb.TypedQuery[Project]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[Project],
		Expect: pgdb.ExpectMany,
	}
}

func accessibleProjectCountQuery(userID uuid.UUID, filter ProjectFilter) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{
		"user_id": userID,
	}

	// Anchor the result on target_user so an existing user without assignments returns zero while a
	// missing user returns no row.
	sql := fmt.Sprintf(`
		WITH
			target_user AS (
				SELECT id
				FROM useraccess.users
				WHERE external_id = @user_id
			)
		SELECT count(project.id)
		FROM target_user AS target
		LEFT JOIN LATERAL rbac.accessible_project_ids(target.id) AS accessible ON true
		LEFT JOIN (
			SELECT id
			FROM org.projects AS project
			%s
		) AS project ON project.id = accessible.project_id
		GROUP BY target.id`, projectWhereClause(filter, params))

	return pgdb.TypedQuery[int]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowTo[int],
		Expect: pgdb.ExpectOne,
	}
}

// projectWhereClause builds an optional WHERE clause from f, adding any required
// named parameters to params as a side effect.
func projectWhereClause(f ProjectFilter, params pgx.NamedArgs) string {
	var clauses []string
	if name := strings.TrimSpace(f.Name); name != "" {
		params["name_prefix"] = name + "%"
		clauses = append(clauses, "project.name ILIKE @name_prefix")
	}
	if len(clauses) == 0 {
		return ""
	}
	return "WHERE " + strings.Join(clauses, " AND ")
}
