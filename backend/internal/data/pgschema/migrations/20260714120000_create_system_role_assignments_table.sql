-- migrate:up
CREATE TABLE rbac.system_role_assignments (
    user_id INTEGER NOT NULL REFERENCES useraccess.users (id)
    , role_id INTEGER NOT NULL REFERENCES rbac.roles (id)
    , PRIMARY KEY (user_id, role_id)
);


-- migrate:down
DROP TABLE rbac.system_role_assignments;
