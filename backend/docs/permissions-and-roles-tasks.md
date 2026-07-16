# Permissions and roles — implementation tasks

Breaks down docs/permissions-and-roles.md into ordered, independently-shippable tasks. Existing
code referenced for grounding: users/auth already exist (internal/core/mdl, internal/core/auth,
internal/core/pgstores/pguser, internal/api/grpc/unary_interceptors.go, schemas/auth.proto,
schemas/user.proto); organizations, projects, roles, and permissions don't exist yet, and there's
no proto schema for any of them. `cmd/cli` and the urfave/cli dependency exist as of phase 1 (see
below), alongside the existing `cmd/server`.

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
spanning many concerns. This means more phases and some later rework (a column added in one phase
sometimes needs revisiting once a later phase's tables exist, e.g. `roles.org_id` isn't added until
organizations exist), but it keeps each review small and keeps no phase assuming the full shape of
a later phase's schema up front — which matters because that shape may still change mid-implementation.

The CLI (`cmd/cli`) grows the same way, in its own dedicated phases rather than as tasks
folded into whichever feature phase happens to need a new command: phase 1 stands the tool up with
a `user create` command; phase 5 adds granting `superadmin`; phase 9 adds bootstrapping `theapp`/
`dev`/`control`; phase 26 adds seeding the `robot` user. Each CLI phase depends only on the scaffold
(phase 1) plus whatever core-layer function it wires up, never on another CLI phase.

## Phase 1 — CLI tool scaffold and user creation — done

