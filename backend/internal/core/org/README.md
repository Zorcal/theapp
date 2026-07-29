# org

Core business logic for organizations and projects.

## Design decisions and known tradeoffs

**There is no "project membership" table.** A user's access to a project is derived from role assignments owned by `rbac`: a `project_role_assignments` row grants access to that one project directly; an `org_role_assignments` row grants access to every project under that org; and a system-role assignment grants access to every project only when the role carries `project:discover-all`. Project- and org-scoped custom-role grants are active only while the user has current membership in the owning organization.

**Org membership is the tenant boundary, not an access grant.** `org_membership` is required when assigning or unassigning either a project- or org-scoped custom role, and permission resolution ignores those assignments after membership ends. Membership alone grants no project permission; a role assignment is still required. Membership removal explicitly deletes every assignment first in the same transaction. Assignment rows without membership violate that invariant and require manual database repair.
