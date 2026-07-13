-- migrate:up

-- This trigger has no exception for an orphaned static role (one removed from mdl.StaticRoles but
-- still present here) -- it blocks UPDATE/DELETE unconditionally whenever is_static is true. To
-- remove such a row by hand, wrap the disable/delete/enable sequence in one transaction --
-- ALTER TABLE ... TRIGGER is transactional DDL in Postgres, so a failure partway through (or a
-- dropped connection) rolls the whole thing back instead of leaving the trigger disabled:
--   BEGIN;
--   ALTER TABLE rbac.roles DISABLE TRIGGER prevent_static_role_mutation;
--   DELETE FROM rbac.roles WHERE name = '<orphaned role name>';
--   ALTER TABLE rbac.roles ENABLE TRIGGER prevent_static_role_mutation;
--   COMMIT;
-- TRUNCATE also bypasses this trigger, but only ever use it in a dev/test database you're fully
-- re-seeding anyway -- it empties every row in the table, not just the orphaned one.
CREATE FUNCTION rbac.prevent_static_role_mutation() RETURNS TRIGGER AS $$
BEGIN
    IF OLD.is_static THEN
        RAISE EXCEPTION 'static role "%" cannot be updated or deleted', OLD.name;
    END IF;
    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER prevent_static_role_mutation
    BEFORE UPDATE OR DELETE ON rbac.roles
    FOR EACH ROW
    EXECUTE FUNCTION rbac.prevent_static_role_mutation();


-- migrate:down
DROP TRIGGER prevent_static_role_mutation ON rbac.roles;
DROP FUNCTION rbac.prevent_static_role_mutation();
