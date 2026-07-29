-- migrate:up
CREATE TABLE rbac.permissions (
    id SERIAL PRIMARY KEY
    , name TEXT UNIQUE NOT NULL
);

-- Resolve API-facing permission names to the integer identities used by RBAC relations.
CREATE FUNCTION rbac.permission_ids(permission_names TEXT[])
RETURNS TABLE (id INTEGER, name TEXT)
LANGUAGE sql
STABLE
STRICT
SECURITY INVOKER
AS $$
    SELECT permission.id, permission.name
    FROM rbac.permissions AS permission
    JOIN (
        SELECT DISTINCT requested.name
        FROM unnest($1) AS requested(name)
    ) AS requested ON requested.name = permission.name
$$;


-- migrate:down
DROP FUNCTION rbac.permission_ids(TEXT[]);
DROP TABLE rbac.permissions;