1. `cmd/cli` CLI scaffold ([urfave/cli](https://github.com/urfave/cli), shell completion, commands split into their own files under a `commands` package), alongside the existing `cmd/server` entrypoint. Every mutating command takes a required `--operator` flag (resolved by UUID or email, the same way any user lookup is) for actor attribution — it has no observable effect until `pgdb` sets `app.user_id` from it (phase 19) and auditing exists to record it (phase 24), but building it into the scaffold now means no later command has to retrofit it.
2. `user create` command, wired to the existing user-creation core logic. Depends on 1.

Also delivered in this phase, beyond the original task list: a `db migrate` command (no `--operator` — it's schema DDL, not an audited data write, and it's what creates the `users` table in the first place); a `user list` command (paginated, no `--operator` since it's read-only); and `scripts/seed-dev-operator.sh` (`make seed-dev-operator`) to seed a local bootstrap operator, since `--operator` always requiring an existing user means the very first one in a fresh environment can't come from the CLI itself.

**Checkpoint:** an operator can create a user via the CLI today. Met.

## Phase 2 — permission types and table — done

3. `mdl.Permission` type and the full hardcoded constant list, in `internal/core/mdl`. The list currently covers only the permissions the existing `user` RPCs need (`user:read`, `user:create`, `user:update` — no `user:delete`, since there's no delete endpoint); it grows as later phases add resources and RPCs, rather than pre-declaring permissions for services that don't exist yet.
4. Migration: `permissions` table, in a new `rbac` schema (mirroring `useraccess`) that will hold this feature's tables going forward.

**Checkpoint:** the type compiles and the migration runs; the table exists (empty until phase 3 seeds it — there's no role yet to reference a permission by ID). Met.

## Phase 3 — static roles and permission seeding — done

5. Migration: `roles` table (`is_static`; `org_id` deferred to phase 11, added once organizations exist) and `role_permissions` join table. Depends on 4.
6. `is_static` trigger (`prevent_static_role_mutation`, `BEFORE UPDATE OR DELETE`) on `roles`. Depends on 5.
7. `internal/data/pgschema/seed.sql` (its statements wrapped in a single `BEGIN`/`COMMIT`) plus `pgschema.Seed(ctx, pool)`, run wherever a pool is set up (`cmd/server`, test setup): idempotent (`ON CONFLICT ... DO NOTHING`) `INSERT`s for every permission, `superadmin`, and its grants. Depends on 3, 4, 5.
8. Static role definitions: `superadmin` (every permission), seeded via 7. Depends on 5, 7.

Permissions and static roles are rows inserted by `seed.sql`, not something any Go code reconciles at runtime or filters on read. Removing a permission or a static role is a manual step against the database after the code change deploys (see `internal/core/rbac/README.md`) — an accepted tradeoff given how rarely this is expected to happen; if that stops being true, this cleanup can move into a `cmd/cli` command instead of staying a hand-run SQL snippet. `useradmin`/`rolesadmin` are dropped for now — illustrative roles with no permissions of their own to hold yet, given only `user:*` permissions exist at this point; add them back once there's a real permission set for each to scope down to.

**Checkpoint:** static roles exist in the DB with correct permission sets, proven by `TestCore_integration`. `pgschema.Seed`'s idempotency (`TestSeed`, asserting the total row count across every table is unchanged after a second call) and the `is_static` trigger (`TestIsStaticTrigger`) each have their own dedicated test. Met.

## Phase 4 — system-scope assignment and resolver — done

9. Migration: `system_role_assignments` table (user, role — no project or org). Depends on 5.
10. Trigger (`prevent_custom_role_system_assignment`, `BEFORE INSERT`) on `system_role_assignments` rejecting inserts where the target role isn't static. Depends on 5, 9.
11. `mdl.AuthUser` struct, alongside existing `mdl/auth.go`. `mdl.AuthSession` isn't added yet — it pairs `AuthUser` with a `ProjectID` that has no meaning until request-time resolution exists, so it's deferred to phase 6, which is what actually assembles one.
12. System-scope-only resolver: `auth.Core.AuthUser(ctx, userID)` resolves a user's identity and the permissions it holds from `system_role_assignments` alone (project/org legs added in phase 11), backed by a `PermissionStorer` interface implemented by `pgrbac.Store`. Depends on 8, 9, 11.
13. `rbac.Core.AssignSystemRole(ctx, userID, roleName)` inserts a `system_role_assignments` row (no gRPC endpoint yet). Depends on 9, 10.

**Checkpoint:** given a `system_role_assignments` row inserted via `AssignSystemRole`, `AuthUser` resolves the correct identity and permission set for that user, proven by `auth.TestCore_integration`. The `BEFORE INSERT` trigger rejecting a non-static role is proven by `TestStore_AssignSystemRole_error`. Met.

## Phase 5 — CLI: grant superadmin — done

14. `role assign-system` command (takes `--role` rather than being superadmin-specific, since 13 itself assigns any static role by name), wired to 13, using the scaffold from phase 1. Depends on 1, 13.

**Checkpoint:** an operator can grant `superadmin` to a user via the CLI. Verified manually against a local dev database (`role assign-system --role superadmin`, then confirmed the `system_role_assignments` row).

## Phase 6 — permission registry and the superadmin gate — done

15. Permission registry (`permissionRegistry` in `internal/api/grpc/permissions.go`): map of every existing RPC method to its real, correct required permissions (using the constants from 3), including explicit empty-list entries for endpoints that legitimately require none (`AuthService/RevokeAllSessions`). `TestPermissionRegistry_exhaustiveness` enumerates every method the server actually registers (via `grpc.Server.GetServiceInfo`) and asserts each one is either public (`publicMethods`) or has a registry entry, so a new RPC added without one fails the build. No new proto needed — this maps permissions onto RPCs that already exist. Depends on 3.
16. `mdl.AuthSession` struct (`User AuthUser`, `ProjectID int`), alongside existing `mdl/auth.go`. `ProjectID` stays at its zero value this phase — the request-metadata plumbing that would populate it doesn't exist until phase 10. `permissionUnaryInterceptor` resolves one per protected request via 12, enforce `codes.PermissionDenied` via the registry (15); runs right after the existing auth interceptor in the unary chain, since it depends on the authenticated user ID already being in context. Depends on 12, 15.

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

## Phase 10 — ProjectID metadata plumbing

21. `ProjectID` request-metadata plumbing: read/require the metadata key, explicit exceptions list for project-less endpoints.

**Checkpoint:** a call missing `ProjectID` metadata is rejected unless the RPC is on the exceptions list — proven independent of any permission resolution logic.

## Phase 11 — project-scoped resolution

22. Migration: `project_role_assignments`, `org_role_assignments` tables. Depends on 5, 17.
23. Add `org_id` (nullable) to `roles`, now that organizations exist (edits phase 3's migration from task 5 in place). Depends on 5, 17.
24. Three-way union resolver: extend 12 to also union `project_role_assignments` (direct) and `org_role_assignments` (via project→org). Depends on 12, 21, 22.
25. Interceptor updated to resolve `AuthSession` via the full three-way resolver (24) instead of the system-scope-only version from phase 6. Depends on 16, 24.

**Checkpoint:** permission checks are project-scoped end-to-end, with real organizations/projects backing them — still no custom roles or self-service org/project creation endpoint.

## Phase 12 — role service: CRUD and ownership

26. Proto schema: `schemas/role.proto` (`RoleService` — create/update/delete a custom role, assign/unassign, list roles). Run `make generate`.
27. Role service skeleton: create/edit/delete custom roles, rejecting any target with `is_static = true`. Depends on 23, 26.
28. Role-org-ownership check on every role-service operation, matching role `org_id` to the assignment's target org. Depends on 27.
29. Role listing filtered by caller's org (for the "assign a role" UI). Depends on 27.

**Checkpoint:** custom roles can be created, edited, deleted, and listed via the API, correctly scoped to the caller's org — no assignment yet.

## Phase 13 — role service: assignment endpoints

30. Assign/unassign endpoints writing to the three assignment tables, gated by the `org_membership` check (18) for org-scoped assignment. Depends on 18, 22, 27.
31. Restrict `system` scope assignment to static roles in the role service (mirrors the DB trigger in 10). Depends on 30.

**Checkpoint:** roles can be assigned/unassigned via the API. Privilege-escalation and lockout checks aren't in place until phases 14-15 — don't expose this beyond trusted internal testing until those land, since as it stands any caller with `role:assign` can grant permissions beyond their own.

## Phase 14 — role service: privilege escalation checks

32. Privilege-escalation superset check on grant: resolve the actor's permission set fresh, scoped correctly per grant target (project/org/system, no laundering upward), inside the same transaction as the grant. Depends on 24, 30.
33. Apply the same superset check to revoke. Depends on 32.

**Checkpoint:** grant and revoke both enforce the superset rule.

## Phase 15 — role service: lockout and cleanup

34. Last-role-management-holder lockout check on revoke, role deletion, and permission removal from a role. Depends on 30, 32.
35. Explicit assignment-row cleanup on role deletion. Depends on 30.

**Checkpoint:** the role service is now safe to expose more broadly — escalation, lockout, and cleanup are all in place.

## Phase 16 — org creation endpoint

36. Proto schema: `schemas/organization.proto` (create/delete org). Run `make generate`.
37. Org creation gRPC endpoint: wires 19 behind `org:create` scoped to `theapp/control`, plus the `theapp` org-membership check. The membership check resolves the `theapp` org by `mdl.BootstrapOrgName` — at that point, rename the constant to drop the `Bootstrap` prefix (it's no longer just a CLI-seeding detail but a permission-anchoring concept referenced from endpoint code too, the same as `mdl.ControlProjectName`). Depends on 25, 30, 36.
38. Org creation request carries the default project's name (`CreateOrganization.ProjectName`), passed through to 19's `CreateOrganization`, which creates the organization, its control project, and the named default project. Depends on 37.
39. Org creator assigned an admin role at org scope (see permissions-and-roles.md, "Creating organizations and projects"). Depends on 30, 37.

**Checkpoint:** an organization can be created end-to-end via the API, seeded with a default project and an org-scoped admin assignment for its creator.

## Phase 17 — project creation endpoint

40. Proto schema: `schemas/project.proto` (create/delete project). Run `make generate`.
41. Project creation gRPC endpoint: wires 19 behind `project:create`, anchored on the org's default project the same way `org:create` anchors on `theapp/control`. Depends on 25, 40.

**Checkpoint:** a project can be created end-to-end via the API within an existing org.

## Phase 18 — org/project deletion cascade cleanup

42. Explicit cascade cleanup for org/project deletion (assignments, custom roles, org-project mappings). Depends on 30, 37, 41.

**Checkpoint:** deleting an org or project leaves no dangling assignment/role/mapping rows.

## Phase 19 — pgdb transaction-local settings

43. `pgdb`: set `app.project_id`, `app.user_id`, `app.trace_id` as `SET LOCAL` transaction-scoped settings, sourced from `ctx`.

**Checkpoint:** a test transaction shows the three settings are visible via `current_setting()` and reset at commit/rollback.

## Phase 20 — RLS on project-scoped resource tables

44. RLS + `FORCE ROW LEVEL SECURITY` on project-scoped resource tables, keyed on `app.project_id`. Depends on 17, 43.
45. CI test (real Postgres): app-role connection, set `app.project_id`, assert cross-project `SELECT` returns nothing. Depends on 44.

**Checkpoint:** the CI test in task 45 passes.

## Phase 21 — RLS on assignment tables and the cross-user listing function

46. RLS on `project_role_assignments` / `org_role_assignments` / `system_role_assignments`, keyed on `app.user_id`. Depends on 9, 22, 43.
47. `SECURITY DEFINER` function for "list everyone with a role in project X". Depends on 46.

**Checkpoint:** assignment-table RLS holds, and the one function that legitimately needs to see across users works correctly.

## Phase 22 — is_protected backstop

48. `is_protected` columns + trigger on `organizations`/`projects`/`users`. Depends on 17.

**Checkpoint:** deleting or renaming `theapp`/`dev`/`control`/`robot`/a protected user is rejected at the DB level.

## Phase 23 — soft delete for users

49. Migration: `deleted_at` on `users`.
50. Exclude soft-deleted users (`deleted_at IS NULL`) from all three legs of the resolver (24). Depends on 24, 49.
51. CI test (real Postgres): soft-deleted user with live rows in all three assignment tables resolves to zero permissions. Depends on 50.

**Checkpoint:** the CI test in task 51 passes.

## Phase 24 — auditing: the audit_log table and trigger

52. Migration: `audit_log` table plus the generic audit trigger function and the `audit.enable(table, excluded_columns)` migration helper.
53. Revoke `UPDATE`/`DELETE` on `audit_log` for the app's runtime DB role. Depends on 52.

**Checkpoint:** `audit.enable` can be attached to a table (proven on one test table) and produces correctly-shaped, immutable rows.

## Phase 25 — auditing: wire onto existing tables

54. Wire `audit.enable(...)` onto every table introduced in phases 1-23 that should be audited, with `excluded_columns` for any secret-bearing columns. Depends on 43, 52.

**Checkpoint:** every relevant table is audited.

## Phase 26 — CLI: seed robot user

55. CLI command (or extension of phase 9's bootstrap command) seeding the `robot` user, guaranteed to exist for actor-less audit attribution on system-initiated writes. Depends on 1; only functionally relevant once 54 lands.

**Checkpoint:** the `robot` user exists and is used to attribute system-initiated audit rows.

## Phase 27 — auth-data-exposure endpoint

56. Extend `schemas/auth.proto` with the auth-data-exposure RPC (caller ID, email, resolved permissions). Run `make generate`.
57. Auth-data-exposure endpoint. Depends on 24, 56.

**Checkpoint:** the endpoint returns the caller's ID, email, and resolved permissions.

## Phase 28 — discover-accessible-projects endpoint

58. Extend `schemas/auth.proto` with the discover-accessible-projects RPC (paginated — a system-scoped caller resolves to every project in the system). Run `make generate`.
59. Discover-accessible-projects endpoint. Depends on 24, 58.

**Checkpoint:** the endpoint lists every project the caller has any role in, paginated.

## Ongoing / cross-cutting

60. Application-level `project_id` filter convention audit across every core-layer store method touching a project-scoped resource (`WHERE id = $1 AND project_id = $2`). Not a one-time task — apply it as a review checklist to every project-scoped store method as it's written, alongside phase 20.
61. Periodic sweep job for soft-deleted users' assignment rows past retention. Depends on 49, existing DBOS workflow infra (`internal/workflows`).

## Notes

- The app isn't in production yet, so migrations for this feature don't need to be purely additive. Edit an existing migration file directly (e.g. add a column, change a table this feature just introduced) instead of creating a new migration to alter it. Reserve new migration files for genuinely new tables/objects. Only start appending forward-only migrations once this schema has shipped to a live environment. This is what makes the vertical-slice approach above practical — e.g. task 23 (`roles.org_id`) edits the migration from task 5 in place rather than layering an `ALTER TABLE` on top.
- The same applies to the new proto schemas: edit them in place as message shapes settle instead of layering on deprecated fields — there are no external clients depending on them yet.
- Tasks 45 and 51 should be written as soon as their dependencies land, not deferred to the end — they're what prove the backstop actually works.
- Review happens once per phase, at the phase boundary — see "Working process" above. The phase checkpoints describe what should be true and demonstrable by the time that review happens.
- Phase 13's checkpoint calls out a phase that is intentionally not yet safe to expose broadly. It's the one phase in this breakdown where "reviewed and committed" doesn't mean "safe to deploy publicly" — flag this distinction if it ever needs to leave a dev/staging environment before phase 15 lands.
- `scripts/seed-dev-operator.sh` (`make seed-dev-operator`) seeds a local-only bootstrap operator user (`operator@theapp.com`) and grants it `superadmin` at system scope, so every `cmd/cli` command has a valid, usable `--operator` without a manual `psql` insert. The `superadmin` grant is a direct SQL insert rather than a CLI call, since phase 5 hasn't added a CLI command for it yet — switch the script to that command once it exists. It's a standing dev convenience, not a task with its own checkpoint — revisit it whenever a phase changes what a fresh local environment needs to be immediately useful (e.g. phase 9 might have it add `theapp` org membership) instead of leaving that as another manual step.
