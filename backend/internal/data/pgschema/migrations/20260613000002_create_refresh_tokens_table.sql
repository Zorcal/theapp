-- migrate:up
CREATE TABLE useraccess.refresh_tokens (
    id BIGSERIAL PRIMARY KEY
    , user_id BIGINT NOT NULL REFERENCES useraccess.users(id)
    , token_hash TEXT NOT NULL UNIQUE
    , expires_at TIMESTAMPTZ NOT NULL
    , revoked_at TIMESTAMPTZ
    , created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Covers lookups and bulk revocation by user_id.
CREATE INDEX refresh_tokens_user_id_idx ON useraccess.refresh_tokens (user_id);

-- migrate:down
DROP INDEX useraccess.refresh_tokens_user_id_idx;
DROP TABLE useraccess.refresh_tokens;
