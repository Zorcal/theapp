// Package rbac provides the core business logic for the permissions and roles domain.
package rbac

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgorg"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
)

//go:generate moq -rm -fmt goimports -out role_storer_moq_test.go . RoleStorer:MockedRoleStorer

// RoleStorer defines the database operations the Core requires.
type RoleStorer interface {
	// StaticRoles returns every static role and the names of the permissions currently granted to
	// it.
	StaticRoles(ctx context.Context) ([]pgrbac.RoleStatic, error)
	// OrgRoles returns orgID's own custom roles.
	OrgRoles(ctx context.Context, orgID int) ([]pgrbac.RoleCustom, error)
	// RoleByExternalID returns the custom role with the given external ID.
	// Returns [sql.ErrNoRows] if no such role exists.
	RoleByExternalID(ctx context.Context, externalID uuid.UUID) (pgrbac.RoleCustom, error)
	// CreateRole inserts a new custom role and returns it.
	CreateRole(ctx context.Context, cr pgrbac.CreateRole) (pgrbac.RoleCustom, error)
	// UpdateRole replaces the name and permission set of the custom role identified by ur.ID and
	// owned by ur.OrgID, and returns the updated role.
	// Returns [sql.ErrNoRows] if no such role, owned by ur.OrgID, exists.
	UpdateRole(ctx context.Context, ur pgrbac.UpdateRole) (pgrbac.RoleCustom, error)
	// DeleteRole deletes the custom role identified by id and owned by orgID.
	// Returns [sql.ErrNoRows] if no such role, owned by orgID, exists.
	DeleteRole(ctx context.Context, id, orgID int) error
	// AssignSystemRole grants userID the static role named roleName at system scope.
	// Returns [sql.ErrNoRows] if no static role named roleName exists.
	AssignSystemRole(ctx context.Context, userID int, roleName string) error
	// AssignProjectRole grants userID roleID for projectID. A no-op if the grant already exists.
	AssignProjectRole(ctx context.Context, userID, roleID, projectID int) error
	// AssignOrgRole grants userID roleID for orgID. A no-op if the grant already exists.
	AssignOrgRole(ctx context.Context, userID, roleID, orgID int) error
	// UnassignProjectRole revokes userID's roleID grant for projectID. A no-op if no such grant
	// exists.
	UnassignProjectRole(ctx context.Context, userID, roleID, projectID int) error
	// UnassignOrgRole revokes userID's roleID grant for orgID. A no-op if no such grant exists.
	UnassignOrgRole(ctx context.Context, userID, roleID, orgID int) error
	// OrgAssignmentExists reports whether userID already holds roleID at orgID's org scope.
	OrgAssignmentExists(ctx context.Context, userID, roleID, orgID int) (bool, error)
	// DeleteProjectAssignmentsForOrg deletes every project-scope assignment of roleID held by
	// userID across every project under orgID.
	DeleteProjectAssignmentsForOrg(ctx context.Context, userID, roleID, orgID int) error
	// RoleAssignmentsForUser returns every custom role userID holds within orgID: project-scope
	// rows for projects under orgID, unioned with the org-scope row for orgID itself.
	RoleAssignmentsForUser(ctx context.Context, userID, orgID int) ([]pgrbac.RoleAssignment, error)
	// StaticRoleByName returns the static role named name.
	// Returns [sql.ErrNoRows] if no such static role exists.
	StaticRoleByName(ctx context.Context, name string) (pgrbac.RoleStatic, error)
	// SystemRoleAssignmentsForUser returns the names of every static role userID holds at system
	// scope.
	SystemRoleAssignmentsForUser(ctx context.Context, userID int) ([]string, error)
}

//go:generate moq -rm -fmt goimports -out org_storer_moq_test.go . OrgStorer:MockedOrgStorer

// OrgStorer defines the organization/project database operations the Core requires.
type OrgStorer interface {
	// ProjectByID returns the project with the given ID.
	// Returns [sql.ErrNoRows] if no such project exists.
	ProjectByID(ctx context.Context, id int) (pgorg.Project, error)
	// IsOrgMember reports whether userID is a member of orgID.
	IsOrgMember(ctx context.Context, userID, orgID int) (bool, error)
}

//go:generate moq -rm -fmt goimports -out user_storer_moq_test.go . UserStorer:MockedUserStorer

