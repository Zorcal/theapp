-- migrate:up
CREATE TABLE org.org_membership (
    user_id INTEGER NOT NULL REFERENCES useraccess.users (id)
    , org_id INTEGER NOT NULL REFERENCES org.organizations (id)
    , PRIMARY KEY (user_id, org_id)
);


-- migrate:down
DROP TABLE org.org_membership;
