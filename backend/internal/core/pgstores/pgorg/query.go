package pgorg

import (
	"github.com/jackc/pgx/v5"

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
				INSERT INTO org.projects (org_id, name, is_control, created_at)
				SELECT id, @control_project_name, true, NOW() FROM new_org
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

// projectByNameQuery matches name case-insensitively, mirroring the org_id/lower(name) unique
// index that enforces project name uniqueness within an org.
func projectByNameQuery(orgID int, name string) pgdb.TypedQuery[Project] {
	params := pgx.NamedArgs{"org_id": orgID, "name": name}
	const sql = `
		SELECT id, org_id, name, is_control, created_at, updated_at
		FROM org.projects
		WHERE org_id = @org_id AND lower(name) = lower(@name)`

	return pgdb.TypedQuery[Project]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[Project],
		Expect: pgdb.ExpectOne,
	}
}

func projectByIDQuery(id int) pgdb.TypedQuery[Project] {
	params := pgx.NamedArgs{"id": id}
	const sql = `
		SELECT id, org_id, name, is_control, created_at, updated_at
		FROM org.projects
		WHERE id = @id`

	return pgdb.TypedQuery[Project]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[Project],
		Expect: pgdb.ExpectOne,
	}
}

func isOrgMemberQuery(userID, orgID int) pgdb.TypedQuery[bool] {
	params := pgx.NamedArgs{"user_id": userID, "org_id": orgID}
	const sql = `
		SELECT EXISTS (
			SELECT 1 FROM org.org_membership WHERE user_id = @user_id AND org_id = @org_id
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

func createProjectQuery(cp CreateProject) pgdb.TypedQuery[Project] {
	params := pgx.NamedArgs{"org_id": cp.OrgID, "name": cp.Name}

	// Resolve cp.OrgID via a join rather than depending on the org_id foreign key, so an unknown org yields zero rows
	// instead of a distinct constraint-violation error.
	const sql = `
		INSERT INTO org.projects (org_id, name, is_control, created_at)
		SELECT o.id, @name, false, NOW()
		FROM org.organizations AS o
		WHERE o.id = @org_id
		RETURNING id, org_id, name, is_control, created_at, updated_at`

	return pgdb.TypedQuery[Project]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[Project],
		Expect: pgdb.ExpectOne,
	}
}
