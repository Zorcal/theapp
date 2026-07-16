-- migrate:up
CREATE FUNCTION org.protect_control_project() RETURNS TRIGGER AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.is_control THEN
            RAISE EXCEPTION 'control project "%" cannot be deleted', OLD.name;
        END IF;
        RETURN OLD;
    END IF;

    IF NEW.is_control IS DISTINCT FROM OLD.is_control THEN
        RAISE EXCEPTION 'is_control cannot be changed after creation (project "%")', OLD.name;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER protect_control_project
    BEFORE UPDATE OR DELETE ON org.projects
    FOR EACH ROW
    EXECUTE FUNCTION org.protect_control_project();


-- migrate:down
DROP TRIGGER protect_control_project ON org.projects;
DROP FUNCTION org.protect_control_project();
