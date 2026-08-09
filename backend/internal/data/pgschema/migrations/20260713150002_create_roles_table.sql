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
    , name TEXT NOT NULL
    , org_id INTEGER NOT NULL REFERENCES org.organizations (id)
    , managed_key TEXT
    , created_at TIMESTAMPTZ NOT NULL
    , updated_at TIMESTAMPTZ
    , etag UUID UNIQUE NOT NULL
    , CONSTRAINT custom_roles_name_check CHECK (name <> '' AND name = btrim(name))
    , CONSTRAINT custom_roles_managed_key_check
        CHECK (managed_key IS NULL OR managed_key = 'organization_admin')
);

-- Custom role names are unique within an organization, regardless of case.
CREATE UNIQUE INDEX custom_roles_org_id_lower_name_key ON rbac.custom_roles (org_id, lower(name));

-- Identifies the single application-managed organization role without coupling its identity to
-- display text. It supports managed-role reconciliation and prevents duplicate admin definitions.
CREATE UNIQUE INDEX custom_roles_org_id_managed_key_key
    ON rbac.custom_roles (org_id, managed_key)
    WHERE managed_key IS NOT NULL;

-- Supplies the referenced key for assignment foreign keys that prove a role belongs to the
-- assignment organization. The primary key on id alone cannot be a composite FK target.
ALTER TABLE rbac.custom_roles ADD CONSTRAINT custom_roles_id_org_id_key UNIQUE (id, org_id);

CREATE TABLE rbac.system_role_permissions (
    role_id INTEGER NOT NULL REFERENCES rbac.system_roles (id)
    , permission_id INTEGER NOT NULL REFERENCES rbac.permissions (id)
    -- Role-first ordering serves role expansion and seeded-role reconciliation, which read or
    -- replace the complete permission set for one role. Permissions are immutable seed data, so
    -- permission-first lookup is not a runtime path.
    , PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE rbac.custom_role_permissions (
    role_id INTEGER NOT NULL REFERENCES rbac.custom_roles (id)
    , permission_id INTEGER NOT NULL REFERENCES rbac.permissions (id)
    -- Role-first ordering serves custom-role reads, permission replacement/modification, and role
    -- deletion cleanup. Permissions are immutable seed data, so permission-first lookup is not a
    -- runtime path.
    , PRIMARY KEY (role_id, permission_id)
);

SELECT audit.enable('rbac.system_roles');
SELECT audit.enable('rbac.custom_roles');
SELECT audit.enable('rbac.system_role_permissions');
SELECT audit.enable('rbac.custom_role_permissions');


-- migrate:down
DROP TABLE rbac.custom_role_permissions;
DROP TABLE rbac.system_role_permissions;
DROP TABLE rbac.custom_roles;
DROP TABLE rbac.system_roles;
