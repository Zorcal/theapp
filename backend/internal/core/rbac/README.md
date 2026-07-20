# rbac

Core business logic for permissions and roles.

## Design decisions and known tradeoffs

**Permissions and static roles are seed data.** `internal/data/pgschema/seed.sql` inserts every permission and static role. `AllPermissions` in `internal/core/mdl/permission.go` must list the same permissions as `seed.sql`.

**`seed.sql` only ever inserts.** Removing a permission or static role from `seed.sql` stops it from being re-inserted, but doesn't delete the existing row. Actually removing one is a manual step, run against the database after the code change deploys:

```sql
BEGIN;
DELETE FROM rbac.static_role_permissions WHERE role_id IN (SELECT id FROM rbac.static_roles WHERE name = '<removed static role>');
DELETE FROM rbac.static_roles WHERE name = '<removed static role>';
COMMIT;
```

Removing a permission:

```sql
BEGIN;
DELETE FROM rbac.static_role_permissions
USING rbac.permissions p
WHERE static_role_permissions.permission_id = p.id AND p.name = '<removed permission>';

DELETE FROM rbac.custom_role_permissions
USING rbac.permissions p
WHERE custom_role_permissions.permission_id = p.id AND p.name = '<removed permission>';

DELETE FROM rbac.permissions WHERE name = '<removed permission>';
COMMIT;
```

This manual step is an accepted tradeoff, not a gap to close: removing a permission or a static role is expected to be rare, so the cost of doing it by hand is low. If that stops being true, this cleanup can be wrapped into a `cmd/cli` command instead of staying a hand-run SQL snippet.

**Static and custom roles live in separate tables (`rbac.static_roles`, `rbac.custom_roles`), each with its own permission join table (`rbac.static_role_permissions`, `rbac.custom_role_permissions`).** `system_role_assignments.role_id` FKs to `rbac.static_roles`; `project_role_assignments.role_id`/`org_role_assignments.role_id` FK to `rbac.custom_roles`. "A static role is only ever assignable at system scope, and vice versa" is therefore a structural guarantee enforced by the foreign keys themselves, with no trigger involved.