// UserStorer defines the user database operations the Core requires.
type UserStorer interface {
	// UserByExternalID returns the user with the given external ID.
	// Returns [sql.ErrNoRows] if no such user exists.
	UserByExternalID(ctx context.Context, id uuid.UUID) (pguser.User, error)
}

// Transactor runs a function inside a database transaction.
type Transactor interface {
	RunTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Core holds the business logic for the permissions and roles domain.
type Core struct {
	roleStorer RoleStorer
	userStorer UserStorer
	orgStorer  OrgStorer
	transactor Transactor
}

// NewCore constructs a Core backed by the provided RoleStorer, UserStorer, OrgStorer, and
// Transactor.
func NewCore(rs RoleStorer, us UserStorer, os OrgStorer, tr Transactor) *Core {
	return &Core{roleStorer: rs, userStorer: us, orgStorer: os, transactor: tr}
}

// StaticRoles returns every static role and the permissions currently granted to it.
func (c *Core) StaticRoles(ctx context.Context) ([]mdl.RoleStatic, error) {
	rs, err := c.roleStorer.StaticRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("static roles: %w", err)
	}

	return staticRolesFromPg(rs), nil
}

// StaticRolePermissions returns a page of the permissions granted to the static role named
// roleName, and the total count across all pages. Pagination is applied here rather than in the
// store, the same reasoning as OrgRoles: the number of permissions a role can hold is small and
// bounded.
// Returns [mdl.ErrNotFound] if no static role named roleName exists.
func (c *Core) StaticRolePermissions(ctx context.Context, roleName string, pageSize, pageOffset int) ([]mdl.Permission, int, error) {
	role, err := c.roleStorer.StaticRoleByName(ctx, roleName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, 0, mdl.ErrNotFound
		}
		return nil, 0, fmt.Errorf("static role by name: %w", err)
	}

	permissions := permissionsFromPg(role.PermissionNames)
	totalCount := len(permissions)

	end := min(pageOffset+pageSize, totalCount)
	var page []mdl.Permission
	if pageOffset < totalCount {
		page = permissions[pageOffset:end]
	}

	return page, totalCount, nil
}

// OrgRoles returns a page of orgID's own custom roles and the total count across all pages.
// Pagination is applied here rather than in the store: the number of custom roles an org can
// define is small and bounded, so a paginated query at the database layer isn't worth the added
// complexity — this keeps that an implementation detail callers don't need to know about, so it
// can move to the store later without changing this signature.
func (c *Core) OrgRoles(ctx context.Context, orgID, pageSize, pageOffset int) ([]mdl.RoleCustom, int, error) {
	rs, err := c.roleStorer.OrgRoles(ctx, orgID)
	if err != nil {
		return nil, 0, fmt.Errorf("org roles: %w", err)
	}

	roles := rolesCustomFromPg(rs)
	totalCount := len(roles)

	end := min(pageOffset+pageSize, totalCount)
	var page []mdl.RoleCustom
	if pageOffset < totalCount {
		page = roles[pageOffset:end]
	}

	return page, totalCount, nil
}

// CreateRole creates a new custom role owned by orgID and returns it.
// Returns [mdl.ErrValidation] if cr is invalid.
func (c *Core) CreateRole(ctx context.Context, orgID int, cr mdl.CreateRole) (mdl.RoleCustom, error) {
	if err := cr.Validate(); err != nil {
		return mdl.RoleCustom{}, fmt.Errorf("validate: %w", err)
	}

	role, err := c.roleStorer.CreateRole(ctx, createRoleToPg(orgID, cr))
	if err != nil {
		return mdl.RoleCustom{}, fmt.Errorf("create role: %w", err)
	}

	return roleCustomFromPg(role), nil
}

