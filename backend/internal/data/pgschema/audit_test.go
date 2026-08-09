package pgschema_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
	"github.com/zorcal/theapp/backend/internal/data/pgtest"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestAuditEnable(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.NewWithoutSeed(t, ctx)

	actorID := uuid.New()
	traceID := trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	// Prepare the transaction attribution captured with each audit entry.

	ctx = mdl.ContextWithAuthSession(ctx, mdl.AuthSession{User: mdl.AuthUser{UserID: actorID}})
	ctx = trace.ContextWithSpanContext(ctx, trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: traceID,
		SpanID:  trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
	}))

	// Enable auditing on a composite-key table while excluding its secret-bearing column.

	if _, err := pool.Exec(ctx, `
		CREATE TABLE audit_test_records (
			org_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL,
			value TEXT NOT NULL,
			secret TEXT NOT NULL,
			PRIMARY KEY (org_id, user_id)
		);
		SELECT audit.enable('audit_test_records', ARRAY['secret']);`); err != nil {
		t.Fatalf("create audited table: %v", err)
	}

	// Exercise every audited mutation within one attributed transaction.

	if err := pgdb.RunTx(ctx, pool, func(ctx context.Context) error {
		if err := pgdb.RunExec(ctx, pool, `INSERT INTO audit_test_records (org_id, user_id, value, secret)
			VALUES (7, 11, 'before', 'hidden')`); err != nil {
			return err
		}
		if err := pgdb.RunExec(ctx, pool, `UPDATE audit_test_records SET value = 'after' WHERE org_id = 7 AND user_id = 11`); err != nil {
			return err
		}
		if err := pgdb.RunExec(ctx, pool, `DELETE FROM audit_test_records WHERE org_id = 7 AND user_id = 11`); err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatalf("RunTx() error = %v", err)
	}

	// Verify row identity, state transitions, redaction, and attribution together.

	type auditRow struct {
		TableSchema string
		TableName   string
		RowKey      map[string]any
		Action      string
		OldRow      map[string]any
		NewRow      map[string]any
		ActorID     uuid.UUID
		TraceID     string
	}
	rows, err := pool.Query(ctx, `SELECT table_schema, table_name, row_key, action,
		old_row, new_row, actor_id, trace_id
		FROM audit.audit_log
		ORDER BY id`)
	if err != nil {
		t.Fatalf("query audit log: %v", err)
	}
	defer rows.Close()

	var got []auditRow
	for rows.Next() {
		var row auditRow
		if err := rows.Scan(&row.TableSchema, &row.TableName, &row.RowKey, &row.Action,
			&row.OldRow, &row.NewRow, &row.ActorID, &row.TraceID); err != nil {
			t.Fatalf("scan audit log: %v", err)
		}
		got = append(got, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("query audit log: %v", err)
	}

	key := map[string]any{"org_id": float64(7), "user_id": float64(11)}
	before := map[string]any{"org_id": float64(7), "user_id": float64(11), "value": "before"}
	after := map[string]any{"org_id": float64(7), "user_id": float64(11), "value": "after"}
	want := []auditRow{
		{
			TableSchema: "public",
			TableName:   "audit_test_records",
			RowKey:      key,
			Action:      "INSERT",
			OldRow:      nil,
			NewRow:      before,
			ActorID:     actorID,
			TraceID:     traceID.String(),
		},
		{
			TableSchema: "public",
			TableName:   "audit_test_records",
			RowKey:      key,
			Action:      "UPDATE",
			OldRow:      before,
			NewRow:      after,
			ActorID:     actorID,
			TraceID:     traceID.String(),
		},
		{
			TableSchema: "public",
			TableName:   "audit_test_records",
			RowKey:      key,
			Action:      "DELETE",
			OldRow:      after,
			NewRow:      nil,
			ActorID:     actorID,
			TraceID:     traceID.String(),
		},
	}

	testingx.AssertDiff(t, got, want)
}

func TestAuditLogImmutable(t *testing.T) {
	ctx := context.Background()
	pool := pgtest.NewWithoutSeed(t, ctx)

	seedAuditTableQ := `
INSERT INTO audit.audit_log (table_schema, table_name, row_key, action, new_row)
	VALUES ('public', 'example', '{"id": 1}', 'INSERT', '{"id": 1}')`
	if _, err := pool.Exec(ctx, seedAuditTableQ); err != nil {
		t.Fatalf("seed audit log: %v", err)
	}

	tests := []struct {
		name  string
		query string
	}{
		{
			name:  "update",
			query: `UPDATE audit.audit_log SET table_name = 'changed'`,
		},
		{
			name:  "delete",
			query: `DELETE FROM audit.audit_log`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := pool.Exec(ctx, tt.query); err == nil {
				t.Errorf("Exec(%q) error = nil, want error", tt.query)
			}
		})
	}
}
