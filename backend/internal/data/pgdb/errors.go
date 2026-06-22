package pgdb

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// PostgreSQL error codes (SQLSTATE). See https://www.postgresql.org/docs/current/errcodes-appendix.html.
const (
	ErrCodeUniqueViolation = "23505"
)

// ErrAlreadyExists is returned when an insert violates a unique constraint.
var ErrAlreadyExists = errors.New("already exists")

// translatePgErr maps known PostgreSQL error codes to package-level sentinels.
// Unknown errors are returned as-is.
func translatePgErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == ErrCodeUniqueViolation {
		return ErrAlreadyExists
	}
	return err
}