// UpdateRole applies ur.Fields to the custom role identified by ur.ID, owned by orgID, and
// returns the updated role. A field not listed in ur.Fields is left at its current value — the
// store always writes a full name and permission set, so an unset field is filled in from the
// role's existing value before that write, rather than the store itself supporting a partial
// update. Permissions, when applied, replaces the role's entire permission set; see
// ModifyRolePermissions for an additive alternative.
// Returns [mdl.ErrNotFound] if no role with that ID, owned by orgID, exists.
// Returns [mdl.ErrValidation] if ur is invalid.
func (c *Core) UpdateRole(ctx context.Context, orgID int, ur mdl.UpdateRole) (mdl.RoleCustom, error) {
	if err := ur.Validate(); err != nil {
		return mdl.RoleCustom{}, fmt.Errorf("validate: %w", err)
	}

	var role pgrbac.RoleCustom
	if err := c.transactor.RunTx(ctx, func(ctx context.Context) error {
		existing, err := c.resolveOwnedCustomRole(ctx, orgID, ur.ID)
		if err != nil {
			return err
		}

		name := existing.Name
		if ur.Fields.Name {
			name = ur.Name
		}
		permissionNames := existing.PermissionNames
		if ur.Fields.Permissions {
			permissionNames = permissionNamesFromMdl(ur.Permissions)
		}

		role, err = c.roleStorer.UpdateRole(ctx, pgrbac.UpdateRole{
			ID:              existing.ID,
			OrgID:           orgID,
			Name:            name,
			PermissionNames: permissionNames,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return mdl.ErrNotFound
			}
			return fmt.Errorf("update role: %w", err)
		}

		return nil
	}); err != nil {
		return mdl.RoleCustom{}, err
	}

	return roleCustomFromPg(role), nil
}

// ModifyRolePermissions adds and/or removes permissions from the custom role identified by
// m.ID, owned by orgID, and returns the updated role. Unlike UpdateRole's Permissions field, this
// computes the resulting set additively — (existing ∪ m.AddPermissions) \ m.RemovePermissions —
// rather than replacing it wholesale, so a concurrent edit to a different permission on the same
// role isn't silently dropped.
// Returns [mdl.ErrNotFound] if no role with that ID, owned by orgID, exists.
// Returns [mdl.ErrValidation] if m is invalid, or if the result would leave the role with no
// permissions at all.
func (c *Core) ModifyRolePermissions(ctx context.Context, orgID int, m mdl.ModifyRolePermissions) (mdl.RoleCustom, error) {
	if err := m.Validate(); err != nil {
		return mdl.RoleCustom{}, fmt.Errorf("validate: %w", err)
	}

	var role pgrbac.RoleCustom
	if err := c.transactor.RunTx(ctx, func(ctx context.Context) error {
		existing, err := c.resolveOwnedCustomRole(ctx, orgID, m.ID)
		if err != nil {
			return err
		}

		permissionNames := mergePermissionNames(existing.PermissionNames, m.AddPermissions, m.RemovePermissions)
		if len(permissionNames) == 0 {
			return fmt.Errorf("remove_permissions leaves no permissions: %w", mdl.ErrValidation)
		}

		role, err = c.roleStorer.UpdateRole(ctx, pgrbac.UpdateRole{
			ID:              existing.ID,
			OrgID:           orgID,
			Name:            existing.Name,
			PermissionNames: permissionNames,
		})
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return mdl.ErrNotFound
			}
			return fmt.Errorf("update role: %w", err)
		}

		return nil
	}); err != nil {
		return mdl.RoleCustom{}, err
	}

	return roleCustomFromPg(role), nil
}

// DeleteRole deletes the custom role identified by id, owned by orgID.
// Returns [mdl.ErrNotFound] if no role with that ID, owned by orgID, exists.
func (c *Core) DeleteRole(ctx context.Context, orgID int, id uuid.UUID) error {
	return c.transactor.RunTx(ctx, func(ctx context.Context) error {
		existing, err := c.resolveOwnedCustomRole(ctx, orgID, id)
		if err != nil {
			return err
		}

		if err := c.roleStorer.DeleteRole(ctx, existing.ID, orgID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return mdl.ErrNotFound
			}
			return fmt.Errorf("delete role: %w", err)
		}

		return nil
	})
}

// resolveOwnedCustomRole resolves the custom role identified by externalID and checks it's owned
// by orgID — the shared precondition for UpdateRole, ModifyRolePermissions, DeleteRole,
// AssignRole, UnassignRole, and RolePermissions.
//
// A static role's external ID never matches here — static roles live in a separate table, so
// RoleByExternalID simply can't find one — so a caller who passes a static role's ID gets the
// same [mdl.ErrNotFound] as a nonexistent or cross-org ID, not a distinct "that's a static role"
// error.
// Returns [mdl.ErrNotFound] if no such role exists, or if it exists but isn't owned by orgID.
func (c *Core) resolveOwnedCustomRole(ctx context.Context, orgID int, externalID uuid.UUID) (pgrbac.RoleCustom, error) {
	existing, err := c.roleStorer.RoleByExternalID(ctx, externalID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return pgrbac.RoleCustom{}, mdl.ErrNotFound
		}
		return pgrbac.RoleCustom{}, fmt.Errorf("role by external id: %w", err)
	}
	if existing.OrgID != orgID {
		return pgrbac.RoleCustom{}, mdl.ErrNotFound
	}
	return existing, nil
}

