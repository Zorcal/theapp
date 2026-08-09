# Permissions and roles

## Concepts

A **permission** (capability) is a single named action on a resource — for example `user:read`, `user:create`, or `user:update`. Every protected endpoint declares the permission(s) required to call it. Without this system, all authenticated users have implicit access to every endpoint, which is incorrect as the surface grows.

Permissions are granular: reading, creating, updating, and deleting a resource are distinct permissions. Reading a single resource by ID and listing resources normally share a single read permission; they're only split into separate permissions for a resource where that distinction is specifically needed. An endpoint may require more than one permission at once, when the action behind it always needs all of them regardless of what's in the request. This doesn't extend to a sensitive field on an otherwise ordinarily-updatable resource — that gets its own RPC rather than an extra required permission on the general update endpoint (see "Enforcement" below for why).

A **role** is a named set of permissions. Roles are assigned to users within a project (see below); a user may hold multiple roles in the same project. The resolved permission set is the distinct union of all permissions across all of the user's roles in that project.

This union is strictly additive: there is no explicit deny. A role can only ever grant permissions, never subtract from what another of the user's roles already grants, so there's no way to express "everything this role normally gets, except this one permission" without restructuring the roles involved instead of layering an exception on top. This is a deliberate choice, not an oversight — it mirrors GCP IAM's additive-only model rather than AWS IAM's deny-overrides-allow, and keeps resolution a plain union with no precedence rules to reason about. The cost is that a case like "org-wide read access except for one specific project" can't be expressed directly; it has to be modeled as project-scoped grants for every project but the excluded one instead of an org-wide grant plus an exception.

An **organization** (tenant) can contain multiple **projects** — for example a real project alongside test and demo projects. Role assignment is scoped per project rather than per organization, so a role can be tried out in a test project before being granted in the real one without affecting real users. Users belong to organizations and projects rather than to the system globally, and can belong to more than one of each. In practice, belonging to multiple organizations on a single account is expected mainly for developer and internal accounts; most other users who need access to more than one organization do so with a separate account per organization instead of one account spanning tenants.

