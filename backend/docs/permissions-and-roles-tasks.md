# Permissions and roles — implementation tasks

Breaks down docs/permissions-and-roles.md into ordered, independently-shippable tasks. Existing
code referenced for grounding: users/auth already exist (internal/core/mdl, internal/core/auth,
internal/core/pgstores/pguser, internal/api/grpc/unary_interceptors.go, schemas/auth.proto,
schemas/user.proto); organizations, projects, roles, and permissions don't exist yet, and there's
no proto schema for any of them. There's also no `cmd/admin` or urfave/cli dependency yet — only
`cmd/server`.

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

The admin CLI (`cmd/admin`) grows the same way, in its own dedicated phases rather than as tasks
folded into whichever feature phase happens to need a new command: phase 1 stands the tool up with
a `user create` command; phase 5 adds granting `superadmin`; phase 9 adds bootstrapping `theapp`/
`dev`/`control`; phase 26 adds seeding the `robot` user. Each CLI phase depends only on the scaffold
(phase 1) plus whatever core-layer function it wires up, never on another CLI phase.

## Phase 1 — CLI tool scaffold and user creation

Needs nothing new — users and auth already exist. Can be built and reviewed today.

1. `cmd/admin` CLI scaffold ([urfave/cli](https://github.com/urfave/cli), shell completion, commands split into their own files under a `commands` package), alongside the existing `cmd/server` entrypoint. Every mutating command takes a required `--operator` flag (resolved by UUID or email, the same way any user lookup is) for actor attribution — it has no observable effect until `pgdb` sets `app.user_id` from it (phase 19) and auditing exists to record it (phase 24), but building it into the scaffold now means no later command has to retrofit it.
2. `user create` command, wired to the existing user-creation core logic. Depends on 1.

**Checkpoint:** an operator can create a user via the CLI today.

## Phase 2 — permission types and table

3. `mdl.Permission` type and the full hardcoded constant list, in `internal/core/mdl`.
4. Migration: `permissions` table.

**Checkpoint:** the type compiles and the migration runs; the table exists (empty until phase 3 seeds it — there's no role yet to reference a permission by ID).

## Phase 3 — static roles and the permission sync

5. Migration: `roles` table (`is_static`; `org_id` deferred to phase 11, added once organizations exist) and `role_permissions` join table. Depends on 4.
6. `is_static` trigger on `roles` (`BEFORE UPDATE OR DELETE`). Depends on 5.
7. Permission sync at startup: reconcile `permissions` table against the code-defined list, stripping removed permissions from roles before deleting the row, and granting any newly added permission to `superadmin`. Depends on 3, 4, 5.
8. Static role definitions: hardcode `superadmin` (every permission, granted as real `role_permissions` rows kept in sync by 7) and other illustrative static roles (`useradmin`, `rolesadmin`), seeded at startup. Unit test asserting `superadmin`'s resolved permission set equals the full code-defined permission list. Depends on 5, 7.

**Checkpoint:** static roles exist in the DB with correct, sync-maintained permission sets, proven by the unit test in task 8.

## Phase 4 — system-scope assignment and resolver

9. Migration: `system_role_assignments` table (user, role — no project or org). Depends on 5.
10. Trigger on `system_role_assignments` rejecting inserts where the target role isn't static. Depends on 5, 9.
11. `mdl.AuthSession` / `mdl.AuthUser` structs, alongside existing `mdl/auth.go`.
12. System-scope-only resolver: permissions for a user from `system_role_assignments` alone (project/org legs added in phase 11). Depends on 8, 9, 11.
13. Core-layer function to insert a `system_role_assignments` row (no gRPC endpoint yet). Depends on 9, 10.

**Checkpoint:** given a manually-inserted (via task 13) `system_role_assignments` row, the resolver in task 12 returns the correct permission set for that user.

## Phase 5 — CLI: grant superadmin

14. CLI command granting `superadmin` to a user, wired to 13, using the scaffold from phase 1. Depends on 1, 13.

**Checkpoint:** an operator can grant `superadmin` to a user via the CLI.

## Phase 6 — permission registry and the superadmin gate

15. Permission registry: map of every existing RPC method to its real, correct required permissions (using the constants from 3), including explicit empty-list entries for endpoints that legitimately require none. Exhaustiveness test asserts every registered gRPC method has an entry. No new proto needed — this maps permissions onto RPCs that already exist. Depends on 3.
16. Interceptor: resolve `AuthSession` per-request via 12, enforce `codes.PermissionDenied` via the registry (15). Depends on 12, 15.

**Checkpoint:** every existing RPC is permission-gated and enforced. A user granted `superadmin` via phase 5's CLI command can call anything requiring a permission; every other authenticated user is denied on any endpoint with a non-empty required-permission list. Still no organizations, projects, custom roles, or self-service creation endpoint.

## Phase 7 — organizations and projects schema

17. Migration: `organizations`, `projects`, `org_project` mapping table.
18. Migration: `org_membership` table (or equivalent) gating org-scoped assignment. Depends on 17.

**Checkpoint:** the tables exist; org/project/membership rows can be created and joined correctly, proven directly at the DB level — no core logic yet.

## Phase 8 — org/project core creation functions

19. Core-layer `CreateOrganization` / `CreateProject` functions (no gRPC endpoint yet). Depends on 17.

**Checkpoint:** an org and a project can be created via these functions directly, proven by a test — independent of any CLI or gRPC surface.

## Phase 9 — CLI: bootstrap theapp/dev/control

20. CLI command to bootstrap `theapp`/`dev`/`control`, wired to 19, using the scaffold from phase 1 — idempotent, safe to re-run. Depends on 1, 19.

**Checkpoint:** an operator can bootstrap `theapp`/`dev`/`control` via the CLI.

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
37. Org creation gRPC endpoint: wires 19 behind `org:create` scoped to `theapp/control`, plus the `theapp` org-membership check. Depends on 25, 30, 36.
38. Org creation seeds a default project of the same name via 19's `CreateProject`. Depends on 37.
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
