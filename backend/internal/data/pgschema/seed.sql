-- Seed data applied on every startup — every statement in this file must be idempotent, since it runs against a
-- database that may already have this data.

BEGIN;

INSERT INTO rbac.permissions (name) VALUES
    ('user:read'),
    ('user:create'),
    ('user:update')
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

COMMIT;
