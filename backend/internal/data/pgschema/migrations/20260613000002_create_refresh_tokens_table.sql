-- migrate:up
CREATE TABLE useraccess.refresh_tokens (
    id         BIGSERIAL    PRIMARY KEY
    , user_id  BIGINT       NOT NULL REFERENCES useraccess.users(id)
    , token_hash TEXT       NOT NULL UNIQUE
    , expires_at TIMESTAMPTZ NOT NULL
    , revoked_at TIMESTAMPTZ
    , created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- migrate:down
DROP TABLE useraccess.refresh_tokens;