A project belongs to exactly one organization, via a `projects.org_id` column (`UNIQUE (org_id, name)` also enforces that a project's name is unique within its organization). Organization is mostly a hidden concept for clients: most users only ever interact with projects directly and never need to know which organization a project belongs to. Organization membership only surfaces for the smaller set of users who create or administer an organization itself (see "Creating organizations and projects" below).

Organization membership and org-scoped role assignment are separate concepts, and the former gates the latter: an `org_role_assignments` row can only be created for a user who is already a member of that organization. Without this, an org-scoped role could be granted to a user who has no other relationship to the organization at all, which defeats the point of tracking membership as a distinct concept in the first place — role assignment would become the only place membership was ever checked, for one scope only, rather than membership meaning "belongs to this organization" everywhere it's referenced. Composite foreign keys make membership and organization ownership database invariants for project- and org-scoped assignments, while `RoleService` still performs the same checks to return useful domain errors.

## AuthSession

At request time, after validating the access token, the request is resolved into an `mdl.AuthSession` struct that is threaded through the call stack. `AuthSession.Project` is nil for a request with no project context (see below) — in that case `User.Permissions` is resolved from system-scope role assignments only. Otherwise, the project contains its ID, organization ID and name, control-project marker, and whether the caller belongs to the organization. `User.Permissions` is resolved from project-, org-, and system-scope role assignments for that project.

A specialized auth-store query resolves the caller and all project metadata together. Permissions remain a separate query because permission resolution has a distinct purpose and is reused elsewhere. A project ID that doesn't match any real project is rejected outright — the same `mdl.ErrNotFound` used for an unresolvable caller identity. A caller with a real project ID but no relevant role assignment is a different case and not an error: the session resolves normally with an empty (or system-scope-only) permission set, which permission-checking code rejects on its own.

`AuthUser.Permissions` is resolved from the database on each request rather than cached in the token itself — otherwise a revoked role would stay effective until the token expired, which for a normal access token TTL is far too long a window.

If per-request DB resolution ever becomes a bottleneck, the fix is a short-lived server-side cache keyed by `(user, project)` when `ProjectID` is set or `(user)` otherwise, on the order of 15-30 seconds, invalidated on role assignment/removal where practical and falling back to the TTL as a backstop otherwise — not caching in the token, which reintroduces the same staleness problem this section exists to avoid. Such a cache would only ever back the ordinary per-request `AuthUser.Permissions` resolution above; the privilege-escalation check (see "Privilege escalation" below) always resolves the actor's permissions fresh, since it's a comparatively rare, high-stakes operation where a 15-30 second staleness window is a meaningfully different tradeoff than it is for read-heavy request checks.

```
AuthSession {
	User    AuthUser
	Project *AuthProject  // nil for a request with no project context
}

AuthProject {
	ID        int
	OrgID     int
	OrgName   string
	IsControl bool
	IsOrgMember bool
}

AuthUser {
    UserID      uuid.UUID
    Permissions []Permission  // distinct, resolved from all assigned roles
}
```

`Permission` is a typed string constant; the full set is defined in `mdl/`.

The per-request resolution above assumes a unary RPC, where every call is a fresh check. A server-streaming RPC that stays open resolves `AuthUser.Permissions` once at stream-open; a role change made mid-stream doesn't take effect until the stream ends and reconnects, which is a meaningfully longer staleness window than the per-request model this section otherwise guarantees. There are no long-lived streaming RPCs in the system today, so this doesn't need solving now, but if one is ever added it needs its own explicit re-check inside the stream loop rather than silently inheriting the unary assumption — that RPC's implementation should carry a code comment pointing back to this paragraph, since someone implementing it by reading the interceptor code alone would have no reason to suspect the assumption doesn't hold.

The project ID is sent as request metadata rather than as a field on each RPC's request message, since every project-scoped endpoint needs it and putting it on each message would mean threading it through every proto by hand. Project IDs are unique globally, not just within an organization, so the metadata value alone is enough to resolve the project without also needing an organization ID. The interceptor validates that this metadata is present by default, rejecting the call otherwise; endpoints that legitimately have no project context resolve a `nil` project ID instead, via an explicit exceptions list, rather than opting out ad hoc. Note that most endpoints that might look project-less at first, such as creating an organization, still resolve a project — org creation requires `org:create` in `theapp/control`, so it still sends that project's ID as metadata; the exceptions list is for the rarer case of an endpoint with no meaningful project at all. `UserService` is the current example: it's a system-wide user directory rather than a project- or org-scoped resource, so every one of its RPCs is on the exceptions list, and its permissions (`user:read`, `user:create`, `user:update`) can only be granted through a system-scope role assignment.

## Standard roles

`superadmin` holds every permission in the system. This isn't a code-level bypass of the permission check — `superadmin` is a role like any other, with a real `system_role_permissions` row for every permission, so its grants are visible as ordinary data rather than a special case in the interceptor. Beyond it, a set of other system roles might be hardcoded in the codebase and seeded at startup, scoped to a narrower slice of permissions than superadmin — for example a `useradmin` role limited to user-management permissions, or a `rolesadmin` role limited to role-management permissions (the exact set is still open, these are illustrative). Without at least one assigned role a user can authenticate but cannot call any protected endpoint.

System roles are hardcoded and seeded at startup, and are structurally distinct from the
organization-owned roles exposed through `RoleService` (see "Custom role management" below).
`SystemRoleService` can list and assign/unassign the seeded definitions, but it has no create,
update, or delete operation. `RoleService` operates only on organization-owned roles. Most are
user-defined custom roles; the organization-admin role is a managed definition maintained by the
application. The split is reflected in the API, core/store operations, permission namespaces, and
database tables, so a system role can never be edited or deleted through the custom-role API and
drift from what the codebase expects it to be.

System and custom roles live in separate tables, `rbac.system_roles` and `rbac.custom_roles`. A write path aimed at custom roles has no `rbac.system_roles` row to reach, so a system role's identity can't be mutated by a write aimed at a custom role, or vice versa — the separation is structural, enforced by the schema itself rather than a trigger.

Each role table has its own permission join table, `system_role_permissions` and `custom_role_permissions`, rather than a column on `system_roles`/`custom_roles` itself. Table separation says nothing about these join tables — a write path that bypassed the role service could still insert or delete a system role's permission rows without touching `system_roles` itself. This is a deliberate gap rather than an oversight: see "Database backstops vs. application checks" under "Enforcement" below for why they don't get the same treatment.

A custom role is itself owned by exactly one organization — `custom_roles` carries a `NOT NULL
org_id`; system roles have no owner and are visible system-wide. Every `RoleService` operation that
touches an organization-owned role checks that the role's `org_id` matches the organization the
target assignment resolves to (the organization itself, for an org-scoped assignment; the
project's organization, for a project-scoped one). Without this, nothing would stop one
organization's admin from assigning another organization's role ID to a project in their own
organization. Role listing for the assignment UI is filtered by the caller's organization for the
same reason: a role belonging to another organization should never be discoverable, not merely
unassignable.

The managed organization-admin definition lives in `rbac.custom_roles` because its permissions
resolve through organization assignments exactly like any other organization-owned role. A
nullable stable identifier such as `managed_key = 'organization_admin'` distinguishes it from
user-defined roles without turning its display name into an identifier. A partial unique index
allows at most one such role per organization. In the API this maps to
`ROLE_KIND_ORGANIZATION_ADMIN`; ordinary roles map to `ROLE_KIND_CUSTOM`. The database key is not
part of the public contract.

A managed organization role behaves like a system role only in the lifecycle of its semantic
definition: the application owns its managed identity, permission set, existence, and assignment
scope. Ordinary core and API operations cannot delete it or modify its permissions. Its display
name remains organization-editable because reconciliation identifies it by `managed_key`, not by
presentation text. It is not a system-scoped role. It remains owned by one organization, may
contain only custom-role-assignable permissions, is assigned through `org_role_assignments`, and
never enters `system_role_assignments`. Authorized administrators may rename it and assign or
unassign it at organization scope.

System roles, like permissions (see "Permission seeding" below), are seed data rather than something reconciled at runtime: adding one is a change to the seed data, applied on the next startup; removing one — or shrinking which permissions it's granted — needs a manual cleanup step against the database, since the seed step only ever inserts. Until that cleanup runs, a removed system role (or a permission removed from one) stays exactly as real as it was before the code change, including in `AuthUser.Permissions` resolution — there's no code-side filtering trying to treat it as already gone. See `internal/core/rbac/README.md` for the manual cleanup procedure.

## Role assignment scope

Role assignment normally targets one project, matching the project-scoped model above. `superadmin` is the exception this model has to account for: assigning it per project individually wouldn't scale, since every new project would need its own `superadmin` assignment for existing internal staff. Rather than making `superadmin` a global permission exception, role assignment has three scopes: `project`, `org`, and `system`.

Org-wide scope is a real need beyond just accommodating `superadmin`: an organization's own admin/owner should have access to every project under their organization, including ones created after the role was assigned, without being re-assigned each time a new project is created. System-wide scope, by contrast, is only ever used by the system roles (`superadmin` and friends) — a customer-defined custom role has no reason to reach across organizations it doesn't own.

Permissions also declare the narrowest assignment scope at which they are meaningful. A
project-scoped permission may be carried by either a project or organization assignment; an
organization-scoped permission may only be carried by an organization assignment; and a
system-scoped permission may only be carried by a system role. A role's minimum assignment scope
is the broadest scope required by any permission it contains. A mixed role containing even one
organization-scoped permission is therefore organization-only. Project assignment rejects such a
role with a failed-precondition error rather than accepting an assignment whose organization
permissions could never authorize an organization-scoped operation. The managed
organization-admin role is always organization-only.

The same invariant applies when a role's permissions change. Adding an organization-scoped
permission to a role that still has project assignments is rejected with a failed-precondition
error; the administrator must explicitly remove those project assignments and retry rather than
having a permission edit silently revoke access. Project assignment and permission mutation lock
the same custom-role row in their transactions. This serializes the compatibility check with the
write, so a concurrent project assignment cannot race with a change that makes the role
organization-only. Consequently, every supported operation preserves the invariant that an
organization-only role has no project assignment rows.

Each scope is its own table rather than one table with a scope column: `project_role_assignments` (user, role, project ID), `org_role_assignments` (user, role, org ID), and `system_role_assignments` (user, role — no project or org at all). Forcing all three into a single table would mean a project ID column that means "the target" for project scope but "administered under, target derived by lookup" for org scope, and nothing at all for system scope — three different meanings behind one column. Separate tables let each row only carry the columns its scope actually needs, so granting `superadmin` is just a row in `system_role_assignments`, with no project to invent for it.

Organization membership is the tenant boundary for every custom-role assignment. A user must be a current member of the role's owning organization before receiving or giving up either a project- or org-scoped role. Membership alone grants no access; it only makes custom-role assignments eligible to contribute permissions. Both assignment tables carry the target organization ID. Composite foreign keys tie each assignment to the user's membership and the custom role's organization, and additionally tie a project assignment to the project's organization. Membership removal explicitly deletes every project- and org-scoped assignment before deleting the membership, in the same transaction, so those scopes stop contributing permissions because their assignments no longer exist. The foreign keys use their default `NO ACTION` behavior, so an incomplete cleanup is rejected rather than leaving permission resolution to compensate for dangling assignments.

Resolving a user's permissions for a request's `ProjectID` is then a union of three independent lookups: `project_role_assignments` filtered by that project ID directly, `org_role_assignments` filtered by the org the project belongs to (one lookup to resolve project → org, then filter), and `system_role_assignments` filtered by user alone, unconditionally, since it needs no relation to the request's project at all. These unions live in small `STABLE`, `SECURITY INVOKER` SQL functions shared by authentication, privilege checks, accessible-project discovery, and later database authorization work. The functions accept internal integer IDs and return sets of IDs, keeping the authorization rules in one place without materializing derived access.

The rows this union returns are trusted as-is, with no code-side filtering layered on top: permissions and system roles are seed data (see "Permission seeding" below), so a removed one is only ever actually gone from the database once its manual cleanup step has run, not merely removed from the code-defined list. Turning the union into `AuthUser.Permissions` is therefore a plain flatten-and-deduplicate of what the three tables return.

## Enforcement

Permission checks happen in the gRPC interceptor layer, not inside business logic. Each RPC declares its required permission(s) via a registry map. The interceptor resolves `AuthUser` from the token, then rejects the call with `codes.PermissionDenied` if the required permissions are absent. When an endpoint requires more than one permission, all of them must be present — required permissions are ANDed, never ORed.

An RPC with no entry in the registry map is denied, not allowed through unchecked. An RPC that intentionally requires no permission (anyone authenticated may call it) still needs an explicit entry mapping it to an empty permission list — that's the only way to distinguish "deliberately open" from "someone forgot to register this." A unit test enumerates every registered gRPC method and asserts each one has a registry entry, so a new RPC added without one fails the build instead of silently defaulting to either extreme.

An empty permission list is only safe for an endpoint that either doesn't return project-scoped resource data at all, or reflects only the caller's own identity or already-resolved permissions — the "exposing auth data to clients" endpoint below is the existing example of the latter. `ProjectID` metadata (see above) is supplied by the caller and never itself validated against membership; an empty-permission endpoint skips the one check that would normally catch a caller sending a `ProjectID` for a project they have no relationship to, and Postgres RLS (see below) enforces "rows for the project you claim," not "you're actually in that project," so RLS doesn't catch this either. A future endpoint that returns project-scoped data and reaches for an empty permission list because "the caller is authenticated, that's enough" would read as correct in isolation while actually exposing every project's data to every authenticated user regardless of membership — the empty-list registry entry proves the omission was intentional, not that whatever the endpoint returns is safe to hand back for an arbitrary `ProjectID`.

Because the registry maps a permission list to an RPC method and nothing else, the interceptor never inspects the request body — it has no way to tell whether a given call to, say, `UpdateUser` happens to touch a particular field or not. A sensitive field on an otherwise ordinarily-updatable resource is therefore never folded into that resource's general update RPC behind an extra required permission; it gets its own RPC instead (`UpdateUserEmail` alongside `UpdateUser`, for example), gated by its own registry entry. Requiring the extra permission on the general update RPC would apply it to the entire call unconditionally, since the interceptor has no field mask to condition on — a role holding only the base update permission would lose the ability to update the resource at all, the opposite of the reason the permission was split out in the first place. Splitting the RPC keeps the interceptor content-blind for every endpoint, rather than carving out a one-off exception that inspects the request body to decide which permissions apply.

The interceptor only checks whether the caller holds a permission for the request's `ProjectID` — it never looks at which project the resource identified in the request body actually belongs to. Without a further rule, a caller with `user:update` in their own project could update a user ID belonging to a different project, since that ID alone doesn't carry a project check. Every core-layer query or mutation for a project-scoped resource must therefore filter by the resolved `ProjectID` in addition to the resource's own primary key (`WHERE id = $1 AND project_id = $2`, not `WHERE id = $1`), so a resource ID from another project simply doesn't match rather than being editable across a tenant boundary.

This filter is application-level, which means a single store method that forgets it is a full cross-tenant data breach, not a degraded feature — so every ordinary project-scoped resource table also requires a database-level backstop. `internal/data/pgdb` already sets `app.project_id` as a transaction-local setting alongside `app.user_id` and `app.trace_id` (see "Auditing" below). When the first such resource is introduced, its migration must add a Postgres row-level security policy restricting visible rows to `project_id = current_setting('app.project_id')::int`, plus `FORCE ROW LEVEL SECURITY`. The application-layer `WHERE` clause remains the primary mechanism, since it fails with a clean not-found rather than an opaque empty result; RLS is the safety net that turns a missed filter into "returns nothing" instead of "returns another tenant's row." The current schema has project and assignment infrastructure but no ordinary project-owned resource table, so there is no resource policy to install yet.

Postgres RLS is a no-op for the role that owns the table (and for a superuser role), regardless of policy — so this backstop only exists if the app actually connects as a role RLS applies to. Migrations run as an owner role that can alter the schema; the runtime app connects as a separate, non-owning role that only has DML grants and is subject to every policy. The first project-resource migration must therefore include a real-Postgres test that connects as the runtime app role, sets `app.project_id` to one project, and proves a row from another project is invisible.

Assignment tables deliberately do not use RLS. Resolving "what can this user do" reads assignments across projects, while organization administrators normally assign, unassign, and inspect roles belonging to other users. A policy keyed on either `app.project_id` or the actor's `app.user_id` would therefore block ordinary authorization and administration paths. These tables instead rely on composite foreign keys for tenant consistency and core-layer permission and privilege-escalation checks for authorization. RLS would only become appropriate after a deliberate redesign that routes every legitimate cross-user operation through narrowly scoped privileged database functions; adding exceptions for only some paths would provide an incomplete and misleading boundary.

### Database backstops vs. application checks

The database-level backstops in this document aren't applied uniformly, and that's deliberate rather than an oversight. Tenant isolation (RLS plus `FORCE ROW LEVEL SECURITY` as project-scoped resource tables are introduced), a custom role never landing at system scope (`system_role_assignments.role_id` only ever references `rbac.system_roles`, see "Role assignment scope" above), protection of bootstrap-critical identities once mutation APIs can threaten them (see "Bootstrapping" below), and `audit_log` immutability all get one, because each protects an actual root of trust in the system: a single missed check there is a full cross-tenant breach, a corrupted bootstrap foundation, or a compromised audit trail — not a degraded feature.

Tenant consistency on project- and org-scoped assignments is also enforced structurally. Composite foreign keys require the target user to belong to the assignment organization, the custom role to be owned by it, and a project target to belong to it. These constraints complement rather than replace application checks: the services still validate requests so expected mistakes receive useful domain errors, while the database rejects invalid state from any write path.

Most other invariants in this document — a system role's own identity and its `system_role_permissions` membership, and the privilege-escalation superset check on granting or revoking a role (see "Privilege escalation" below) — are enforced only in application code. A trigger or `SECURITY DEFINER` function is more logic to keep in sync with the Go code it duplicates, written in a language and toolchain the team touches far less often than the core service code; that maintenance cost is only worth paying where the blast radius of a missed check is severe enough to justify it. Everywhere else, `SystemRoleService` and `RoleService` being the sole application write paths for their respective tables, backed by code review and the project's existing conventions, is treated as sufficient — the same trust already placed in every other core-layer store method that isn't wrapped in its own trigger.

## Permission seeding

Permissions are hardcoded, not created through any endpoint, but are stored in the database so roles can reference them by ID. They're seeded — along with `superadmin`'s role row and its grants — by `internal/data/pgschema/seed.sql`. Unlike a migration, it isn't tracked as applied-once: it's run against the pool wherever one is set up (server startup, test setup) and every statement in it is written to be a no-op when the row it inserts already exists (`ON CONFLICT ... DO NOTHING`), so running it again is always safe. Without this, a newly added permission or a newly added system-role grant would be unusable until someone updated the database by hand.

This is deliberately one-directional: `seed.sql` only ever inserts. Removing a permission or a system-role grant from `seed.sql` stops it from being re-inserted on the next startup, but does nothing to a row already in the database — an automatic delete tied to a code change, re-run on every process start, would be a destructive write racing against whichever version of the code happens to be running on each instance during a rolling deploy. Actually removing a permission, or a system role, or shrinking a system role's granted permissions, is therefore a manual step run against the database directly after the code change has deployed — see `internal/core/rbac/README.md` for the procedure.

This means a permission or system-role grant removed from `seed.sql` stays exactly as real as before until that manual step runs — nothing filters it out elsewhere. This is an accepted tradeoff given how rarely a permission or system role is removed outright, against how dangerous an automatic destructive step tied to every startup would be. If removals ever stop being rare enough for a hand-run SQL snippet, this cleanup can move into a `cmd/cli` command instead.

## Exposing authorization context to clients

The auth service exposes authorization context that includes the authenticated caller's ID and email plus
effective project-, organization-, and system-scoped permission sets — not full profile data. The
project set combines project-, organization-, and system-scope assignments for the selected
project; the organization set combines organization- and system-scope assignments for that
project's organization; the system set contains only system-scope assignments. This tells a
frontend where a permission is usable without exposing which role or assignment supplied it.
Without these separate sets, a project-only grant could make an organization-scoped control appear
available even though the backend correctly rejects it. This lives on the auth service rather than
the user service because it is authorization context for the current session, not user profile
data, and it only ever concerns the caller rather than an arbitrary user ID.

## System role management

`SystemRoleService` is the API for the seeded, ownerless roles in `system_roles`. It lists system roles with their permissions, assigns or unassigns one for a user, and lists a user's current system-role assignments. It cannot create, edit, or delete system roles; those definitions remain seed data.

Permissions are part of the `SystemRole` resource rather than exposed through a per-role list endpoint. The administrative UI needs a role and its grants together when displaying or selecting it; embedding them avoids a follow-up request for every row. System roles and permission sets are small seed data, so independently paginating one role's permissions would add API and client complexity without a practical payload benefit. A user's assignment list also returns `SystemRole` resources, so it has the same complete shape.

Every system-role RPC is anchored on the `theapp` organization's control project and requires the
caller to be a member of `theapp`. System-scoped authority therefore does not bypass the internal
staff boundary represented by that membership. Its permission namespace is distinct from
custom-role management: reads require `system-role:read`, assignment requires
`system-role:assign`, and unassignment requires `system-role:unassign`. A future custom-role
permission must not satisfy one of these checks merely because both APIs deal with roles.

The project anchor determines whether the caller may reach the endpoint, but it does not justify a system-wide grant by itself. Before assigning or unassigning, the core resolves the actor's permissions again from system scope only and applies the superset rule described below in the same transaction as the write. Transaction-level advisory locks serialize assignment changes for the actor and target, so the actor's authority cannot change between the check and the write. Project- and org-scoped permissions—including ones resolved through `theapp/control`—cannot be laundered into authority over `system_role_assignments`.

Unassigning also preserves a global recovery path: after any system-role revoke, at least one active user must still hold every registered permission through the effective union of that user's remaining system-scoped assignments. The permissions may be distributed across multiple system roles assigned to that user; project- and org-scoped assignments never contribute. This guarantees that one system administrator can repair any lower scope through the ordinary APIs. The bootstrap CLI is the explicit exception used to establish the first fully privileged user, not the normal recovery mechanism for an unsafe revoke.

## Custom role management

Organization-owned roles are exposed through a separate `RoleService`: creating, editing, and
deleting user-defined role definitions, and assigning or unassigning compatible roles at project
or organization scope. Managed definitions are returned by its read and assignment operations but
rejected by every definition-mutation operation. This is kept separate from the user service
because role assignment is a many-to-many mutation; folding it into the user service's
field-mask-based update would mean replacing a user's entire role list on every write, silently
dropping concurrent assignments made by other admins. It is also separate from
`SystemRoleService`: organization-owned roles never enter `system_role_assignments`, and system
roles never enter project/org assignment tables.

Custom-role endpoints use their own `custom-role:*` permission namespace. Definition operations use `custom-role:create`, `custom-role:read`, `custom-role:update`, and `custom-role:delete`. Project assignment operations use `custom-role:assign-project`, `custom-role:unassign-project`, and `custom-role:read-project-assignments`; organization assignment operations use `custom-role:assign-org`, `custom-role:unassign-org`, and `custom-role:read-org-assignments`. The two paginated assignment-list endpoints require a target user ID and return complete custom-role resources for exactly one scope. This separation lets organization administrators delegate project-level role management and visibility without also delegating organization-wide assignment authority or visibility. These permissions are introduced with `RoleService`, not predeclared by the system-role phase.

Organization assignment authorization resolves the actor's permission from organization- and system-scope assignments only. A project-scoped role carrying `custom-role:assign-org`, `custom-role:unassign-org`, or `custom-role:read-org-assignments` does not authorize an organization-wide mutation or assignment listing merely because the request is anchored on that project.

Permissions for system-wide services, including `user:*` and `system-role:*`, cannot be included in a custom role. They may only reach a user through a system-role assignment. The model keeps an explicit set of these permissions, and every custom-role mutation path rejects them before writing role permissions. The database permission table remains a shared registry, so this scope restriction is an application invariant rather than a foreign-key constraint.

A custom `Role` likewise embeds its permissions. `UpdateRole` can replace the complete permission set, while `ModifyRolePermissions` atomically adds and removes selected permissions without requiring the caller to reproduce set logic. Adding an existing permission or removing an absent one is a no-op; including the same permission in both lists is invalid. This deliberately differs from AIP-144's separate, non-idempotent add/remove methods to make multi-permission changes atomic. ETags are returned but not yet enforced on mutations; their future use is documented in `schemas/README.md`.

A separate `schemas/permission.proto` owns the static `Permission` enum used by every API message
that exposes permissions, including system roles, custom roles, permission modifications, the
permission catalog, and auth data. It also defines `AssignmentScope`. Generated frontend clients
can therefore use enum values for authorization-dependent rendering and scope compatibility
instead of redefining backend strings and classification rules. The database and core model retain
readable permission names such as `user:read`; exhaustive conversion tests require every protobuf
enum value and scope classification to map to the model and back without drift.

Permission enum values are public API identifiers rather than secrets. Authorization remains correct when a caller knows every identifier, and permission names stay at the level of product capabilities rather than exposing sensitive implementation details. Once shipped, renaming or removing an enum value follows the same compatibility rules as any other public schema change. Avoiding discovery of system-only identifiers would require separate public and internal schemas; filtering a response cannot hide values already present in generated clients and schema descriptors, and that separation is not warranted here.

The same schema defines a read-only `PermissionService` that exposes
`PermissionDescriptor` resources containing the permission and its minimum assignment scope.
It omits system-only permissions because system-role definitions cannot be created or edited
through the API; callers only assign or unassign complete seeded system roles, whose responses
already embed their permissions. `Role` similarly exposes `RoleKind` and its derived minimum
assignment scope. Keeping the catalog outside `RoleService` avoids presenting permission
definitions as state owned by an organization or by one role.

The administrative UI groups permission choices by scope and marks organization-only choices.
Selecting one updates the role preview to explain that the role can only be assigned to the
organization. Project assignment pickers show incompatible roles disabled with that explanation
rather than silently hiding them. Managed organization-admin roles carry a managed badge, allow
display-name editing, and disable permission editing and deletion. These are usability rules; the
backend remains authoritative for both mutability and assignment-scope validation.

The customer-facing Swagger UI should eventually publish only customer-supported services. Internal surfaces such as `UserService` and `SystemRoleService` remain in the shared backend and internal API documentation but are omitted from the customer Swagger bundle to avoid suggesting that customers should integrate with them. Both bundles are generated from the same protobuf definitions; service visibility is documentation and SDK hygiene, not an authorization boundary, so every omitted RPC remains fully permission-gated.

Role assignment requests carry project metadata to establish the active project or organization
authorization context. The assignment target remains explicit: either that project or its
organization. The exposed auth data for a client likewise resolves against the project the client
is currently working in, not just the user.

Deleting a user-defined custom role leaves behind its rows in `project_role_assignments` and
`org_role_assignments` unless they're removed in the same transaction. `RoleService` deletes these
assignment rows explicitly as part of role deletion, the same explicit-cleanup approach used for
organization/project deletion below, rather than relying on a cascading foreign key. Managed role
definitions cannot enter this operation.

## Privilege escalation

Without a check, a user could assign a role, or create/edit a custom role, to grant permissions they don't themselves hold — the classic privilege escalation path in a role system. The rule: an actor can only grant a role, or add a permission to a custom role, if their own resolved permission set (in the scope the grant applies to) is already a superset of what's being granted. The same applies to creating or editing a custom role: you can't put a permission into a role unless you hold that permission yourself.

Revoking a role, or removing a permission from a custom role, is subject to the same superset rule: an actor can only revoke a role, or remove a permission from a custom role, if their resolved permission set (in the scope of the assignment being revoked) is a superset of what's being removed. This isn't about escalation — a revocation only ever reduces the target's access — but about a different failure mode it shares with escalation: without this check, an actor holding a narrow permission like `custom-role:unassign-project` in a project could strip a role from a user with far broader access than the actor themselves will ever hold, including an org-wide admin's assignment in that same project. That's privilege sabotage rather than privilege escalation, but it's the same underlying problem — acting on privilege you don't hold — just in the opposite direction, so it gets the same rule rather than an exemption.

This resolution and the grant or revoke it authorizes happen inside the same database transaction. Without that, the actor's own role could be revoked by someone else in the gap between the check and the write, letting a permission set that's no longer current still authorize a grant or revoke that lands after it stopped being true.

The actor's permission set for this check is resolved specifically for the grant's target project or org, not read off `AuthSession.User.Permissions`. The session's permissions are scoped to the request's `ProjectID` metadata, which is the caller's current working project and not necessarily the project or org the grant targets — a role-assignment call can target a different project than the one the caller is sitting in. Reusing the session's permission set here would check the wrong scope, either rejecting a legitimate org-wide admin acting on a project they're not currently "in", or missing an escalation that only shows up once resolved against the right target.

For a project-scoped grant, this resolution mirrors the same three-way union used to resolve permissions at request time (see "Role assignment scope" above): `project_role_assignments` for that project, `org_role_assignments` for the project's org, and `system_role_assignments`. The org leg matters here, not just at request time — an org-wide admin must be able to grant project roles in any project under their org without first being individually assigned a role in that specific project.

For an org-scoped grant, this resolution must only union the actor's `org_role_assignments` (for that org) and `system_role_assignments` — never any `project_role_assignments`, even one scoped to a project inside the target org. Otherwise an actor holding a permission in a single project could grant an org-wide role carrying that permission, which then applies to every project under the org, including ones the actor has no access to at all. Scope must never be laundered upward: a permission held at project scope only ever justifies a grant at project scope for that same project.

For a system-scoped grant, this resolution must only consider the actor's own `system_role_assignments` — never `org_role_assignments` or `project_role_assignments`, no matter how broad. Otherwise an org-wide admin holding `system-role:assign` in their own organization could grant a system-scoped role — `superadmin` among them — which then applies across every organization in the system, not just their own. This is the same laundering rule as the project-to-org case above, one level further up: a permission held at org scope only ever justifies a grant at that org's scope, never at system scope. `SystemRoleService` owns this system-only check; `RoleService` never writes a system-scoped assignment.

This superset check, for both granting and revoking, is enforced by the service that owns the assignment scope — `SystemRoleService` for system assignments and `RoleService` for project/org assignments. There is no database-level backstop for it. See "Database backstops vs. application checks" under "Enforcement" above for why: the check is comparatively complex, scope-dependent logic, and duplicating it in a trigger would mean keeping another implementation of permission resolution correct in SQL alongside the Go version, for an invariant the application services being the sole write paths are judged sufficient to hold in practice.

Assigning a role at `system` scope is restricted to the system roles (`superadmin` and friends); a custom role can never be targeted by a `system_role_assignments` insert. `SystemRoleService` only resolves names from `system_roles`, while `RoleService` exposes no system-scope target — otherwise a custom role carrying even a modest permission could be assigned system-wide and apply across every organization, a blast radius the role's own permission set was never designed for.

The application-level checks alone leave a gap for any write path that bypasses `SystemRoleService` — and `system_role_assignments` is the single highest-blast-radius table in the schema, since a row there escapes every project and org boundary at once. It gets a database-level backstop structurally rather than through a trigger: `system_role_assignments.role_id` is a foreign key into `rbac.system_roles` specifically, so a custom role's ID simply isn't a row that column can reference, even through a write path that skips the service entirely.

System-role revocation is also checked against a recovery rule that is independent of the acting user's authorization: it must leave at least one active user with every registered permission at system scope. The check considers each remaining user's effective union across system-role assignments rather than requiring one particular role to contain the complete registry. It runs in the same transaction as the revoke and is serialized with other system-role changes so concurrent revokes cannot each remove part of the final fully privileged user's access.

The recovery invariant is enforced by the system-role service rather than a database trigger. A complete trigger-based version would need to coordinate assignment deletion, permission-registry changes, system-role grant changes, and the bootstrap transition from no administrator to the first one. System-role definitions and their permissions are seed-managed rather than mutable through the API, so serializing the ordinary unassignment path provides the required runtime guarantee without duplicating seed and bootstrap rules in database code.

Project and organization scopes do not have separate last-manager guards. A managed
organization-admin assignment may be removed, and a user-defined role may be unassigned, deleted,
or stripped of management permissions, even when that removes the last locally scoped
administrator. The managed organization-admin definition itself remains available, and the
retained fully privileged system administrator can restore an assignment through the ordinary
API. This deliberately favors one global recovery invariant over substantially more complex checks
across every custom-role assignment and affected scope. If relying on system-administrator
intervention becomes an operational problem, local continuity guards can be added from observed
requirements rather than anticipated ones.

## Discovering accessible projects

Since organization is mostly a hidden concept for clients, discovery is project-first: `ProjectService` lists projects reached through direct project assignments, organization assignments, or a system assignment whose role carries `project:discover-all`. An arbitrary system role does not expose every tenant's project metadata; global discovery is an explicit system-only capability. The result is what a frontend uses to build a project switcher and to pick the `ProjectID` it sends as metadata on every subsequent call. This listing does not require project metadata because selecting a project is its purpose. Organization-level information is only included for the smaller set of users who administer an organization directly, rather than being a concept every client has to deal with.

For most callers this list is small — a handful of projects at most — but a system-scoped assignment carrying `project:discover-all` resolves to every project in the system. This endpoint is therefore paginated like any other list endpoint returning a potentially unbounded set, rather than assuming the common case (a handful of projects) is the only case.

## Creating organizations and projects

Creating an organization is not self-service — without a permission check, any authenticated user could create one. Rather than making org creation a global exception to project-scoped permissions, the `org:create` permission is itself project-scoped, to a dedicated `control` project under the `theapp` organization. This keeps enforcement uniform — every permission check is a project-scoped lookup, with no separate "global permission" code path.

Holding `org:create` isn't enough on its own, though: the caller must also actually be a member of the `theapp` organization. This isn't a special case invented for `theapp` — it's the same membership prerequisite that gates every org-scoped role assignment (see "Concepts" above), since `org:create` is granted via a role assigned at `theapp`'s org scope. A permission check alone confirms what a role can do, not that the caller belongs to the organization the role's project lives under; here, as everywhere else org-scoped assignment is used, both checks apply. Only internal staff are members of `theapp`; regular end users are never added to it.

When an organization is created, it is seeded with a default project — its name supplied explicitly alongside the organization's, rather than always matching it — so a newly created organization is immediately usable without a separate "create your first project" step.

Organization creation also makes the creator an organization member, creates an
organization-admin custom role, and assigns that role to the creator at organization scope in the
same transaction. Org scope, not project scope, matters here: an organization administrator needs
authority across every project in the organization, including projects created after the initial
assignment, without being assigned again for each project.

The organization-admin role can CRUD the organization's custom-role definitions and manage role
assignments at both organization and project scope. It therefore holds `custom-role:create`,
`custom-role:read`, `custom-role:update`, `custom-role:delete`,
`custom-role:read-org-assignments`, `custom-role:assign-org`,
`custom-role:unassign-org`, `custom-role:read-project-assignments`,
`custom-role:assign-project`, and `custom-role:unassign-project`. It also holds the ordinary
`project:create` permission. Project creation anchors on the organization's default project the
same way organization creation anchors on `theapp/control`, but `project:create` has organization
minimum scope because creating a sibling project changes organization-level state. The metadata
project resolves the organization, and only an organization- or system-scoped assignment can grant
the permission; a project-scoped custom-role assignment cannot.

The canonical organization-admin permission set is used for new organization creation and seed
reconciliation. After permission seeding, an idempotent synchronization adds missing permissions
and removes noncanonical permissions from every `organization_admin` managed role. Capabilities
introduced after launch therefore reach existing organizations, while removed capabilities no
longer remain granted by managed roles.

## Managing users within an organization

`UserService` itself has no organization or project concept — it's a system-wide directory (see "AuthSession" above). Managing which users belong to which organization is a separate, org-scoped concern, exposed through `OrgService` rather than `UserService`.

Creating a user and adding them to an organization is a single endpoint: given an email, it creates
the system user if none exists, then adds that user to the calling organization. If the user already
exists in the system, the endpoint only adds the organization membership; it never creates a
duplicate user. The endpoint is anchored on the organization's control project, requiring the
`x-project-id` metadata to be that project's ID (`org.projects.is_control = true`), the same way
organization creation itself anchors on `theapp/control` (see "Creating organizations and
projects" above). Every organization-level administrative action resolves through its control
project rather than an arbitrary member project.

The organization-admin role receives the organization-scoped permissions for creating, viewing,
updating, and removing organization users as those operations are introduced. Adding a new
organization-user capability must update both the permissions used for newly created
organization-admin roles and the rollout path for existing organizations; it must not leave older
organizations with a permanently weaker administrator role.

Listing users scoped to an organization is a separate endpoint from `UserService.ListUsers`, for
the same reason: it is an org-level concern, not part of the system user directory. It is likewise
anchored on the organization's control project via `x-project-id` and requires `org:user-read`.
The optional project filter answers which organization members have effective access to that
project. It uses the canonical system-, organization-, and project-assignment union rather than
`org_membership`, which only establishes that a user belongs to the organization. A missing project
or a project from another organization is rejected.

Each organization currently has one control project for organization administration and one
default project for customer workloads. Public multi-project management is intentionally
postponed until there is an operational need for additional projects. Project and organization
deletion must then include explicit, ordered cleanup of dependent assignments and roles rather
than relying on `ON DELETE CASCADE`.

## Auditing

Auditing uses the classic Postgres audit-trigger pattern: a single generic trigger function, written once, that on `INSERT`/`UPDATE`/`DELETE` writes the table name, primary-key values, action, and old/new row state (as `jsonb`) to one shared `audit_log` table. The primary-key values are also `jsonb`, so the same shape handles single-column and composite keys. Making a table audited is then a matter of attaching this trigger to it, rather than hand-writing per-table audit logic — a migration helper (`audit.enable(target_table regclass, excluded_columns text[] default '{}')`) discovers the primary key and wraps the `CREATE TRIGGER` boilerplate so enabling auditing on a new table is a one-line call in that table's migration.

Without `excluded_columns`, the trigger would write whatever the table holds into `audit_log` verbatim, including any secret-bearing column (password hash, API key, session token) — and `audit_log` typically has broader read access than the table it audits, since it's meant to be browsed by admins/support rather than gated per source table. The excluded column names are passed as trigger arguments and stripped from the captured row (`to_jsonb(row) - excluded_columns`) before the row is written, so a table with a secret column stays audited for everything else without the secret ever landing in `audit_log`. This list isn't re-checked automatically when a table's schema changes later, so a migration that adds a new secret-bearing column to an already-audited table must also update its `excluded_columns` call in the same migration — there's no enforcement for this beyond review, since the trigger has no way to know a new column is secret-shaped just from its name or type.

`excluded_columns` only ever strips columns that are secret-shaped regardless of whose data they are (password hashes, tokens) — it isn't a general per-row redaction mechanism, so ordinary PII (name, email) that happens to belong to a since-deleted or erasure-requested user still sits in old/new row state indefinitely, same as it would for any other row. A data-erasure request against `audit_log` is expected to be rare enough that it doesn't warrant a dedicated, permission-gated code path of its own. A database administrator handles it as a controlled maintenance operation, including temporarily disabling the immutability trigger, on the rare occasion it is needed.

No permission in the system grants `UPDATE` or `DELETE` on `audit_log`, including for `superadmin` — it's written only by the trigger itself. A database trigger rejects both operations, including through the application's current schema-owner connection. Without this, a sufficiently privileged actor could edit or remove the very rows meant to record their own actions, which would defeat the point of auditing.

No permission grants read access to `audit_log` through the API either, and it carries no `project_id` column or RLS policy of its own. It's only ever read by a database administrator connecting directly, which is also who bears responsibility for not exposing one tenant's rows to another when investigating an incident. If a future need arises to expose audit history through the API (e.g. an admin-facing activity log), that read path would need the same project-scoped filtering and RLS treatment required for resource tables in "Enforcement" above — it does not get that treatment today because the need doesn't exist yet.

That's an application-layer guarantee, though, and the same reasoning that puts a database-level backstop behind tenant isolation (see "Enforcement" above) applies here: without one, a single interceptor bug, or a raw SQL statement through the application connection, would have no obstacle at all to tampering with the log. The immutability trigger therefore prevents ordinary SQL from changing audit history after a row is written. If deployment later separates schema ownership from the runtime database role, `UPDATE` and `DELETE` privileges should also be withheld from that runtime role as defense in depth.

Triggers don't see the acting user by default, since that's an application-level concept. `internal/data/pgdb` sets two transaction-local settings — `app.user_id` and `app.trace_id` — at the start of every transaction it opens, pulled from `ctx`; the trigger function reads both via `current_setting(..., true)` (the `missing_ok` form, so a transaction that never set them still writes an audit row rather than failing) and attaches them to each row it writes. `SET LOCAL` rather than `SET` matters here specifically because of connection pooling: the setting resets at `COMMIT`/`ROLLBACK`, so a pooled connection handed to a different request between transactions never carries over the previous request's actor.

Every pgstore package is expected to go through `pgdb`'s transaction helper by convention rather than this being structurally enforced (e.g. by hiding the underlying pool behind it) — the cost of forcing that in code, in terms of the abstraction every store would need, outweighs the risk: a store that bypasses it simply produces an audit row with a `NULL` actor/trace, not a security hole, and is easy to spot and fix.

`app.trace_id` is set from the active trace's ID (from request tracing context, the same one already flowing through `otelgrpc`), stored in its own `audit_log` column — it answers "which request produced this row," distinct from "who." This is useful for correlating an audit entry back to logs/traces, particularly when the same user made several changes close together, or when a system-initiated change needs to be traced back to the job run that caused it.

Some writes have no human actor at all: the bootstrap CLI acting before any user exists, or a DBOS background step running outside a request. For these, a seeded `robot` user (a fixed, well-known ID, guaranteed to exist the same way the `theapp` org and its `dev`/`control` projects are guaranteed to exist — see "Bootstrapping" below) is set as `app.user_id` instead of leaving it unset. This keeps `audit_log.actor_id` a real, meaningful value for system-initiated changes, and keeps a `NULL` actor reserved for the case of "a write bypassed `pgdb`," rather than that also being the normal shape of a system action.

## Bootstrapping

Granting the first `superadmin` can't happen through the gRPC API, since every endpoint that could do it is itself permission-gated. This is done through an admin CLI tool that talks to the database directly, bypassing the interceptor layer. The tool also guarantees the `theapp` organization and its `dev` and `control` projects, and the `robot` system user (see "Auditing" above), always exist, since bootstrapping and local development both need these stable, known rows to seed roles and attribute system-initiated writes to, rather than creating them ad hoc each time.

The CLI is built with [urfave/cli](https://github.com/urfave/cli), which gives it shell completion (bash, zsh, fish, PowerShell) for free. It lives in `cmd/cli`, alongside the existing `cmd/server` entrypoint, with individual commands split into their own files under a `commands` package. It imports `internal/core` directly rather than calling the gRPC API, since its whole purpose is to act before the permission system it's bootstrapping can authorize anything.

Because the CLI bypasses the interceptor entirely, the permission model provides no protection against its misuse — whoever can run it with valid database credentials can grant `superadmin` to any user. This makes host and credential access the actual security boundary for the whole system's root of trust, not a detail left to deployment convention: the CLI is only ever run from a restricted set of hosts with its own database credentials, separate from the ones the running service uses.

Writes made through the CLI still go through `pgdb`, so they're audited the same as any other write — but the `robot` user (see "Auditing" above) is reserved for genuinely actor-less writes, and a human running the bootstrap CLI to grant the first `superadmin` is not that. Every CLI command that mutates data requires an `--operator` flag naming the acting user (by UUID or email, resolved the same way any user lookup is), which the CLI sets as `app.user_id` for that transaction instead of `robot`; the command refuses to run without it. This keeps the operation that establishes the whole system's root of trust individually attributable in `audit_log`, rather than collapsing every bootstrap and background-job write into the same anonymous actor.

The `theapp` organization, its `dev` and `control` projects, and the `robot` user aren't only at risk from a missing bootstrap run — once they exist, they're also reachable through ordinary organization/project deletion and any future user-deletion path, the same way any other org, project, or user row is. Deleting any of them would take down whatever depends on their specific identity: `org:create` is gated on a role assignment scoped to the `theapp` org's control project, internal staff access depends on `theapp` membership, and every actor-less audit row points at `robot` (see "Auditing" above). Renaming `theapp` or `dev` carries the same risk, since internal-staff membership and the bootstrap CLI's own lookup key off `theapp`'s name; a control project's identity, by contrast, is tracked structurally (`is_control`, not its name) precisely so renaming one doesn't have this effect — only deleting it does. Unlike other rows lost to a bad delete, there's no clean recovery step for these through the ordinary API — the bootstrap CLI guarantees they exist, but doing so idempotently on startup is a different claim than repairing a live system that's already had real projects and roles built on top of them.

The corresponding protection belongs in the schema when an ordinary API first permits one of these destructive mutations. At that point, the affected table gets a protected-identity marker and a trigger that rejects changing or deleting the protected row, with coverage through the public mutation API. Adding those columns before such an API exists would protect only direct SQL access, which is outside the pgstore API boundary and would leave no meaningful application behavior to test.
