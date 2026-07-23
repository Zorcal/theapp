# rbac

Core business logic for permissions and roles.

## System and custom role boundaries

System and custom roles live in separate tables, each with its own permission join table. A
system role is only assignable through `system_role_assignments`. Custom roles are assigned
through project- or org-scoped assignment tables and cannot be assigned at system scope.

## System-role assignment authorization

Assign and unassign identify the actor from `mdl.AuthSession`; a missing session is a programming
error. `BootstrapAssignSystemRole` is the explicit exception used to establish the first system
administrator.

In one transaction, the core:

1. Acquire transaction-level advisory locks for the actor and target.
2. Load the system role and the actor's system permissions.
3. Require the actor's permissions to be a superset of the role's permissions.
4. Insert or delete the target user's assignment.

Project- and org-scoped grants cannot authorize a system-wide change. Applying the same superset
rule to unassignment prevents an actor from stripping authority they do not hold.

Every assignment mutation takes the same per-user advisory lock, including bootstrap assignment.
Actor and target locks are acquired in UUID order to prevent deadlocks. The locks work even when a
user has no assignments and are released when the transaction ends.

Unassignments also take a system-role-management advisory lock before the user locks. If the role
carries `system-role:assign` or `system-role:unassign`, each permission must remain available
through another system-role assignment.

## Seed data

`internal/data/pgschema/seed.sql` inserts permissions and system roles. `AllPermissions()` in
`internal/core/mdl/permission.go` must stay in sync. The seed only inserts, so removing an entry
does not delete existing database rows.

### Removing a system role

Run the cleanup against the database after the code change deploys:

```sql
BEGIN;
DELETE FROM rbac.system_role_assignments WHERE role_id IN (SELECT id FROM rbac.system_roles WHERE name = '<removed system role>');
DELETE FROM rbac.system_role_permissions WHERE role_id IN (SELECT id FROM rbac.system_roles WHERE name = '<removed system role>');
DELETE FROM rbac.system_roles WHERE name = '<removed system role>';
COMMIT;
```

### Removing a permission

```sql
BEGIN;
DELETE FROM rbac.system_role_permissions
USING rbac.permissions p
WHERE system_role_permissions.permission_id = p.id AND p.name = '<removed permission>';

DELETE FROM rbac.custom_role_permissions
USING rbac.permissions p
WHERE custom_role_permissions.permission_id = p.id AND p.name = '<removed permission>';

DELETE FROM rbac.permissions WHERE name = '<removed permission>';
COMMIT;
```

Manual cleanup is acceptable while removals remain rare; otherwise it should become a `cmd/cli`
command.
