-- migrate:up
CREATE TABLE rbac.permissions (
    id SERIAL PRIMARY KEY
    , name TEXT UNIQUE NOT NULL
);

-- Resolve a complete API-facing permission-name set to the integer identities used by RBAC
-- relations. Duplicate names are removed. A NULL or empty input returns one row containing empty
-- arrays; an unknown name returns no row so callers can reject the complete mutation.
CREATE FUNCTION rbac.resolve_permissions(requested_permission_names TEXT[])
RETURNS TABLE (permission_ids INTEGER[], permission_names TEXT[])
LANGUAGE sql
STABLE
SECURITY INVOKER
AS $$
    WITH requested_permissions AS (
        SELECT DISTINCT requested.name
        FROM unnest(COALESCE($1, '{}')) AS requested(name)
    )
    SELECT
        COALESCE(
            array_agg(permission.id ORDER BY requested.name)
                FILTER (WHERE permission.id IS NOT NULL),
            '{}'
        ) AS permission_ids,
        COALESCE(
            array_agg(requested.name ORDER BY requested.name)
                FILTER (WHERE requested.name IS NOT NULL),
            '{}'
        ) AS permission_names
    FROM requested_permissions AS requested
    LEFT JOIN rbac.permissions AS permission ON permission.name = requested.name
    -- The LEFT JOIN retains unknown names with a NULL permission ID. Equal counts therefore mean
    -- every distinct requested name resolved; otherwise the function returns no row.
    HAVING COUNT(requested.name) = COUNT(permission.id)
$$;


-- migrate:down
DROP FUNCTION rbac.resolve_permissions(TEXT[]);
DROP TABLE rbac.permissions;
