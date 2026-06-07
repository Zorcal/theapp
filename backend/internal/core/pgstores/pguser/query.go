package pguser

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/zorcal/theapp/backend/internal/data/order"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

func userByExternalIDQuery(id uuid.UUID) pgdb.TypedQuery[User] {
	params := pgx.NamedArgs{
		"external_id": id,
	}
	const sql = `
		SELECT external_id, email, name, created_at, updated_at, etag
		FROM useraccess.users
		WHERE external_id = @external_id`

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
		RETURNING external_id, email, name, created_at, updated_at, etag`

	return pgdb.TypedQuery[User]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[User],
		Expect: pgdb.ExpectOne,
	}
}

func usersQuery(orderBys []order.By[OrderByField], pageSize, pageOffset int) pgdb.TypedQuery[User] {
	params := pgx.NamedArgs{
		"page_size":   pageSize,
		"page_offset": pageOffset,
	}
	sql := fmt.Sprintf(`
		SELECT external_id, email, name, created_at, updated_at, etag
		FROM useraccess.users
		ORDER BY %s
		LIMIT @page_size OFFSET @page_offset`, orderByClause(orderBys))

	return pgdb.TypedQuery[User]{
		SQL:    sql,
		Args:   params,
		Scan:   pgx.RowToStructByName[User],
		Expect: pgdb.ExpectMany,
	}
}

func userCountQuery() pgdb.TypedQuery[int] {
	const sql = `SELECT COUNT(*) FROM useraccess.users`

	return pgdb.TypedQuery[int]{
		SQL: sql,
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

func orderByClause(orderBys []order.By[OrderByField]) string {
	parts := make([]string, 0, len(orderBys)+1)
	for _, o := range orderBys {
		parts = append(parts, fmt.Sprintf("%s %s", o.Field, o.Direction))
	}
	parts = append(parts, "id ASC")
	return strings.Join(parts, ", ")
}
