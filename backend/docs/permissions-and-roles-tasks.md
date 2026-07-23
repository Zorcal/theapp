# Permissions and roles — implementation tasks

Breaks down docs/permissions-and-roles.md into ordered, independently-shippable tasks. Phases 1–11
are complete. They established users/auth, organizations/projects, the RBAC schema, system roles,
permission resolution, the bootstrap CLI, and project-scoped enforcement. Phase 12 is the current
phase: finish the system-role API that has already been scaffolded in `schemas/system_role.proto`
before beginning custom-role management.

## Working process

For any new agent session working on this, make sure to read all .md (CLAUDE.md and README.md) files in the codebase to
get the proper context.

This is built iteratively, phase by phase. Work through all the tasks in a phase, then stop and
hand the whole phase back for review rather than continuing on to the next phase. The code gets
committed after review before the next phase starts, so each phase begins from a clean, committed
baseline. Within a phase, tasks can be done in any order that respects their listed dependencies —
they don't each need their own individual review round.

Phases are deliberately small and vertical: each one delivers one self-contained, reviewable
increment — a table, a resolver, a CLI command, an endpoint — rather than a broad layer of work
spanning many concerns. This means more phases and some later rework (a table introduced in one
phase sometimes needs revisiting once a later phase's related tables exist), but it keeps each
review small and keeps no phase assuming the full shape of
a later phase's schema up front — which matters because that shape may still change mid-implementation.

The CLI (`cmd/cli`) grows the same way, in its own dedicated phases rather than as tasks
folded into whichever feature phase happens to need a new command: phase 1 stands the tool up with
a `user create` command; phase 5 adds granting `superadmin`; phase 9 adds bootstrapping `theapp`/
`dev`/`control`; phase 27 adds seeding the `robot` user. Each CLI phase depends only on the scaffold
(phase 1) plus whatever core-layer function it wires up, never on another CLI phase.

## Phase 1 — CLI tool scaffold and user creation — done

1. `cmd/cli` CLI scaffold ([urfave/cli](https://github.com/urfave/cli), shell completion, commands split into their own files under a `commands` package), alongside the existing `cmd/server` entrypoint. Every mutating command takes a required `--operator` flag (resolved by UUID or email, the same way any user lookup is) for actor attribution — it has no observable effect until `pgdb` sets `app.user_id` from it (phase 20) and auditing exists to record it (phase 25), but building it into the scaffold now means no later command has to retrofit it.
2. `user create` command, wired to the existing user-creation core logic. Depends on 1.

Also delivered in this phase, beyond the original task list: a `db migrate` command (no `--operator` — it's schema DDL, not an audited data write, and it's what creates the `users` table in the first place); a `user list` command (paginated, no `--operator` since it's read-only); and `scripts/seed-dev-operator.sh` (`make seed-dev-operator`) to seed a local bootstrap operator, since `--operator` always requiring an existing user means the very first one in a fresh environment can't come from the CLI itself.

**Checkpoint:** an operator can create a user via the CLI today. Met.

## Phase 2 — permission types and table — done

3. `mdl.Permission` type and the hardcoded constant list, in `internal/core/mdl`. It began with the permissions the existing `user` RPCs needed (`user:read`, `user:create`, `user:update` — no `user:delete`, since there's no delete endpoint) and grows alongside each API surface. Phase 12 adds the separate `system-role:*` namespace; the later custom-role service gets its own `custom-role:*` permissions rather than sharing these.
4. Migration: `permissions` table, in a new `rbac` schema (mirroring `useraccess`) that will hold this feature's tables going forward.

**Checkpoint:** the type compiles and the migration runs; the table exists (empty until phase 3 seeds it — there's no role yet to reference a permission by ID). Met.

## Phase 3 — system roles and permission seeding — done

5. Migration: separate `system_roles` and `custom_roles` tables, each with its own permission join table. Depends on 4.
6. System roles are structurally protected from custom-role mutation because they live in a separate table. Depends on 5.
7. `internal/data/pgschema/seed.sql` (its statements wrapped in a single `BEGIN`/`COMMIT`) plus `pgschema.Seed(ctx, pool)`, run wherever a pool is set up (`cmd/server`, test setup): idempotent (`ON CONFLICT ... DO NOTHING`) `INSERT`s for every permission, `superadmin`, and its grants. Depends on 3, 4, 5.
8. System role definitions: `superadmin` (every permission), seeded via 7. Depends on 5, 7.

Permissions and system roles are rows inserted by `seed.sql`, not something any Go code reconciles at runtime or filters on read. Removing a permission or a system role is a manual step against the database after the code change deploys (see `internal/core/rbac/README.md`) — an accepted tradeoff given how rarely this is expected to happen; if that stops being true, this cleanup can move into a `cmd/cli` command instead of staying a hand-run SQL snippet. `useradmin`/`rolesadmin` are dropped for now — illustrative roles with no permissions of their own to hold yet, given only `user:*` permissions exist at this point; add them back once there's a real permission set for each to scope down to.

**Checkpoint:** system roles exist in the DB with correct permission sets, proven by `TestCore_integration`. `pgschema.Seed`'s idempotency (`TestSeed`, asserting the total row count across every table is unchanged after a second call) has its own dedicated test. Met.

## Phase 4 — system-scope assignment and resolver — done

9. Migration: `system_role_assignments` table (user, role — no project or org). Depends on 5.
10. The `system_role_assignments.role_id` foreign key targets `system_roles`, structurally rejecting custom roles. Depends on 5, 9.
11. `mdl.AuthUser` struct, alongside existing `mdl/auth.go`. `mdl.AuthSession` isn't added yet — it pairs `AuthUser` with a `ProjectID` that has no meaning until request-time resolution exists, so it's deferred to phase 6, which is what actually assembles one.
12. System-scope-only resolver: `auth.Core.AuthUser(ctx, userID)` resolves a user's identity and the permissions it holds from `system_role_assignments` alone (project/org legs added in phase 11), backed by a `PermissionStorer` interface implemented by `pgrbac.Store`. Depends on 8, 9, 11.
13. `rbac.Core.AssignSystemRole(ctx, userID, roleName)` inserts a `system_role_assignments` row (no gRPC endpoint yet). Depends on 9, 10.

**Checkpoint:** given a `system_role_assignments` row inserted via `AssignSystemRole`, `AuthUser` resolves the correct identity and permission set for that user, proven by `auth.TestCore_integration`. Rejection of a custom role is proven by `TestStore_AssignSystemRole_error`. Met.

## Phase 5 — CLI: grant superadmin — done

14. `role assign-system` command (takes `--role` rather than being superadmin-specific, since 13 itself assigns any system role by name), wired to 13, using the scaffold from phase 1. Depends on 1, 13.

**Checkpoint:** an operator can grant `superadmin` to a user via the CLI. Verified manually against a local dev database (`role assign-system --role superadmin`, then confirmed the `system_role_assignments` row).

## Phase 6 — permission registry and the superadmin gate — done

15. Permission registry (`permissionRegistry` in `internal/api/grpc/server.go`): map of every existing RPC method to its real, correct required permissions (using the constants from 3), including explicit empty-list entries for endpoints that legitimately require none (`AuthService/RevokeAllSessions`). `TestPermissionRegistry_exhaustiveness` enumerates every method the server actually registers (via `grpc.Server.GetServiceInfo`) and asserts each one is either public (`publicMethods`) or has a registry entry, so a new RPC added without one fails the build. No new proto needed — this maps permissions onto RPCs that already exist. Depends on 3.
16. `mdl.AuthSession` struct (`User AuthUser`, `ProjectID int`), alongside existing `mdl/auth.go`. `ProjectID` stays at its zero value this phase — the request-metadata plumbing that would populate it doesn't exist until phase 10. `permissionUnaryInterceptor` resolves one per protected request via 12 and enforces `codes.PermissionDenied` via the registry (15); it runs right after the existing auth interceptor in the unary chain, since it depends on the authenticated user ID already being in context. Depends on 12, 15.

**Checkpoint:** every existing RPC is permission-gated and enforced. A user granted `superadmin` via phase 5's CLI command can call anything requiring a permission; every other authenticated user is denied on any endpoint with a non-empty required-permission list. Still no organizations, projects, custom roles, or self-service creation endpoint. Proven by `TestPermissionRegistry_exhaustiveness`, `TestPermissionUnaryInterceptor(_error)`, and `TestAuth_MagicLinkIntegration`'s use of a real `ListUsers` call gated on an assigned role.

## Phase 7 — organizations and projects schema — done

17. Migration: new `org` schema; `organizations` and `projects`, with `projects.org_id` (a project belongs to exactly one organization) and `UNIQUE (org_id, name)` enforcing a project's name is unique within its organization. Project IDs (the `projects.id` serial) are globally unique by construction. No mapping table or trigger: an earlier version of this migration modeled the org/project link as a separate `org_project` table with a trigger to enforce the per-org name uniqueness, but since the design has no case where a project's org changes or a project spans more than one org, that indirection bought nothing — a plain `org_id` column plus a native `UNIQUE` constraint enforces the same invariant with no PL/pgSQL and no per-write trigger.
18. Migration: `org_membership` table (`user_id`, `org_id`) gating org-scoped assignment. Depends on 17.

**Checkpoint:** the tables exist; org/project/membership rows can be created and joined correctly, and the per-org unique-name constraint holds on both insert and rename — proven directly at the DB level (verified manually; no core logic yet, so no Go tests are checked in for this phase — dedicated tests land once `internal/core/org` exists in phase 8 onward).

## Phase 8 — org/project core creation functions — done

19. Core-layer `CreateOrganization` / `CreateProject` functions (`internal/core/org`, backed by `internal/core/pgstores/pgorg`; no gRPC endpoint yet). `mdl.CreateOrganization` takes the default project's name explicitly (`ProjectName`) rather than defaulting it to the organization's own name. The store layer's `CreateOrganization` inserts the organization and a control project (`org.projects.is_control`) in one statement, so the two rows are created atomically; a project's identity as an org's control project is tracked by `is_control`, not by name, so it can be renamed later without losing that identity — `mdl.Organization.ControlProjectID` exposes it directly, no name lookup needed. The core layer's `CreateOrganization` then calls the store's `CreateProject` inside the same `Transactor.RunTx` to add the caller's named default project. `CreateProject` resolves the target org via a join rather than relying on the `org_id` foreign key's violation error, returning `sql.ErrNoRows` (→ `mdl.ErrNotFound`) the same way `pgrbac.AssignSystemRole` resolves a role by name — so an invalid org ID fails via "zero rows returned", not a distinct FK-violation code path. Depends on 17.

**Checkpoint:** an org and a project can be created via these functions directly, proven by `TestCore_integration`, `TestStore_CreateOrganization(_error)`, and `TestStore_CreateProject(_error)` — independent of any CLI or gRPC surface.

## Phase 9 — CLI: bootstrap theapp/dev/control — done

20. `org bootstrap` command, wired to 19, using the scaffold from phase 1. It looks up `theapp` by name before creating it, so re-running the command is a no-op once it (and its `dev`/control projects, created alongside it per phase 8) already exists, rather than failing on `mdl.ErrAlreadyExists`. `theapp.ControlProjectID` (populated by 19 regardless of whether the org was just created or already existed) is used directly — no separate lookup for the control project is needed. This adds `OrganizationByName` / `ProjectByName` to `internal/core/org` and `internal/core/pgstores/pgorg`, alongside the existing `Create*` functions from phase 8. Depends on 1, 19.

**Checkpoint:** an operator can bootstrap `theapp`/`dev`/`control` via the CLI. Verified manually against a local dev database (`org bootstrap --operator ...`, run twice, confirming identical org/project IDs both times).

## Phase 10 — ProjectID metadata plumbing — done

21. `ProjectID` request-metadata plumbing: `projectUnaryInterceptor` reads the `x-project-id` metadata key and attaches it to the context (`contextWithProjectID` / `projectIDFromContext`, in the new `internal/api/grpc/context.go` — unlike `mdl.ContextWithAuthUser`, this is transport-layer plumbing with no app-wide need yet, so it stays gRPC-internal rather than living in `mdl`; phase 11 folding it into `mdl.AuthSession` is what will give it an app-wide home), rejecting the call with `codes.InvalidArgument` if it's missing, empty, non-numeric, or not a positive integer. `publicMethods` and a new explicit `noProjectMethods` list (seeded with `AuthService/RevokeAllSessions`, the one existing endpoint that reflects only the caller's own identity) are exempt. Runs after `authUnaryInterceptor`, before `permissionUnaryInterceptor` — the latter doesn't consume `ProjectID` yet, that's phase 11. No stream counterpart yet, mirroring `permissionUnaryInterceptor` (added only once a protected streaming RPC needs it).

**Checkpoint:** a call missing `ProjectID` metadata is rejected unless the RPC is on the exceptions list — proven independent of any permission resolution logic, by `TestProjectUnaryInterceptor(_error)`.

## Phase 11 — project-scoped resolution — done

22. Migration: `project_role_assignments`, `org_role_assignments` tables. Depends on 5, 17.
23. Custom roles carry a non-null `org_id`, now that organizations exist; system roles remain ownerless in their separate table. This is defined in phase 3's role migration. Depends on 5, 17.
24. Three-way union resolver: extend 12 to also union `project_role_assignments` (direct) and `org_role_assignments` (via project→org). Depends on 12, 21, 22.
25. Interceptor updated to resolve `mdl.AuthSession` via a single core method taking a `*int` project ID: non-nil triggers the full three-way resolver (24) plus an org lookup, setting `ProjectID`/`OrgID` on the session; nil (for a method in `noProjectMethods`) resolves system-scope permissions only, leaving `ProjectID`/`OrgID` unset, replacing the system-scope-only version from phase 6. Depends on 16, 24.

**Checkpoint:** permission checks are project-scoped end-to-end, with real organizations/projects backing them — still no custom roles or self-service org/project creation endpoint.

## Phase 12 — system role service

26. Finalize `schemas/system_role.proto` and its generated gRPC/gateway/OpenAPI artifacts: list system roles, list a system role's permissions, assign/unassign a system role, and list a user's system-role assignments. Every method is anchored on `theapp`'s control project. The schema and unimplemented handler scaffold already exist; finish field documentation and confirm request/response shapes before implementing them.
27. Seed `system-role:read`, `system-role:assign`, and `system-role:unassign`; add their `mdl.Permission` constants and register every `SystemRoleService` method in `permissionRegistry`. List operations require `system-role:read`, assign requires `system-role:assign`, and unassign requires `system-role:unassign`. This part is already complete. Depends on 7, 15, 26.
28. Add paginated store/core read operations for system roles, their permissions, and a user's system-role assignments; add `UnassignSystemRole` alongside the existing `AssignSystemRole`. Depends on 9, 13, 26.
29. Enforce the system-scope superset rule inside the same transaction as assign and unassign: the actor's authority must be resolved only from its own `system_role_assignments`, never from project- or org-scoped grants. Depends on 12, 28.
30. Reject unassigning the last system-scope assignment that carries the system-role management permissions being removed, then wire and validate all five gRPC handlers with unit and integration coverage. Handler validation includes proving that `x-project-id` resolves specifically to `theapp`'s control project, not merely to any valid project. Depends on 20, 27, 29.

The service is deliberately separate from custom-role management. It can read and assign seeded
system roles, but it cannot create, edit, or delete them. Custom roles never enter
`system_role_assignments`, structurally enforced by that table's foreign key to `system_roles`.

**Checkpoint:** an authorized system administrator can list and assign/unassign seeded system roles through the API; all calls require `theapp/control`, grants and revokes enforce the system-only superset rule, and the last system-role-management holder cannot be removed.

## Phase 13 — custom role service: CRUD and ownership

31. Proto schema: `schemas/role.proto` (`RoleService` — create/update/delete a custom role and list custom roles). Add the `custom-role:create`, `custom-role:read`, `custom-role:update`, and `custom-role:delete` permissions and run `make generate`.
32. Custom-role service skeleton: create/edit/delete custom roles. System roles are not accepted by this service and live behind `SystemRoleService`. Depends on 23, 31.
33. Role-org-ownership check on every custom-role operation, matching the role's `org_id` to the caller's resolved org. Depends on 32.
34. Role listing filtered by the caller's org (for the "assign a role" UI). Depends on 32.

**Checkpoint:** custom roles can be created, edited, deleted, and listed via the API, correctly scoped to the caller's org — no custom-role assignment yet.

## Phase 14 — custom role service: assignment endpoints

35. Add project- and org-scope assign/unassign endpoints using `custom-role:assign` and `custom-role:unassign`. They write only to `project_role_assignments` and `org_role_assignments`; system assignment remains exclusively in `SystemRoleService`. Depends on 18, 22, 32.
36. Gate org-scoped assignment on `org_membership`, and require the custom role's owning `org_id` to match the target project/org. Depends on 18, 33, 35.

**Checkpoint:** custom roles can be assigned and unassigned at project or org scope. Privilege-escalation and lockout checks are added in phases 15–16, so these endpoints remain restricted to trusted internal testing until then.

## Phase 15 — custom role service: privilege escalation checks

37. Privilege-escalation superset check on grant: resolve the actor's permission set fresh in the target project or org scope, inside the same transaction as the grant. Depends on 24, 35.
38. Apply the same superset check to revoke and to permissions added to or removed from a custom role. Depends on 32, 37.

**Checkpoint:** custom-role grant, revoke, and permission edits enforce the correctly scoped superset rule.

## Phase 16 — custom role service: lockout and cleanup

39. Last-custom-role-management-holder lockout check on revoke, role deletion, and permission removal from a custom role. Depends on 35, 37.
40. Explicit project/org assignment-row cleanup on custom-role deletion. Depends on 35.

**Checkpoint:** the custom-role service is safe to expose more broadly — escalation, lockout, and cleanup are all in place.

## Phase 17 — org creation endpoint

41. Proto schema: `schemas/organization.proto` (create/delete org). Run `make generate`.
42. Org creation gRPC endpoint: wires 19 behind `org:create` scoped to `theapp/control`, plus the `theapp` org-membership check. The membership check resolves the `theapp` org by `mdl.BootstrapOrgName` — at that point, rename the constant to drop the `Bootstrap` prefix (it's no longer just a CLI-seeding detail but a permission-anchoring concept referenced from endpoint code too, the same as `mdl.ControlProjectName`). Depends on 25, 35, 41.
43. Org creation request carries the default project's name (`CreateOrganization.ProjectName`), passed through to 19's `CreateOrganization`, which creates the organization, its control project, and the named default project. Depends on 42.
44. Org creator assigned an admin role at org scope (see permissions-and-roles.md, "Creating organizations and projects"). Depends on 35, 42.

**Checkpoint:** an organization can be created end-to-end via the API, seeded with a default project and an org-scoped admin assignment for its creator.

## Phase 18 — project creation endpoint

45. Proto schema: `schemas/project.proto` (create/delete project). Run `make generate`.
46. Project creation gRPC endpoint: wires 19 behind `project:create`, anchored on the org's default project the same way `org:create` anchors on `theapp/control`. Depends on 25, 45.

**Checkpoint:** a project can be created end-to-end via the API within an existing org.

## Phase 19 — org/project deletion cascade cleanup

47. Explicit cascade cleanup for org/project deletion (assignments and custom roles). Depends on 35, 42, 46.

**Checkpoint:** deleting an org or project leaves no dangling assignment/role/mapping rows.

## Phase 20 — pgdb transaction-local settings

48. `pgdb`: set `app.project_id`, `app.user_id`, `app.trace_id` as `SET LOCAL` transaction-scoped settings, sourced from `ctx`.

**Checkpoint:** a test transaction shows the three settings are visible via `current_setting()` and reset at commit/rollback.

## Phase 21 — RLS on project-scoped resource tables

49. RLS + `FORCE ROW LEVEL SECURITY` on project-scoped resource tables, keyed on `app.project_id`. Depends on 17, 48.
50. CI test (real Postgres): app-role connection, set `app.project_id`, assert cross-project `SELECT` returns nothing. Depends on 49.

**Checkpoint:** the CI test in task 50 passes.

## Phase 22 — RLS on assignment tables and the cross-user listing function

51. RLS on `project_role_assignments` / `org_role_assignments` / `system_role_assignments`, keyed on `app.user_id`. Depends on 9, 22, 48.
52. `SECURITY DEFINER` function for "list everyone with a role in project X". Depends on 51.

**Checkpoint:** assignment-table RLS holds, and the one function that legitimately needs to see across users works correctly.

## Phase 23 — is_protected backstop

53. `is_protected` columns + trigger on `organizations`/`projects`/`users`. Depends on 17.

**Checkpoint:** deleting or renaming `theapp`/`dev`/`control`/`robot`/a protected user is rejected at the DB level.

## Phase 24 — soft delete for users

54. Migration: `deleted_at` on `users`.
55. Exclude soft-deleted users (`deleted_at IS NULL`) from all three legs of the resolver (24). Depends on 24, 54.
56. CI test (real Postgres): soft-deleted user with live rows in all three assignment tables resolves to zero permissions. Depends on 55.

**Checkpoint:** the CI test in task 56 passes.

## Phase 25 — auditing: the audit_log table and trigger

57. Migration: `audit_log` table plus the generic audit trigger function and the `audit.enable(table, excluded_columns)` migration helper.
58. Revoke `UPDATE`/`DELETE` on `audit_log` for the app's runtime DB role. Depends on 57.

**Checkpoint:** `audit.enable` can be attached to a table (proven on one test table) and produces correctly-shaped, immutable rows.

## Phase 26 — auditing: wire onto existing tables

59. Wire `audit.enable(...)` onto every table introduced in phases 1–24 that should be audited, with `excluded_columns` for any secret-bearing columns. Depends on 48, 57.

**Checkpoint:** every relevant table is audited.

## Phase 27 — CLI: seed robot user

60. CLI command (or extension of phase 9's bootstrap command) seeding the `robot` user, guaranteed to exist for actor-less audit attribution on system-initiated writes. Depends on 1; only functionally relevant once 59 lands.

**Checkpoint:** the `robot` user exists and is used to attribute system-initiated audit rows.

## Phase 28 — auth-data-exposure endpoint

61. Extend `schemas/auth.proto` with the auth-data-exposure RPC (caller ID, email, resolved permissions). Run `make generate`.
62. Auth-data-exposure endpoint. Depends on 24, 61.

**Checkpoint:** the endpoint returns the caller's ID, email, and resolved permissions.

## Phase 29 — discover-accessible-projects endpoint

63. Extend `schemas/auth.proto` with the discover-accessible-projects RPC (paginated — a system-scoped caller resolves to every project in the system). Run `make generate`.
64. Discover-accessible-projects endpoint. Depends on 24, 63.

**Checkpoint:** the endpoint lists every project the caller has any role in, paginated.

## Phase 30 — org-scoped user management endpoints

65. Extend `schemas/organization.proto` with a create-or-assign-user RPC: creates a user if none exists with the given email, then assigns them to the calling org; if the user already exists, only the org assignment happens. Anchored on the org's control project — the `x-project-id` metadata must be that project's ID. Run `make generate`. Depends on 41, 42.
66. Extend `schemas/organization.proto` with an org-scoped list-users RPC, separate from `UserService.ListUsers` (see permissions-and-roles.md, "Managing users within an organization"). Also anchored on the org's control project; the request body additionally carries a project ID filter, resolved through the three-way union (24), not `org_membership`. Run `make generate`. Depends on 41, 42.
67. Wire both endpoints behind the appropriate org-scoped permissions. Depends on 65, 66.

**Checkpoint:** a user can be created-or-assigned into an organization, and users can be listed scoped to an organization or filtered down to a specific project within it, both via the API.

## Ongoing / cross-cutting

68. Application-level `project_id` filter convention audit across every core-layer store method touching a project-scoped resource (`WHERE id = $1 AND project_id = $2`). Not a one-time task — apply it as a review checklist to every project-scoped store method as it's written, alongside phase 21.
69. Periodic sweep job for soft-deleted users' assignment rows past retention. Depends on 54, existing DBOS workflow infra (`internal/workflows`).

## Notes

- The app isn't in production yet, so migrations for this feature don't need to be purely additive. Edit an existing migration file directly (e.g. add a column, change a table this feature just introduced) instead of creating a new migration to alter it. Reserve new migration files for genuinely new tables/objects. Only start appending forward-only migrations once this schema has shipped to a live environment. This is what makes the vertical-slice approach above practical — e.g. task 23 keeps custom-role ownership in the role-table migration from task 5 rather than layering an `ALTER TABLE` on top.
- The same applies to the new proto schemas: edit them in place as message shapes settle instead of layering on deprecated fields — there are no external clients depending on them yet.
- Tasks 50 and 56 should be written as soon as their dependencies land, not deferred to the end — they're what prove the backstop actually works.
- Review happens once per phase, at the phase boundary — see "Working process" above. The phase checkpoints describe what should be true and demonstrable by the time that review happens.
- Phase 14's checkpoint calls out a phase that is intentionally not yet safe to expose broadly. It's the one phase in this breakdown where "reviewed and committed" doesn't mean "safe to deploy publicly" — flag this distinction if it ever needs to leave a dev/staging environment before phase 16 lands.
- `scripts/seed-dev-operator.sh` (`make seed-dev-operator`) seeds a local-only bootstrap operator user (`operator@theapp.com`) and grants it `superadmin` at system scope, so every `cmd/cli` command has a valid, usable `--operator` without a manual `psql` insert. The `superadmin` grant is a direct SQL insert rather than a CLI call, since phase 5 hasn't added a CLI command for it yet — switch the script to that command once it exists. It's a standing dev convenience, not a task with its own checkpoint — revisit it whenever a phase changes what a fresh local environment needs to be immediately useful (e.g. phase 9 might have it add `theapp` org membership) instead of leaving that as another manual step.
- `permissionStreamInterceptor` mirrors `permissionUnaryInterceptor` exactly and is wired into the stream chain alongside `authStreamInterceptor`, even though no streaming RPC exists yet to exercise it — built proactively so a future streaming RPC is safe by construction rather than depending on someone remembering to add it when it lands.
- `projectUnaryInterceptor` and `projectStreamInterceptor` (phase 10) were removed once `authUnaryInterceptor`/`authStreamInterceptor` started parsing `x-project-id` themselves to build `mdl.AuthSession` — at that point the two interceptors ran the identical check a second time, and since `authUnaryInterceptor`/`authStreamInterceptor` always run first in the chain, the dedicated project interceptors could never actually be the one to reject a malformed project ID. Project-ID validation now lives solely in `authUnaryInterceptor`/`authStreamInterceptor`.
