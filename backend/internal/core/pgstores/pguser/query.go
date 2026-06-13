package pguser

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/zorcal/theapp/backend/internal/data/order"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

func userByEmailQuery(email string) pgdb.TypedQuery[User] {
	params := pgx.NamedArgs{"email": email}
	const sql = `
		SELECT id, external_id, email, name, created_at, updated_at, etag
		FROM useraccess.users
		WHERE email = @email`

	return pgdb.TypedQuery[User]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[User],
		Expect: pgdb.ExpectOne,
	}
}

func userByExternalIDQuery(id uuid.UUID) pgdb.TypedQuery[User] {
	params := pgx.NamedArgs{
		"external_id": id,
	}
	const sql = `
		SELECT id, external_id, email, name, created_at, updated_at, etag
		FROM useraccess.users
		WHERE external_id = @external_id`

	return pgdb.TypedQuery[User]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[User],
		Expect: pgdb.ExpectOne,
	}
}

func getOrCreateUserByEmailQuery(email string) pgdb.TypedQuery[User] {
	params := pgx.NamedArgs{"email": email}
	const sql = `
		INSERT INTO useraccess.users (external_id, email, name, created_at, etag)
		VALUES (gen_random_uuid(), @email, '', NOW(), gen_random_uuid())
		ON CONFLICT (email) DO UPDATE SET email = EXCLUDED.email
		RETURNING id, external_id, email, name, created_at, updated_at, etag`

	return pgdb.TypedQuery[User]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[User],
		Expect: pgdb.ExpectOne,
	}
}

func createUserQuery(cu CreateUser) pgdb.TypedQuery[User] {
	params := pgx.NamedArgs{
		"email": cu.Email,
		"name":  cu.Name,
	}
	const sql = `
		INSERT INTO useraccess.users (external_id, email, name, created_at, etag)
		VALUES (gen_random_uuid(), @email, @name, NOW(), gen_random_uuid())
		RETURNING id, external_id, email, name, created_at, updated_at, etag`

	return pgdb.TypedQuery[User]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[User],
		Expect: pgdb.ExpectOne,
	}
}

func updateUserQuery(uu UpdateUser) pgdb.TypedQuery[User] {
	params := pgx.NamedArgs{"external_id": uu.ExternalID}
	var setClauses []string

	if uu.Fields.Name {
		setClauses = append(setClauses, "name = @name")
		params["name"] = uu.Name
	}

	setClauses = append(setClauses, "updated_at = NOW()", "etag = gen_random_uuid()")

	sql := fmt.Sprintf(`
		UPDATE useraccess.users
		SET %s
		WHERE external_id = @external_id
		RETURNING id, external_id, email, name, created_at, updated_at, etag`,
		strings.Join(setClauses, ", "))

	return pgdb.TypedQuery[User]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[User],
		Expect: pgdb.ExpectOne,
	}
}

func usersQuery(filter Filter, orderBys []order.By[OrderByField], pageSize, pageOffset int) pgdb.TypedQuery[User] {
	params := pgx.NamedArgs{
		"page_size":   pageSize,
		"page_offset": pageOffset,
	}
	sql := fmt.Sprintf(`
		SELECT id, external_id, email, name, created_at, updated_at, etag
		FROM useraccess.users
		%s
		ORDER BY %s
		LIMIT @page_size OFFSET @page_offset`, whereClause(filter, params), orderByClause(orderBys))

	return pgdb.TypedQuery[User]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[User],
		Expect: pgdb.ExpectMany,
	}
}

func userCountQuery(filter Filter) pgdb.TypedQuery[int] {
	params := pgx.NamedArgs{}
	sql := fmt.Sprintf(`
		SELECT COUNT(*)
		FROM useraccess.users
		%s
	`, whereClause(filter, params))

	return pgdb.TypedQuery[int]{
		SQL:  sql,
		Args: params,
		Scan: func(row pgx.CollectableRow) (int, error) {
			var count int
			if err := row.Scan(&count); err != nil {
				return 0, fmt.Errorf("scan count: %w", err)
			}
			return count, nil
		},
		Expect: pgdb.ExpectOne,
	}
}

// whereClause builds an optional WHERE clause from f, adding any required
// named parameters to params as a side effect.
func whereClause(f Filter, params pgx.NamedArgs) string {
	var clauses []string
	if f.Email != "" {
		params["email_prefix"] = f.Email + "%"
		clauses = append(clauses, "email ILIKE @email_prefix")
	}
	if f.Name != "" {
		params["name_prefix"] = f.Name + "%"
		clauses = append(clauses, "name ILIKE @name_prefix")
	}
	if len(clauses) == 0 {
		return ""
	}
	return "WHERE " + strings.Join(clauses, " AND ")
}

func orderByClause(orderBys []order.By[OrderByField]) string {
	parts := make([]string, 0, len(orderBys)+1)
	for _, o := range orderBys {
		parts = append(parts, fmt.Sprintf("%s %s", o.Field, o.Direction))
	}
	parts = append(parts, "id ASC")
	return strings.Join(parts, ", ")
}
