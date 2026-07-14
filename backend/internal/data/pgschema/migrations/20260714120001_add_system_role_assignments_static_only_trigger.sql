-- migrate:up
CREATE FUNCTION rbac.prevent_custom_role_system_assignment() RETURNS TRIGGER AS $$
DECLARE
    target_is_static BOOLEAN;
BEGIN
    SELECT is_static INTO target_is_static FROM rbac.roles WHERE id = NEW.role_id;
    IF NOT target_is_static THEN
        RAISE EXCEPTION 'only a static role can be assigned at system scope';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER prevent_custom_role_system_assignment
    BEFORE INSERT ON rbac.system_role_assignments
    FOR EACH ROW
    EXECUTE FUNCTION rbac.prevent_custom_role_system_assignment();


-- migrate:down
DROP TRIGGER prevent_custom_role_system_assignment ON rbac.system_role_assignments;
DROP FUNCTION rbac.prevent_custom_role_system_assignment();
