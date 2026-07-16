# org

Core business logic for organizations and projects.

## Design decisions and known tradeoffs

**There is no "project membership" table.** A user's access to a project is entirely derived from role assignments, owned by `rbac`: a `project_role_assignments` row grants access to that one project directly; an `org_role_assignments` row grants access to every project under that org; a `system_role_assignments` row grants access to every project system-wide. Whether a user can act on a project is answered by resolving that three-way union, not by checking membership in any project-level table.

**Org membership gates org-scoped assignment, not project access.** `org_membership` records which users belong to which organization, and exists solely as a prerequisite for granting an `org_role_assignments` row — an org-scoped role can only be assigned to a user who is already a member of that org. It is not consulted when resolving whether a user can access a given project; that's answered entirely by the role-assignment union above. Confusing the two — treating org membership as a stand-in for project access — would wrongly grant access to every org member regardless of whether they hold any role at all.
