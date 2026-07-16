-- migrate:up
CREATE TABLE org.organizations (
    id SERIAL PRIMARY KEY
    , name TEXT UNIQUE NOT NULL
    , created_at TIMESTAMPTZ NOT NULL
    , updated_at TIMESTAMPTZ
);


-- migrate:down
DROP TABLE org.organizations;
