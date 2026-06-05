-- migrate:up
CREATE TABLE useraccess.users (
    id SERIAL PRIMARY KEY
    , external_id UUID UNIQUE NOT NULL
    , email TEXT UNIQUE NOT NULL
    , created_at TIMESTAMPTZ NOT NULL
    , updated_at TIMESTAMPTZ
    , etag UUID UNIQUE NOT NULL
);


-- migrate:down
drop table useraccess.users;
