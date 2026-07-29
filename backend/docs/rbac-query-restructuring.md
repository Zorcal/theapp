# RBAC query restructuring

## Purpose

The RBAC schema correctly enforces access, but many store queries independently reconstruct the
same relationships between users, memberships, projects, roles, and permissions. This work makes
those queries smaller and harder to diverge while strengthening tenant isolation in the schema.
It is a refactor of the existing authorization model, not a change to which permissions a user
receives.

The implementation should retain the current separate tables for system roles, custom roles, and
the three assignment scopes. Those distinctions express real security boundaries and make invalid
states harder to represent.

## Decisions

### Keep integer permission identities

`rbac.permissions.id` remains the primary key used by role-permission relations. Permission names
are mutable product vocabulary and should not become relational identities.

A small `STABLE` SQL lookup function will translate a set of permission names into rows containing
permission IDs and names. Permission write paths can use it once to validate input and obtain IDs,
instead of each query spelling its own join and validation CTE. Queries returning permissions
continue to join by integer ID so they can return the current name. The lookup is set-oriented; it
must not be called once per result row.

### Put the organization boundary into assignment keys

`project_role_assignments` will gain `org_id`; `org_role_assignments` already carries it.
Projects and custom roles will expose composite unique keys on `(id, org_id)`. The assignment
tables will use composite foreign keys to establish all of the following:

- the assigned user is a member of the target organization;
- the custom role belongs to the target organization;
- for a project assignment, the project belongs to the target organization.

The application supplies `org_id` after resolving the target. A trigger does not need to derive
it, and the existing application checks remain useful for domain-specific errors.

All foreign keys retain PostgreSQL's default `NO ACTION` behavior. Deletion flows must explicitly
remove dependent rows in the intended order and transaction. The database should reject incomplete
cleanup; it must never perform broad cleanup through `ON DELETE CASCADE`.

### Centralize effective-access relations

Small `LANGUAGE sql`, `STABLE`, `SECURITY INVOKER` functions will provide the canonical relations
for:

- system permission IDs for a user;
- organization permission IDs for a user and organization;
- project permission IDs for a user and project;
- project IDs accessible to a user.

They accept internal integer IDs where possible and return sets of IDs. Scope-specific functions
compose narrower functions with `UNION`, preserving additive permission semantics and
deduplication. Authentication, privilege checks, accessible-project listing and counting, and
future database authorization should all consume these relations instead of maintaining separate
copies of the same joins.

The functions remain security-invoker helpers. A later `SECURITY DEFINER` operation may call them,
but must still perform its own authorization check before returning cross-user data.

### Prefer constraints over trigger-maintained authorization state

Composite keys and foreign keys are sufficient for assignment tenant integrity. This work does not
add trigger-maintained effective-permission or accessible-project tables. Derived authorization
state remains live so role edits, assignment changes, membership removal, and permission changes
take effect immediately without cache invalidation logic.

Indexes will follow the query directions and foreign keys introduced by the final schema. User-first
paths serve permission resolution; target-first paths serve organization/project listings and
explicit deletion cleanup. This work will avoid redundant indexes whose leading columns are already
covered by a primary key or another index.

## Explicit non-goals

- Do not combine a paginated list and its total count into one query. Their shared access relation
  will be centralized, but the list and count operations remain separate until that design is
  considered independently.
- Do not add performance tests or benchmarks as part of this work.
- Do not replace integer permission IDs with permission names as keys.
- Do not merge system and custom roles, or the three assignment scopes, into generic tables.
- Do not use `ON DELETE CASCADE`.
- Do not materialize or cache effective authorization data.

## Implementation sequence

### 1. Strengthen assignment structure

1. Add the composite project and custom-role keys required by assignment foreign keys.
2. Add `org_id` to project assignments and define composite membership, role ownership, and target
   ownership foreign keys for both custom-role assignment scopes.
3. Adjust primary keys and indexes for user-first resolution, target-first discovery, foreign-key
   checks, and explicit cleanup.
4. Update assignment writes and test seed helpers to supply the organization ID.
5. Cover non-member and cross-organization behavior through the supported store/core APIs.
   Successful assignment tests exercise the foreign keys; do not bypass the store merely to force
   a constraint error that supported call sequences cannot produce.

### 2. Add permission-name lookup

1. Add the set-returning permission lookup function.
2. Replace repeated permission-name validation and ID lookup in custom-role writes.
3. Keep ordinary read joins where a permission name is part of the returned resource.

### 3. Add canonical effective-access functions

1. Add system-, organization-, and project-scope permission relations.
2. Add the accessible-project relation, including the system-only `project:discover-all` rule.
3. Refactor permission resolution, privilege checks, and project list/count queries onto the
   canonical relations.
4. Keep list filtering, stable ordering, pagination, and total counting in their existing
   operations.

### 4. Align later roadmap work

- User soft deletion must be applied inside the canonical access functions so every consumer
  excludes deleted users consistently.
- Project- and organization-first indexes needed by organization user listing and deletion should
  be selected against the final composite keys rather than added speculatively beforehand.
- Assignment-table RLS must account for the security-invoker functions and transaction-local
  settings.
- The future cross-user `SECURITY DEFINER` listing function should reuse the canonical project
  permission relation for its internal authorization check.
- Organization and project deletion remains explicit, ordered cleanup protected by `NO ACTION`
  foreign keys.

## Validation

Each implementation slice uses the repository-wide formatter, test suite, linter, and diff checks.
Coverage should demonstrate the same happy paths and security boundaries through observable
results. This restructuring does not introduce query timing assertions, benchmarks, or other
performance-testing infrastructure.
