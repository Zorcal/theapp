-- migrate:up

CREATE TABLE rbac.system_roles (
    id SERIAL PRIMARY KEY
    , external_id UUID UNIQUE NOT NULL
    , name TEXT UNIQUE NOT NULL
    , created_at TIMESTAMPTZ NOT NULL
    , updated_at TIMESTAMPTZ
);

CREATE TABLE rbac.custom_roles (
    id SERIAL PRIMARY KEY
    , external_id UUID UNIQUE NOT NULL
    , name TEXT UNIQUE NOT NULL
    , org_id INTEGER NOT NULL REFERENCES org.organizations (id)
    , created_at TIMESTAMPTZ NOT NULL
    , updated_at TIMESTAMPTZ
    , etag UUID UNIQUE NOT NULL
);

CREATE TABLE rbac.system_role_permissions (
    role_id INTEGER NOT NULL REFERENCES rbac.system_roles (id)
    , permission_id INTEGER NOT NULL REFERENCES rbac.permissions (id)
    , PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE rbac.custom_role_permissions (
    role_id INTEGER NOT NULL REFERENCES rbac.custom_roles (id)
    , permission_id INTEGER NOT NULL REFERENCES rbac.permissions (id)
    , PRIMARY KEY (role_id, permission_id)
);


-- migrate:down
DROP TABLE rbac.custom_role_permissions;
DROP TABLE rbac.system_role_permissions;
DROP TABLE rbac.custom_roles;
DROP TABLE rbac.system_roles;
