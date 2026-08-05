-- migrate:up

-- Resolve the permission identities granted to a user at system scope.
CREATE FUNCTION rbac.system_permission_ids(target_user_id INTEGER)
RETURNS TABLE (permission_id INTEGER)
LANGUAGE sql
STABLE
STRICT
SECURITY INVOKER
AS $$
    SELECT DISTINCT role_permission.permission_id
    FROM useraccess.users AS usr
    JOIN rbac.system_role_assignments AS assignment ON assignment.user_id = usr.id
    JOIN rbac.system_role_permissions AS role_permission ON role_permission.role_id = assignment.role_id
    WHERE usr.id = $1 AND usr.deleted_at IS NULL
$$;

-- Resolve effective organization permissions: organization grants plus system grants.
CREATE FUNCTION rbac.org_permission_ids(target_user_id INTEGER, target_org_id INTEGER)
RETURNS TABLE (permission_id INTEGER)
LANGUAGE sql
STABLE
STRICT
SECURITY INVOKER
AS $$
    SELECT role_permission.permission_id
    FROM useraccess.users AS usr
    JOIN rbac.org_role_assignments AS assignment ON assignment.user_id = usr.id
    JOIN rbac.custom_role_permissions AS role_permission ON role_permission.role_id = assignment.role_id
    WHERE usr.id = $1
        AND usr.deleted_at IS NULL
        AND assignment.org_id = $2

    UNION

    SELECT permission_id
    FROM rbac.system_permission_ids($1)
$$;

-- Resolve effective project permissions: project, organization, and system grants.
CREATE FUNCTION rbac.project_permission_ids(target_user_id INTEGER, target_project_id INTEGER)
RETURNS TABLE (permission_id INTEGER)
LANGUAGE sql
STABLE
STRICT
SECURITY INVOKER
AS $$
    SELECT role_permission.permission_id
    FROM useraccess.users AS usr
    JOIN rbac.project_role_assignments AS assignment ON assignment.user_id = usr.id
    JOIN rbac.custom_role_permissions AS role_permission ON role_permission.role_id = assignment.role_id
    WHERE usr.id = $1
        AND usr.deleted_at IS NULL
        AND assignment.project_id = $2

    UNION

    SELECT permission.permission_id
    FROM org.projects AS project
    CROSS JOIN LATERAL rbac.org_permission_ids($1, project.org_id) AS permission
    WHERE project.id = $2
$$;

-- Resolve every project reachable through direct, organization, or global-discovery grants.
CREATE FUNCTION rbac.accessible_project_ids(target_user_id INTEGER)
RETURNS TABLE (project_id INTEGER)
LANGUAGE sql
STABLE
STRICT
SECURITY INVOKER
AS $$
    SELECT assignment.project_id
    FROM useraccess.users AS usr
    JOIN rbac.project_role_assignments AS assignment ON assignment.user_id = usr.id
    WHERE usr.id = $1 AND usr.deleted_at IS NULL

    UNION

    SELECT project.id
    FROM useraccess.users AS usr
    JOIN rbac.org_role_assignments AS assignment ON assignment.user_id = usr.id
    JOIN org.projects AS project ON project.org_id = assignment.org_id
    WHERE usr.id = $1 AND usr.deleted_at IS NULL

    UNION

    SELECT project.id
    FROM useraccess.users AS usr
    CROSS JOIN org.projects AS project
    WHERE usr.id = $1
        AND usr.deleted_at IS NULL
        AND EXISTS (
            SELECT 1
            FROM rbac.system_permission_ids($1) AS granted
            JOIN rbac.permissions AS permission
                ON permission.id = granted.permission_id
                AND permission.name = 'project:discover-all'
        )
$$;


-- migrate:down
DROP FUNCTION rbac.accessible_project_ids(INTEGER);
DROP FUNCTION rbac.project_permission_ids(INTEGER, INTEGER);
DROP FUNCTION rbac.org_permission_ids(INTEGER, INTEGER);
DROP FUNCTION rbac.system_permission_ids(INTEGER);
