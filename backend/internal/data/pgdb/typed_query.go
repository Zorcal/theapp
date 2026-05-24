package pgdb

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"go.opentelemetry.io/otel/attribute"

	"github.com/zorcal/theapp/backend/internal/telemetry"
)

var ErrTooManyRows = errors.New("too many rows")

// ResultExpectation describes how many rows the query writer expects back.
type (
	ResultExpectation uint8
)

const (
	// ExpectMany means the statement may yield any number of rows (including zero).
	ExpectMany ResultExpectation = iota
	// ExpectOne means the statement must yield exactly one row.
	ExpectOne
	// ExpectExec means the statement is an INSERT/UPDATE/DELETE without RETURNING.
	ExpectExec
	// ExpectExecOneRow means the statement is an INSERT/UPDATE/DELETE that must affect exactly one row.
	ExpectExecOneRow
)

// TypedQuery knows how to queue itself into a pgx.Batch without callers
// repeating boiler-plate.
type TypedQuery[T any] struct {
	SQL    string
	Args   any
	Scan   pgx.RowToFunc[T] // Optional for EXEC operations
	Expect ResultExpectation
}

// Queue adds the query into the batch expecting one row back. Populates dst
// with the result once the queued query finishes. Returns an error if q.Expect
// != ExpectOne.
func (q TypedQuery[T]) Queue(ctx context.Context, b *Batch, dst *T) error {
	_, span := telemetry.StartSpan(ctx, "pgdb.TypedQuery.Queue")
	defer span.End()

	span.SetAttributes(attribute.String("query", fmtQuery(q.SQL)))

	if q.Expect != ExpectOne {
		return fmt.Errorf("TypedQuery.Queue called with Expect=%d, but ExpectOne (%d) is required", q.Expect, ExpectOne)
	}

	b.b.Queue(q.SQL, flattenArgs(q.Args)...).Query(func(rows pgx.Rows) error {
		collectedRow, err := pgx.CollectOneRow(rows, q.Scan)
		if err != nil {
			return fmt.Errorf("collect one row: %w", err)
		}
		if rows.CommandTag().RowsAffected() > 1 {
			return ErrTooManyRows
		}
		*dst = collectedRow
		return nil
	})

	return nil
}

// QueueMany adds the query into the batch expecting zero or more rows.
// Populates dst with the result once the queued query finishes. Returns an
// error if q.Expect != ExpectMany.
func (q TypedQuery[T]) QueueMany(ctx context.Context, b *Batch, dst *[]T) error {
	_, span := telemetry.StartSpan(ctx, "pgdb.TypedQuery.QueueMany")
	defer span.End()

	span.SetAttributes(attribute.String("query", fmtQuery(q.SQL)))

	if q.Expect != ExpectMany {
		return fmt.Errorf("TypedQuery.QueueMany called with Expect=%d, but ExpectMany (%d) is required", q.Expect, ExpectMany)
	}

	b.b.Queue(q.SQL, flattenArgs(q.Args)...).Query(func(rows pgx.Rows) error {
		collectedRows, err := pgx.CollectRows(rows, q.Scan)
		if err != nil {
			return fmt.Errorf("collect rows: %w", err)
		}
		*dst = collectedRows
		return nil
	})

	return nil
}

func flattenArgs(args any) []any {
	switch v := args.(type) {
	case nil:
		return nil
	case []any:
		return v
	default:
		return []any{v}
	}
}

func fmtQuery(q string) string {
	r := strings.NewReplacer(
		"\t", "",
		"\n", " ",
		"( ", "(",
		") ", ")",
	)
	return strings.Trim(r.Replace(q), " ")
}
