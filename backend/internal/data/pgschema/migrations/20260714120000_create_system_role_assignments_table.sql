-- migrate:up
CREATE TABLE rbac.system_role_assignments (
    user_id INTEGER NOT NULL REFERENCES useraccess.users (id)
    , role_id INTEGER NOT NULL REFERENCES rbac.system_roles (id)
    -- User-first ordering serves system permission resolution and listing or mutating one user's
    -- assignments. System roles are immutable seed data, so role-first lookup is not a normal
    -- runtime path.
    , PRIMARY KEY (user_id, role_id)
);

SELECT audit.enable('rbac.system_role_assignments');


-- migrate:down
DROP TABLE rbac.system_role_assignments;