// AssignSystemRole grants userID the static role named roleName at system scope.
// Returns [mdl.ErrNotFound] if no user with that ID, or no static role named roleName, exists.
func (c *Core) AssignSystemRole(ctx context.Context, userID uuid.UUID, roleName string) error {
	u, err := c.userStorer.UserByExternalID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.ErrNotFound
		}
		return fmt.Errorf("user by external id: %w", err)
	}

	if err := c.roleStorer.AssignSystemRole(ctx, u.ID, roleName); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.ErrNotFound
		}
		return fmt.Errorf("assign system role: %w", err)
	}

	return nil
}

// SystemRoleAssignments returns a page of the names of every static role userID holds at system
// scope, and the total count across all pages. Pagination is applied here rather than in the
// store, the same reasoning as OrgRoles: a user's system-scope assignment count is small and
// bounded.
// Returns [mdl.ErrNotFound] if no user with that ID exists.
func (c *Core) SystemRoleAssignments(ctx context.Context, userID uuid.UUID, pageSize, pageOffset int) ([]string, int, error) {
	u, err := c.userStorer.UserByExternalID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, 0, mdl.ErrNotFound
		}
		return nil, 0, fmt.Errorf("user by external id: %w", err)
	}

	names, err := c.roleStorer.SystemRoleAssignmentsForUser(ctx, u.ID)
	if err != nil {
		return nil, 0, fmt.Errorf("system role assignments for user: %w", err)
	}

	totalCount := len(names)

	end := min(pageOffset+pageSize, totalCount)
	var page []string
	if pageOffset < totalCount {
		page = names[pageOffset:end]
	}

	return page, totalCount, nil
}

// AssignRole assigns in.RoleID, owned by orgID, to in.UserID at in.Scope.
//
// At project scope, if in.UserID already holds in.RoleID at in.Scope's project's org scope, the
// assignment is rejected outright: there's no legitimate reason to add a narrower grant when a
// broader one already covers it.
//
// At org scope, any existing project-scope assignments of in.RoleID held by in.UserID within that
// org are deleted in the same transaction as the new org-scope row is inserted — org scope already
// implies the role at every project under it, so those narrower grants become redundant. This
// atomic promotion avoids the window a separate "unassign at project scope, then assign at org
// scope" sequence would leave, where the user briefly holds neither.
//
// Returns [mdl.ErrValidation] if in is invalid.
// Returns [mdl.ErrNotFound] if no role with that ID, owned by orgID, exists; if no user with that
// ID exists; or if in.Scope's project/org doesn't belong to orgID.
// Returns [mdl.ErrRoleScopeConflict] if in.Scope is project-scoped and in.UserID already holds
// in.RoleID at the project's org scope.
// Returns [mdl.ErrNotOrgMember] if in.Scope is org-scoped and in.UserID isn't a member of that org.
func (c *Core) AssignRole(ctx context.Context, orgID int, in mdl.AssignRole) error {
	if err := in.Validate(); err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	return c.transactor.RunTx(ctx, func(ctx context.Context) error {
		role, err := c.resolveOwnedCustomRole(ctx, orgID, in.RoleID)
		if err != nil {
			return err
		}

		u, err := c.userStorer.UserByExternalID(ctx, in.UserID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return mdl.ErrNotFound
			}
			return fmt.Errorf("user by external id: %w", err)
		}

		if in.Scope.ProjectID != nil {
			project, err := c.orgStorer.ProjectByID(ctx, *in.Scope.ProjectID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return mdl.ErrNotFound
				}
				return fmt.Errorf("project by id: %w", err)
			}
			if project.OrgID != orgID {
				return mdl.ErrNotFound
			}

			exists, err := c.roleStorer.OrgAssignmentExists(ctx, u.ID, role.ID, orgID)
			if err != nil {
				return fmt.Errorf("org assignment exists: %w", err)
			}
			if exists {
				return mdl.ErrRoleScopeConflict
			}

			if err := c.roleStorer.AssignProjectRole(ctx, u.ID, role.ID, project.ID); err != nil {
				return fmt.Errorf("assign project role: %w", err)
			}
			return nil
		}

		if *in.Scope.OrgID != orgID {
			return mdl.ErrNotFound
		}

		isMember, err := c.orgStorer.IsOrgMember(ctx, u.ID, orgID)
		if err != nil {
			return fmt.Errorf("is org member: %w", err)
		}
		if !isMember {
			return mdl.ErrNotOrgMember
		}

		if err := c.roleStorer.DeleteProjectAssignmentsForOrg(ctx, u.ID, role.ID, orgID); err != nil {
			return fmt.Errorf("delete project assignments for org: %w", err)
		}
		if err := c.roleStorer.AssignOrgRole(ctx, u.ID, role.ID, orgID); err != nil {
			return fmt.Errorf("assign org role: %w", err)
		}
		return nil
	})
}

