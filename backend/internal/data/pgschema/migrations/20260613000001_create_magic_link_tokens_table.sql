-- migrate:up
CREATE TABLE useraccess.magic_link_tokens (
    id         BIGSERIAL    PRIMARY KEY
    , user_id  BIGINT       NOT NULL REFERENCES useraccess.users(id)
    , token_hash TEXT       NOT NULL UNIQUE
    , expires_at TIMESTAMPTZ NOT NULL
    , used_at  TIMESTAMPTZ
);

-- migrate:down
DROP TABLE useraccess.magic_link_tokens;
