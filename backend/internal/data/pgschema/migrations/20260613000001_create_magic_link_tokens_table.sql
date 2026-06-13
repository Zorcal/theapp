-- migrate:up
CREATE TABLE useraccess.magic_link_tokens (
    id BIGSERIAL PRIMARY KEY
    , user_id BIGINT NOT NULL REFERENCES useraccess.users(id)
    , token_hash TEXT NOT NULL UNIQUE
    , expires_at TIMESTAMPTZ NOT NULL
    , used_at TIMESTAMPTZ
    , created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Covers lookups by user_id and the latest-token query (i.e. ORDER BY created_at DESC LIMIT 1).
CREATE INDEX magic_link_tokens_user_id_created_at_idx ON useraccess.magic_link_tokens (user_id, created_at DESC);

-- migrate:down
DROP INDEX useraccess.magic_link_tokens_user_id_created_at_idx;
DROP TABLE useraccess.magic_link_tokens;
