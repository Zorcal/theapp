-- migrate:up
CREATE SCHEMA audit;

CREATE TABLE audit.audit_log (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    table_schema TEXT NOT NULL,
    table_name TEXT NOT NULL,
    row_key JSONB NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('INSERT', 'UPDATE', 'DELETE')),
    old_row JSONB,
    new_row JSONB,
    actor_id UUID,
    trace_id TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (
        (action = 'INSERT' AND old_row IS NULL AND new_row IS NOT NULL)
        OR (action = 'UPDATE' AND old_row IS NOT NULL AND new_row IS NOT NULL)
        OR (action = 'DELETE' AND old_row IS NOT NULL AND new_row IS NULL)
    )
);

CREATE FUNCTION audit.capture_row() RETURNS TRIGGER AS $$
DECLARE
    excluded_columns TEXT[] := TG_ARGV[0]::TEXT[];
    key_columns TEXT[] := TG_ARGV[1]::TEXT[];
    raw_row JSONB;
    old_row JSONB;
    new_row JSONB;
    row_key JSONB;
BEGIN
    IF TG_OP IN ('UPDATE', 'DELETE') THEN
        old_row := to_jsonb(OLD) - excluded_columns;
    END IF;
    IF TG_OP IN ('INSERT', 'UPDATE') THEN
        new_row := to_jsonb(NEW) - excluded_columns;
    END IF;

    raw_row := CASE WHEN TG_OP = 'DELETE' THEN to_jsonb(OLD) ELSE to_jsonb(NEW) END;
    SELECT jsonb_object_agg(key_column, raw_row -> key_column)
    INTO row_key
    FROM unnest(key_columns) AS key_column;

    INSERT INTO audit.audit_log (
        table_schema,
        table_name,
        row_key,
        action,
        old_row,
        new_row,
        actor_id,
        trace_id
    ) VALUES (
        TG_TABLE_SCHEMA,
        TG_TABLE_NAME,
        row_key,
        TG_OP,
        old_row,
        new_row,
        NULLIF(current_setting('app.user_id', true), '')::UUID,
        NULLIF(current_setting('app.trace_id', true), '')
    );

    IF TG_OP = 'DELETE' THEN
        RETURN OLD;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, audit;

CREATE FUNCTION audit.enable(target_table REGCLASS, excluded_columns TEXT[] DEFAULT '{}') RETURNS VOID AS $$
DECLARE
    key_columns TEXT[];
BEGIN
    SELECT array_agg(attribute.attname ORDER BY key_column.ordinality)
    INTO key_columns
    FROM pg_index AS idx
    CROSS JOIN LATERAL unnest(idx.indkey) WITH ORDINALITY AS key_column(attnum, ordinality)
    JOIN pg_attribute AS attribute
        ON attribute.attrelid = idx.indrelid
        AND attribute.attnum = key_column.attnum
    WHERE idx.indrelid = target_table
        AND idx.indisprimary;

    IF key_columns IS NULL THEN
        RAISE EXCEPTION 'audited table % must have a primary key', target_table;
    END IF;

    EXECUTE format(
        'CREATE TRIGGER audit_row AFTER INSERT OR UPDATE OR DELETE ON %s '
        'FOR EACH ROW EXECUTE FUNCTION audit.capture_row(%L, %L)',
        target_table,
        excluded_columns::TEXT,
        key_columns::TEXT
    );
END;
$$ LANGUAGE plpgsql;

CREATE FUNCTION audit.reject_log_mutation() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'audit log rows are immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER reject_log_mutation
    BEFORE UPDATE OR DELETE ON audit.audit_log
    FOR EACH ROW
    EXECUTE FUNCTION audit.reject_log_mutation();


-- migrate:down
DROP TRIGGER reject_log_mutation ON audit.audit_log;
DROP FUNCTION audit.reject_log_mutation();
DROP FUNCTION audit.enable(REGCLASS, TEXT[]);
DROP FUNCTION audit.capture_row();
DROP TABLE audit.audit_log;
DROP SCHEMA audit;
