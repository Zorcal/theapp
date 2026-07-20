-- migrate:up

CREATE TABLE rbac.project_role_assignments (
    user_id INTEGER NOT NULL REFERENCES useraccess.users (id)
    , role_id INTEGER NOT NULL REFERENCES rbac.custom_roles (id)
    , project_id INTEGER NOT NULL REFERENCES org.projects (id)
    , PRIMARY KEY (user_id, role_id, project_id)
);

-- project_role_assignments is filtered by (user_id, project_id) when resolving a caller's
-- project-scoped permissions, a query run on every project-scoped request. role_id sits between
-- those two columns in the primary key above, so it can't serve this filter as a direct index
-- lookup; without this index, Postgres narrows to the user's rows via the primary key and then
-- checks project_id on every one of them, across all of that user's roles and projects.
-- This index keeps serving that lookup only as long as the query still filters on both user_id
-- and project_id as plain equality comparisons against these columns.
CREATE INDEX project_role_assignments_user_id_project_id_idx ON rbac.project_role_assignments (user_id, project_id);

CREATE TABLE rbac.org_role_assignments (
    user_id INTEGER NOT NULL REFERENCES useraccess.users (id)
    , role_id INTEGER NOT NULL REFERENCES rbac.custom_roles (id)
    , org_id INTEGER NOT NULL REFERENCES org.organizations (id)
    , PRIMARY KEY (user_id, role_id, org_id)
);

-- org_role_assignments is filtered by (user_id, org_id) when resolving a caller's org-scope
-- grants. role_id sits between those two columns in the primary key above, so it can't serve
-- this filter as a direct index lookup without this index.
CREATE INDEX org_role_assignments_user_id_org_id_idx ON rbac.org_role_assignments (user_id, org_id);


-- migrate:down
DROP TABLE rbac.org_role_assignments;
DROP TABLE rbac.project_role_assignments;
