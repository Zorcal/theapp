-- migrate:up

CREATE TABLE rbac.project_role_assignments (
    user_id INTEGER NOT NULL
    , project_id INTEGER NOT NULL
    , role_id INTEGER NOT NULL
    , org_id INTEGER NOT NULL
    -- User-first ordering serves per-request permission resolution and accessible-project
    -- discovery; project_id second serves user/project role listing and mutation. role_id last
    -- completes assignment identity. org_id is derived and protected by the composite foreign
    -- keys, so including it would not distinguish another valid assignment.
    , PRIMARY KEY (user_id, project_id, role_id)
    , FOREIGN KEY (org_id, user_id) REFERENCES org.org_membership (org_id, user_id)
    , FOREIGN KEY (project_id, org_id) REFERENCES org.projects (id, org_id)
    , FOREIGN KEY (role_id, org_id) REFERENCES rbac.custom_roles (id, org_id)
);

-- Serves project-first assignment/user listings and explicit project deletion cleanup. Permission
-- resolution uses the primary key's (user_id, project_id) prefix instead.
CREATE INDEX project_role_assignments_project_id_user_id_idx ON rbac.project_role_assignments (project_id, user_id);

-- Serves explicit membership cleanup and the membership foreign-key check when a (user, org)
-- membership is deleted. Project permission resolution uses the primary key instead.
CREATE INDEX project_role_assignments_user_id_org_id_idx ON rbac.project_role_assignments (user_id, org_id);

-- Serves explicit custom-role deletion cleanup and the role foreign-key check; no primary-key
-- prefix starts with role_id.
CREATE INDEX project_role_assignments_role_id_idx ON rbac.project_role_assignments (role_id);

CREATE TABLE rbac.org_role_assignments (
    user_id INTEGER NOT NULL
    , org_id INTEGER NOT NULL
    , role_id INTEGER NOT NULL
    -- User-first ordering serves per-request organization permission resolution and accessible-
    -- project discovery; org_id second serves user/organization role listing and mutation.
    -- role_id last completes assignment identity.
    , PRIMARY KEY (user_id, org_id, role_id)
    , FOREIGN KEY (org_id, user_id) REFERENCES org.org_membership (org_id, user_id)
    , FOREIGN KEY (role_id, org_id) REFERENCES rbac.custom_roles (id, org_id)
);

-- Serves organization-first assignment/user listings and explicit organization deletion cleanup.
-- Permission resolution uses the primary key's (user_id, org_id) prefix instead.
CREATE INDEX org_role_assignments_org_id_user_id_idx ON rbac.org_role_assignments (org_id, user_id);

-- Serves explicit custom-role deletion cleanup and the role foreign-key check; no primary-key
-- prefix starts with role_id.
CREATE INDEX org_role_assignments_role_id_idx ON rbac.org_role_assignments (role_id);


-- migrate:down
DROP TABLE rbac.org_role_assignments;
DROP TABLE rbac.project_role_assignments;
