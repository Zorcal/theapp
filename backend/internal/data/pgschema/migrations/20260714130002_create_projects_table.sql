-- migrate:up
CREATE TABLE org.projects (
    id SERIAL PRIMARY KEY
    , org_id INTEGER NOT NULL REFERENCES org.organizations (id)
    , name TEXT NOT NULL
    , is_control BOOLEAN NOT NULL
    , created_at TIMESTAMPTZ NOT NULL
    , updated_at TIMESTAMPTZ
    , etag UUID UNIQUE NOT NULL
);
CREATE UNIQUE INDEX projects_org_id_lower_name_key ON org.projects (org_id, lower(name));
CREATE UNIQUE INDEX projects_org_id_control_key ON org.projects (org_id) WHERE is_control;
CREATE INDEX projects_name_org_id_idx ON org.projects (name, org_id);
CREATE INDEX projects_name_trgm_idx ON org.projects USING GIN (name gin_trgm_ops);


-- migrate:down
DROP TABLE org.projects;
