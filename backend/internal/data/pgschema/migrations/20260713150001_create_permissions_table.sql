-- migrate:up
CREATE TABLE rbac.permissions (
    id SERIAL PRIMARY KEY
    , name TEXT UNIQUE NOT NULL
);


-- migrate:down
DROP TABLE rbac.permissions;
