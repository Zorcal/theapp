# Permissions and roles

## Concepts

A **permission** (capability) is a single named action on a resource — for example `user:read`, `user:create`, `user:update`, `user:delete`. Every protected endpoint declares the permission(s) required to call it. Without this system, all authenticated users have implicit access to every endpoint, which is incorrect as the surface grows.

A **role** is a named set of permissions. Roles are assigned to users; a user may hold multiple roles. The resolved permission set is the distinct union of all permissions across all of the user's roles.

## AuthUser

At request time, after validating the access token, the resolved permission set is attached to an `mdl.AuthUser` struct that is threaded through the call stack. Endpoints check permissions against this struct rather than re-querying the database per call.

```
AuthUser {
    UserID      uuid.UUID
    Permissions []Permission  // distinct, resolved from all assigned roles
}
```

`Permission` is a typed string constant; the full set is defined in `mdl/`.

## Standard roles

`superadmin` is the only built-in role and holds every permission in the system. Additional standard roles (e.g. `viewer`, `editor`) are defined as named constants and seeded at startup. Without at least one assigned role a user can authenticate but cannot call any protected endpoint.

## Enforcement

Permission checks happen in the gRPC interceptor layer, not inside business logic. Each RPC declares its required permission(s) via metadata or a registry map. The interceptor resolves `AuthUser` from the token, then rejects the call with `codes.PermissionDenied` if the required permissions are absent.

## Future: tenant-scoped roles

The current model is global — roles apply system-wide. When a project+organization concept is added, roles may carry a scope (tenant ID), and permission resolution will intersect the global set with the tenant-scoped set. The `AuthUser` struct is designed to accommodate a scope field without breaking existing callers; enforcement logic will be the only layer that changes.
