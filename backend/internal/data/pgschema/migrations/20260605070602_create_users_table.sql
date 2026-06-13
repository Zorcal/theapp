-- migrate:up
CREATE TABLE useraccess.users (
    id SERIAL PRIMARY KEY
    , external_id UUID UNIQUE NOT NULL
    , email TEXT UNIQUE NOT NULL
    , name TEXT NOT NULL
    , created_at TIMESTAMPTZ NOT NULL
    , updated_at TIMESTAMPTZ
    , etag UUID UNIQUE NOT NULL
);

-- GIN trigram indexes support ILIKE prefix filtering on email and name.
-- The unique B-tree on email cannot serve case-insensitive scans.
CREATE INDEX users_email_trgm_idx ON useraccess.users USING GIN (email gin_trgm_ops);
CREATE INDEX users_name_trgm_idx ON useraccess.users USING GIN (name gin_trgm_ops);

-- B-tree indexes support ordering by created_at and updated_at.
-- email ordering uses the existing unique index; id ordering uses the primary key.
CREATE INDEX users_created_at_idx ON useraccess.users (created_at);
CREATE INDEX users_updated_at_idx ON useraccess.users (updated_at);


-- migrate:down
DROP INDEX useraccess.users_email_trgm_idx;
DROP INDEX useraccess.users_name_trgm_idx;
DROP INDEX useraccess.users_created_at_idx;
DROP INDEX useraccess.users_updated_at_idx;
DROP TABLE useraccess.users;
