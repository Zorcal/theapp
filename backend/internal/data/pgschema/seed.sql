-- Seed data applied on every startup — every statement in this file must be idempotent, since it runs against a
-- database that may already have this data.

BEGIN;

INSERT INTO rbac.permissions (name) VALUES
    ('user:read'),
    ('user:create'),
    ('user:update'),
    ('system-role:read'),
    ('system-role:assign'),
    ('system-role:unassign'),
    ('org:create'),
    ('project:create'),
    ('org:user-create'),
    ('project:discover-all'),
    ('custom-role:create'),
    ('custom-role:read'),
    ('custom-role:update'),
    ('custom-role:delete'),
    ('custom-role:assign-project'),
    ('custom-role:unassign-project'),
    ('custom-role:assign-org'),
    ('custom-role:unassign-org'),
    ('custom-role:read-project-assignments'),
    ('custom-role:read-org-assignments')
ON CONFLICT (name) DO NOTHING;

INSERT INTO rbac.system_roles (external_id, name, created_at)
SELECT gen_random_uuid(), 'superadmin', NOW()
WHERE NOT EXISTS (SELECT 1 FROM rbac.system_roles WHERE name = 'superadmin');

INSERT INTO rbac.system_role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM rbac.system_roles r
CROSS JOIN rbac.permissions p
WHERE r.name = 'superadmin'
ON CONFLICT DO NOTHING;

CREATE OR REPLACE FUNCTION pg_temp.sync_managed_role_permissions(
    target_managed_key TEXT,
    requested_permission_names TEXT[]
) RETURNS VOID
LANGUAGE plpgsql
AS $function$
DECLARE
    canonical_permission_ids INTEGER[];
BEGIN
    SELECT permission_ids
    INTO canonical_permission_ids
    FROM rbac.resolve_permissions(requested_permission_names);

    IF NOT FOUND THEN
        RAISE EXCEPTION 'unknown permission in managed role %', target_managed_key;
    END IF;

    WITH deleted AS (
        DELETE FROM rbac.custom_role_permissions AS role_permission
        WHERE role_permission.role_id IN (
                SELECT role.id
                FROM rbac.custom_roles AS role
                WHERE role.managed_key = target_managed_key
            )
            AND NOT role_permission.permission_id = ANY(canonical_permission_ids)
        RETURNING role_permission.role_id
    ),
    inserted AS (
        INSERT INTO rbac.custom_role_permissions (role_id, permission_id)
        SELECT role.id, permission.id
        FROM rbac.custom_roles AS role
        CROSS JOIN unnest(canonical_permission_ids) AS permission(id)
        WHERE role.managed_key = target_managed_key
        ON CONFLICT DO NOTHING
        RETURNING role_id
    ),
    changed AS (
        SELECT role_id FROM deleted
        UNION
        SELECT role_id FROM inserted
    )
    UPDATE rbac.custom_roles AS role
    SET updated_at = NOW(), etag = gen_random_uuid()
    FROM changed
    WHERE changed.role_id = role.id;
END;
$function$;

-- Keep every managed organization administrator role aligned with
-- mdl.OrganizationAdminPermissions. Update this canonical list whenever that function changes.
SELECT pg_temp.sync_managed_role_permissions(
    'organization_admin',
    ARRAY[
        ('project:create'),
        ('org:user-create'),
        ('custom-role:create'),
        ('custom-role:read'),
        ('custom-role:update'),
        ('custom-role:delete'),
        ('custom-role:assign-project'),
        ('custom-role:unassign-project'),
        ('custom-role:assign-org'),
        ('custom-role:unassign-org'),
        ('custom-role:read-project-assignments'),
        ('custom-role:read-org-assignments')
    ]
);

COMMIT;
