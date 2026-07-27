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

## Custom-role authorization

Creating a custom role, changing its permissions, and assigning or unassigning it all enforce the
permission-superset rule. The actor must hold every permission being granted or revoked. For a
replacement or incremental permission edit, the check applies only to permissions that actually
change; permissions already present and no-op additions or removals require no additional
authority.

The actor's permissions are resolved fresh in the same transaction as the mutation:

1. Resolve the target project or organization and the actor's permissions in that scope.
2. Load the organization-owned custom role when the operation targets an existing role.
3. Require the actor's permissions to contain every permission being added, removed, assigned, or
   unassigned.
4. Apply the role or assignment mutation.

Project-scoped assignment changes combine project-, organization-, and system-scope permissions.
Organization-scoped assignment changes and role permission edits combine only organization- and
system-scope permissions. A project role can therefore justify a change within its project but
cannot be used to alter organization-wide access or an organization-owned role definition.

Custom roles are owned by an organization rather than a project and may be assigned in any project
within that organization or directly at organization scope. Creating a role or changing its
permissions therefore requires organization- or system-scoped authority: allowing a permission
held in one project to define a reusable role would let that project-level authority be carried
into other projects or promoted to organization scope. Organization-scoped assignment changes use
the same rule because their effect already spans every project in the organization. Project-scoped
assignment changes are narrower and may be authorized by permissions held in that project.

Missing users, projects, organizations, memberships, roles, or assignments are reported as not
found without exposing cross-organization resources. Attempting to act on permissions the actor
does not hold is reported as permission denied.

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
