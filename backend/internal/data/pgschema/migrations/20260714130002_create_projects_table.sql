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

-- Enforces case-insensitive project-name uniqueness within an organization and supports
-- ProjectByName's organization plus lower(name) lookup.
CREATE UNIQUE INDEX projects_org_id_lower_name_key ON org.projects (org_id, lower(name));

-- Enforces one control project per organization and supports resolving an organization's control
-- project by org_id without indexing ordinary projects.
CREATE UNIQUE INDEX projects_org_id_control_key ON org.projects (org_id) WHERE is_control;

-- Supplies the referenced key for project-assignment foreign keys that prove a project belongs to
-- the assignment organization. The primary key on id alone cannot be a composite FK target.
ALTER TABLE org.projects ADD CONSTRAINT projects_id_org_id_key UNIQUE (id, org_id);

-- Matches the stable name-then-organization ordering used by accessible-project pagination.
CREATE INDEX projects_name_org_id_idx ON org.projects (name, org_id);

-- Supports the case-insensitive name prefix predicate used by accessible-project list/count
-- queries; the ordering B-tree cannot serve ILIKE.
CREATE INDEX projects_name_trgm_idx ON org.projects USING GIN (name gin_trgm_ops);


-- migrate:down
DROP TABLE org.projects;