// UnassignRole unassigns in.RoleID, owned by orgID, from in.UserID at in.Scope.
// Returns [mdl.ErrValidation] if in is invalid.
// Returns [mdl.ErrNotFound] if no role with that ID, owned by orgID, exists; if no user with that
// ID exists; or if in.Scope's project/org doesn't belong to orgID.
func (c *Core) UnassignRole(ctx context.Context, orgID int, in mdl.UnassignRole) error {
	if err := in.Validate(); err != nil {
		return fmt.Errorf("validate: %w", err)
	}

	return c.transactor.RunTx(ctx, func(ctx context.Context) error {
		role, err := c.resolveOwnedCustomRole(ctx, orgID, in.RoleID)
		if err != nil {
			return err
		}

		u, err := c.userStorer.UserByExternalID(ctx, in.UserID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return mdl.ErrNotFound
			}
			return fmt.Errorf("user by external id: %w", err)
		}

		if in.Scope.ProjectID != nil {
			project, err := c.orgStorer.ProjectByID(ctx, *in.Scope.ProjectID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return mdl.ErrNotFound
				}
				return fmt.Errorf("project by id: %w", err)
			}
			if project.OrgID != orgID {
				return mdl.ErrNotFound
			}

			if err := c.roleStorer.UnassignProjectRole(ctx, u.ID, role.ID, project.ID); err != nil {
				return fmt.Errorf("unassign project role: %w", err)
			}
			return nil
		}

		if *in.Scope.OrgID != orgID {
			return mdl.ErrNotFound
		}

		if err := c.roleStorer.UnassignOrgRole(ctx, u.ID, role.ID, orgID); err != nil {
			return fmt.Errorf("unassign org role: %w", err)
		}
		return nil
	})
}

// ListRoleAssignments returns a page of every role userID holds within orgID and the total count
// across all pages. Pagination is applied here rather than in the store, the same reasoning as
// OrgRoles: a user's assignment count within a single org is small and bounded.
// Returns [mdl.ErrNotFound] if no user with that ID exists.
func (c *Core) ListRoleAssignments(ctx context.Context, orgID int, userID uuid.UUID, pageSize, pageOffset int) ([]mdl.RoleAssignment, int, error) {
	u, err := c.userStorer.UserByExternalID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, 0, mdl.ErrNotFound
		}
		return nil, 0, fmt.Errorf("user by external id: %w", err)
	}

	rows, err := c.roleStorer.RoleAssignmentsForUser(ctx, u.ID, orgID)
	if err != nil {
		return nil, 0, fmt.Errorf("role assignments for user: %w", err)
	}

	assignments := roleAssignmentsFromPg(rows)
	totalCount := len(assignments)

	end := min(pageOffset+pageSize, totalCount)
	var page []mdl.RoleAssignment
	if pageOffset < totalCount {
		page = assignments[pageOffset:end]
	}

	return page, totalCount, nil
}

// RolePermissions returns a page of the permissions granted to the custom role identified by
// roleID, owned by orgID, and the total count across all pages. Pagination is applied here rather
// than in the store, the same reasoning as OrgRoles: the number of permissions a role can hold is
// small and bounded.
// Returns [mdl.ErrNotFound] if no role with that ID, owned by orgID, exists.
func (c *Core) RolePermissions(ctx context.Context, orgID int, roleID uuid.UUID, pageSize, pageOffset int) ([]mdl.Permission, int, error) {
	existing, err := c.resolveOwnedCustomRole(ctx, orgID, roleID)
	if err != nil {
		return nil, 0, err
	}

	permissions := permissionsFromPg(existing.PermissionNames)
	totalCount := len(permissions)

	end := min(pageOffset+pageSize, totalCount)
	var page []mdl.Permission
	if pageOffset < totalCount {
		page = permissions[pageOffset:end]
	}

	return page, totalCount, nil
}
