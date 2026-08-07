-- migrate:up
CREATE TABLE useraccess.users (
    id SERIAL PRIMARY KEY
    , external_id UUID UNIQUE NOT NULL
    , email TEXT UNIQUE NOT NULL
    , name TEXT NOT NULL
    , created_at TIMESTAMPTZ NOT NULL
    , updated_at TIMESTAMPTZ
    , email_verified_at TIMESTAMPTZ
    , etag UUID UNIQUE NOT NULL
);

-- Supports the case-insensitive email prefix predicate used by paginated user list/count queries;
-- the unique B-tree on email cannot serve ILIKE.
CREATE INDEX users_email_trgm_idx ON useraccess.users USING GIN (email gin_trgm_ops);

-- Supports the case-insensitive name prefix predicate used by paginated user list/count queries.
CREATE INDEX users_name_trgm_idx ON useraccess.users USING GIN (name gin_trgm_ops);

-- Supports paginated user lists ordered by creation time.
CREATE INDEX users_created_at_idx ON useraccess.users (created_at);

-- Supports paginated user lists ordered by last update time, including PostgreSQL's default
-- placement of NULL values.
CREATE INDEX users_updated_at_idx ON useraccess.users (updated_at);


-- migrate:down
DROP INDEX useraccess.users_email_trgm_idx;
DROP INDEX useraccess.users_name_trgm_idx;
DROP INDEX useraccess.users_created_at_idx;
DROP INDEX useraccess.users_updated_at_idx;
DROP TABLE useraccess.users;
