-- Seed data applied on every startup — every statement in this file must be idempotent, since it runs against a
-- database that may already have this data.

BEGIN;

INSERT INTO rbac.permissions (name) VALUES
    ('user:read'),
    ('user:create'),
    ('user:update')
ON CONFLICT (name) DO NOTHING;

INSERT INTO rbac.roles (name, is_static, created_at) VALUES
    ('superadmin', TRUE, NOW())
ON CONFLICT (name) DO NOTHING;

INSERT INTO rbac.role_permissions (role_id, permission_id)
SELECT r.id, p.id
FROM rbac.roles r
CROSS JOIN rbac.permissions p
WHERE r.name = 'superadmin'
ON CONFLICT DO NOTHING;

COMMIT;
