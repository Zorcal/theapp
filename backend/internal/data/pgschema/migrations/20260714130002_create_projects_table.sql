-- migrate:up
CREATE TABLE org.projects (
    id SERIAL PRIMARY KEY
    , org_id INTEGER NOT NULL REFERENCES org.organizations (id)
    , name TEXT NOT NULL
    , created_at TIMESTAMPTZ NOT NULL
    , updated_at TIMESTAMPTZ
    , UNIQUE (org_id, name)
);


-- migrate:down
DROP TABLE org.projects;
