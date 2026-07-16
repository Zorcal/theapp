package pgorg

import (
	"github.com/jackc/pgx/v5"

	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

func createOrganizationQuery(co CreateOrganization) pgdb.TypedQuery[Organization] {
	params := pgx.NamedArgs{"name": co.Name}
	const sql = `
		INSERT INTO org.organizations (name, created_at)
		VALUES (@name, NOW())
		RETURNING id, name, created_at, updated_at`

	return pgdb.TypedQuery[Organization]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[Organization],
		Expect: pgdb.ExpectOne,
	}
}

func createProjectQuery(cp CreateProject) pgdb.TypedQuery[Project] {
	params := pgx.NamedArgs{"org_id": cp.OrgID, "name": cp.Name}

	// Resolve cp.OrgID via a join rather than depending on the org_id foreign key, so an unknown org yields zero rows
	// instead of a distinct constraint-violation error.
	const sql = `
		INSERT INTO org.projects (org_id, name, created_at)
		SELECT o.id, @name, NOW()
		FROM org.organizations o
		WHERE o.id = @org_id
		RETURNING id, org_id, name, created_at, updated_at`

	return pgdb.TypedQuery[Project]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[Project],
		Expect: pgdb.ExpectOne,
	}
}
