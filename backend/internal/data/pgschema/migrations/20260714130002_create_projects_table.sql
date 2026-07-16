-- migrate:up
CREATE TABLE org.projects (
    id SERIAL PRIMARY KEY
    , org_id INTEGER NOT NULL REFERENCES org.organizations (id)
    , name TEXT NOT NULL
    , is_control BOOLEAN NOT NULL
    , created_at TIMESTAMPTZ NOT NULL
    , updated_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX projects_org_id_lower_name_key ON org.projects (org_id, lower(name));
CREATE UNIQUE INDEX projects_org_id_control_key ON org.projects (org_id) WHERE is_control;


-- migrate:down
DROP TABLE org.projects;
