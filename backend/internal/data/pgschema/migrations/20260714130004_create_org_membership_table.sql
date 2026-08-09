-- migrate:up
CREATE TABLE org.org_membership (
    user_id INTEGER NOT NULL REFERENCES useraccess.users (id)
    , org_id INTEGER NOT NULL REFERENCES org.organizations (id)
    -- Organization-first ordering matches tenant-owned member listing and organization cleanup.
    -- Exact membership checks constrain both columns and remain efficient in either order; the
    -- reverse user-only direction has a separate index below.
    , PRIMARY KEY (org_id, user_id)
);

-- Supports finding every organization for a user, explicit user membership cleanup, and the
-- foreign-key check when deleting a user. The tenant-first primary key serves organization
-- listings and cleanup but cannot serve user_id-only operations.
CREATE INDEX org_membership_user_id_idx ON org.org_membership (user_id);

SELECT audit.enable('org.org_membership');


-- migrate:down
DROP TABLE org.org_membership;
