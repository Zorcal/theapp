-- migrate:up
CREATE TABLE rbac.roles (
    id SERIAL PRIMARY KEY
    , name TEXT UNIQUE NOT NULL
    , is_static BOOLEAN NOT NULL
    , created_at TIMESTAMPTZ NOT NULL
    , updated_at TIMESTAMPTZ
);

CREATE TABLE rbac.role_permissions (
    role_id INTEGER NOT NULL REFERENCES rbac.roles (id)
    , permission_id INTEGER NOT NULL REFERENCES rbac.permissions (id)
    , PRIMARY KEY (role_id, permission_id)
);


-- migrate:down
DROP TABLE rbac.role_permissions;
DROP TABLE rbac.roles;
