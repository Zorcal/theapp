package rbac

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
	"github.com/zorcal/theapp/backend/pkg/set"
)

// CustomRoles returns a page of custom roles owned by the caller's organization, along with the total count.
func (c *Core) CustomRoles(ctx context.Context, pageSize, pageOffset int) ([]mdl.CustomRole, int, error) {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return nil, 0, errors.New("auth session missing")
	}
	if sess.OrgID == nil {
		return nil, 0, errors.New("organization context missing")
	}

	rs, count, err := c.roleStorer.CustomRoles(ctx, *sess.OrgID, pageSize, pageOffset)
	if err != nil {
		return nil, 0, fmt.Errorf("custom roles: %w", err)
	}

	return customRolesFromPg(rs), count, nil
}

// UserProjectCustomRoles returns a page of custom roles assigned directly to userID in the caller's project.
// Returns [mdl.ErrNotFound] if the user, project, or organization membership does not exist.
func (c *Core) UserProjectCustomRoles(ctx context.Context, userID uuid.UUID, pageSize, pageOffset int) ([]mdl.CustomRole, int, error) {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return nil, 0, errors.New("auth session missing")
	}
	if sess.ProjectID == nil {
		return nil, 0, errors.New("project context missing")
	}

	roles, count, err := c.roleStorer.UserProjectCustomRoles(ctx, userID, *sess.ProjectID, pageSize, pageOffset)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, 0, mdl.ErrNotFound
		}
		return nil, 0, fmt.Errorf("user project custom roles: %w", err)
	}

	return customRolesFromPg(roles), count, nil
}

// UserOrgCustomRoles returns a page of custom roles assigned to userID across the caller's organization.
// Returns [mdl.ErrNotFound] if the user or organization membership does not exist.
func (c *Core) UserOrgCustomRoles(ctx context.Context, userID uuid.UUID, pageSize, pageOffset int) ([]mdl.CustomRole, int, error) {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return nil, 0, errors.New("auth session missing")
	}
	if sess.OrgID == nil {
		return nil, 0, errors.New("organization context missing")
	}

	roles, count, err := c.roleStorer.UserOrgCustomRoles(ctx, userID, *sess.OrgID, pageSize, pageOffset)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, 0, mdl.ErrNotFound
		}
		return nil, 0, fmt.Errorf("user organization custom roles: %w", err)
	}

	return customRolesFromPg(roles), count, nil
}

// CustomRoleByID returns a custom role owned by the caller's organization.
// Returns [mdl.ErrNotFound] if the role does not exist or is owned by another organization.
func (c *Core) CustomRoleByID(ctx context.Context, roleID uuid.UUID) (mdl.CustomRole, error) {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return mdl.CustomRole{}, errors.New("auth session missing")
	}
	if sess.OrgID == nil {
		return mdl.CustomRole{}, errors.New("organization context missing")
	}

	role, err := c.roleStorer.CustomRoleByExternalID(ctx, *sess.OrgID, roleID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.CustomRole{}, mdl.ErrNotFound
		}
		return mdl.CustomRole{}, fmt.Errorf("custom role: %w", err)
	}

	return customRoleFromPg(role), nil
}

// CreateCustomRole creates a custom role in the caller's organization.
// Returns [mdl.ErrValidation] if the input is invalid or contains a system-only permission.
// Returns [mdl.ErrNotFound] if the authenticated actor or selected project no longer exists.
// Returns [mdl.ErrAlreadyExists] if the organization already has a role with that name.
// Returns [mdl.ErrPermissionDenied] if the caller does not hold every permission added to the role at the project scope.
func (c *Core) CreateCustomRole(ctx context.Context, cr mdl.CreateCustomRole) (mdl.CustomRole, error) {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return mdl.CustomRole{}, errors.New("auth session missing")
	}
	if sess.OrgID == nil {
		return mdl.CustomRole{}, errors.New("organization context missing")
	}

	if err := cr.Validate(); err != nil {
		return mdl.CustomRole{}, fmt.Errorf("validate: %w", err)
	}
	if sess.ProjectID == nil {
		return mdl.CustomRole{}, errors.New("project context missing")
	}

	var role pgrbac.CustomRole
	if err := c.transactor.RunTx(ctx, func(ctx context.Context) error {
		userOrgPerms, err := c.roleStorer.OrgPermissionsByProjectID(ctx, sess.User.UserID, *sess.ProjectID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("get actor organization permissions: %w", mdl.ErrNotFound)
			}
			return fmt.Errorf("get actor organization permissions: %w", err)
		}

		if !mdl.IsPermissionSuperset(permissionsFromPg(userOrgPerms.PermissionNames), cr.Permissions) {
			return mdl.ErrPermissionDenied
		}

		role, err = c.roleStorer.CreateCustomRole(ctx, createCustomRoleToPg(cr, userOrgPerms.OrgID))
		if err != nil {
			switch {
			case errors.Is(err, pgdb.ErrAlreadyExists):
				return fmt.Errorf("create custom role: %w", mdl.ErrAlreadyExists)
			case errors.Is(err, pgdb.ErrCheckConstraintViolated):
				return fmt.Errorf("create custom role: %w", mdl.ErrValidation)
			default:
				// sql.ErrNoRows can also indicate a missing application-known permission, so it
				// cannot safely be remapped to mdl.ErrNotFound.
				return fmt.Errorf("create custom role: %w", err)
			}
		}

		return nil
	}); err != nil {
		return mdl.CustomRole{}, fmt.Errorf("run tx: %w", err)
	}

	return customRoleFromPg(role), nil
}

// UpdateCustomRole updates selected fields on a custom role in the caller's organization.
// Returns [mdl.ErrNotFound] if the caller or selected project no longer exists, or the role is not
// owned by the caller's organization.
// Returns [mdl.ErrValidation] if the input is invalid or contains a system-only permission.
// Returns [mdl.ErrAlreadyExists] if the organization already has a role with that name.
// Returns [mdl.ErrPermissionDenied] if the caller does not hold every permission added to or
// removed from the role at the org scope.
// Returns [mdl.ErrInvalidAssignmentScope] if the update would make a role with project assignments
// require organization scope.
func (c *Core) UpdateCustomRole(ctx context.Context, ur mdl.UpdateCustomRole) (mdl.CustomRole, error) {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return mdl.CustomRole{}, errors.New("auth session missing")
	}
	if sess.OrgID == nil {
		return mdl.CustomRole{}, errors.New("organization context missing")
	}

	if err := ur.Validate(); err != nil {
		return mdl.CustomRole{}, fmt.Errorf("validate: %w", err)
	}

	if ur.Fields.Permissions {
		if sess.ProjectID == nil {
			return mdl.CustomRole{}, errors.New("project context missing")
		}

		var role pgrbac.CustomRole
		if err := c.transactor.RunTx(ctx, func(ctx context.Context) error {
			userOrgPerms, currentRole, err := c.customRolePermChangeContext(ctx, sess.User.UserID, *sess.ProjectID, ur.ID)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					return fmt.Errorf("get permission change context: %w", mdl.ErrNotFound)
				}
				return fmt.Errorf("get permission change context: %w", err)
			}

			if currentRole.ManagedKey != nil {
				return mdl.ErrManagedRole
			}

			changedPerms := changedPerms(permissionsFromPg(currentRole.PermissionNames), ur.Permissions)
			if !mdl.IsPermissionSuperset(permissionsFromPg(userOrgPerms.PermissionNames), changedPerms) {
				return mdl.ErrPermissionDenied
			}

			if err := c.validateCustomRoleAssignmentScope(ctx, ur.ID, ur.Permissions); err != nil {
				return fmt.Errorf("validate custom role assignment scope: %w", err)
			}

			role, err = c.roleStorer.UpdateCustomRole(ctx, updateCustomRoleToPg(ur, userOrgPerms.OrgID))
			if err != nil {
				switch {
				case errors.Is(err, pgdb.ErrAlreadyExists):
					return fmt.Errorf("update custom role with permission changes: %w", mdl.ErrAlreadyExists)
				case errors.Is(err, pgdb.ErrCheckConstraintViolated):
					return fmt.Errorf("update custom role with permission changes: %w", mdl.ErrValidation)
				default:
					// The role and requested permissions were resolved earlier in this transaction,
					// so sql.ErrNoRows is an impossible state and must remain an internal error.
					return fmt.Errorf("update custom role with permission changes: %w", err)
				}
			}

			return nil
		}); err != nil {
			return mdl.CustomRole{}, fmt.Errorf("run tx: %w", err)
		}

		return customRoleFromPg(role), nil
	}

	role, err := c.roleStorer.UpdateCustomRole(ctx, updateCustomRoleToPg(ur, *sess.OrgID))
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return mdl.CustomRole{}, fmt.Errorf("update custom role: %w", mdl.ErrNotFound)
		case errors.Is(err, pgdb.ErrAlreadyExists):
			return mdl.CustomRole{}, fmt.Errorf("update custom role: %w", mdl.ErrAlreadyExists)
		case errors.Is(err, pgdb.ErrCheckConstraintViolated):
			return mdl.CustomRole{}, fmt.Errorf("update custom role: %w", mdl.ErrValidation)
		default:
			return mdl.CustomRole{}, fmt.Errorf("update custom role: %w", err)
		}
	}

	return customRoleFromPg(role), nil
}

// ModifyCustomRolePermissions atomically changes permissions on a custom role in the caller's organization.
// Returns [mdl.ErrNotFound] if the caller or selected project no longer exists, or the role is not
// owned by the caller's organization.
// Returns [mdl.ErrValidation] if the input contains overlapping or system-only permissions.
// Returns [mdl.ErrPermissionDenied] if the caller does not hold every permission added to or
// removed from the role at the org scope.
// Returns [mdl.ErrInvalidAssignmentScope] if the change would make a role with project assignments
// require organization scope.
func (c *Core) ModifyCustomRolePermissions(ctx context.Context, mrp mdl.ModifyCustomRolePermissions) (mdl.CustomRole, error) {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return mdl.CustomRole{}, errors.New("auth session missing")
	}
	if sess.OrgID == nil {
		return mdl.CustomRole{}, errors.New("organization context missing")
	}

	if err := mrp.Validate(); err != nil {
		return mdl.CustomRole{}, fmt.Errorf("validate: %w", err)
	}
	if sess.ProjectID == nil {
		return mdl.CustomRole{}, errors.New("project context missing")
	}

	var role pgrbac.CustomRole
	if err := c.transactor.RunTx(ctx, func(ctx context.Context) error {
		userOrgPerms, currentRole, err := c.customRolePermChangeContext(ctx, sess.User.UserID, *sess.ProjectID, mrp.ID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("get permission change context: %w", mdl.ErrNotFound)
			}
			return fmt.Errorf("get permission change context: %w", err)
		}
		if currentRole.ManagedKey != nil {
			return mdl.ErrManagedRole
		}

		nextPerms := set.FromSlice(permissionsFromPg(currentRole.PermissionNames)).
			Add(mrp.AddPermissions...).
			Remove(mrp.RemovePermissions...)

		changedPerms := changedPerms(permissionsFromPg(currentRole.PermissionNames), nextPerms.Values())
		if !mdl.IsPermissionSuperset(permissionsFromPg(userOrgPerms.PermissionNames), changedPerms) {
			return mdl.ErrPermissionDenied
		}

		if err := c.validateCustomRoleAssignmentScope(ctx, mrp.ID, nextPerms.Values()); err != nil {
			return fmt.Errorf("validate custom role assignment scope: %w", err)
		}

		role, err = c.roleStorer.ModifyCustomRolePermissions(ctx, modifyCustomRolePermissionsToPg(mrp, userOrgPerms.OrgID))
		if err != nil {
			// The role is locked, and every application-known permission must exist in the
			// registry, so sql.ErrNoRows is an impossible state that must remain an internal error.
			return fmt.Errorf("modify custom role permissions: %w", err)
		}

		return nil
	}); err != nil {
		return mdl.CustomRole{}, fmt.Errorf("run tx: %w", err)
	}

	return customRoleFromPg(role), nil
}

// DeleteCustomRole deletes a custom role in the caller's organization.
// Returns [mdl.ErrNotFound] if the caller or selected project no longer exists, or the role is not
// owned by the caller's organization.
// Returns [mdl.ErrPermissionDenied] if the caller does not hold every permission in the role.
func (c *Core) DeleteCustomRole(ctx context.Context, roleID uuid.UUID) error {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return errors.New("auth session missing")
	}
	if sess.ProjectID == nil {
		return errors.New("project context missing")
	}

	if err := c.transactor.RunTx(ctx, func(ctx context.Context) error {
		userOrgPerms, role, err := c.customRolePermChangeContext(ctx, sess.User.UserID, *sess.ProjectID, roleID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("get permission change context: %w", mdl.ErrNotFound)
			}
			return fmt.Errorf("get permission change context: %w", err)
		}

		if role.ManagedKey != nil {
			return mdl.ErrManagedRole
		}

		if !mdl.IsPermissionSuperset(permissionsFromPg(userOrgPerms.PermissionNames), permissionsFromPg(role.PermissionNames)) {
			return mdl.ErrPermissionDenied
		}

		if err := c.roleStorer.DeleteCustomRole(ctx, userOrgPerms.OrgID, roleID); err != nil {
			// The organization-owned role was resolved and locked earlier in this transaction, so
			// sql.ErrNoRows is an impossible state that must remain an internal error.
			return fmt.Errorf("delete custom role: %w", err)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("run tx: %w", err)
	}

	return nil
}

// AssignCustomRoleToProject grants targetUserID a custom role in the caller's project.
// Returns [mdl.ErrNotFound] if the caller, target user, role, project, or membership does not
// exist, or the role belongs to a different organization.
// Returns [mdl.ErrAlreadyExists] if the assignment already exists.
// Returns [mdl.ErrPermissionDenied] if the caller does not hold every permission in the role at the project scope.
func (c *Core) AssignCustomRoleToProject(ctx context.Context, targetUserID, roleID uuid.UUID) error {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return errors.New("auth session missing")
	}
	if sess.ProjectID == nil {
		return errors.New("project context missing")
	}

	if err := c.transactor.RunTx(ctx, func(ctx context.Context) error {
		userProjectPerms, err := c.roleStorer.ProjectPermissions(ctx, sess.User.UserID, *sess.ProjectID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return mdl.ErrNotFound
			}
			return fmt.Errorf("get actor project permissions: %w", err)
		}
		if err := c.roleStorer.LockCustomRole(ctx, roleID); err != nil {
			return fmt.Errorf("lock custom role: %w", err)
		}

		role, err := c.roleStorer.CustomRoleByExternalID(ctx, userProjectPerms.OrgID, roleID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("get custom role: %w", mdl.ErrNotFound)
			}
			return fmt.Errorf("get custom role: %w", err)
		}
		if mdl.MinimumAssignmentScope(permissionsFromPg(role.PermissionNames)) > mdl.AssignmentScopeProject {
			return mdl.ErrInvalidAssignmentScope
		}

		if !mdl.IsPermissionSuperset(permissionsFromPg(userProjectPerms.PermissionNames), permissionsFromPg(role.PermissionNames)) {
			return mdl.ErrPermissionDenied
		}

		if err := c.roleStorer.AssignCustomRoleToProject(ctx, targetUserID, roleID, *sess.ProjectID); err != nil {
			switch {
			case errors.Is(err, sql.ErrNoRows):
				return fmt.Errorf("assign custom role to project: %w", mdl.ErrNotFound)
			case errors.Is(err, pgdb.ErrAlreadyExists):
				return fmt.Errorf("assign custom role to project: %w", mdl.ErrAlreadyExists)
			default:
				return fmt.Errorf("assign custom role to project: %w", err)
			}
		}

		return nil
	}); err != nil {
		return fmt.Errorf("run tx: %w", err)
	}

	return nil
}

// UnassignCustomRoleFromProject revokes targetUserID's custom role in the caller's project.
// Returns [mdl.ErrNotFound] if the caller, selected project, membership, or assignment does not
// exist, or the role belongs to a different organization.
// Returns [mdl.ErrPermissionDenied] if the caller does not hold every permission in the role at the project scope.
func (c *Core) UnassignCustomRoleFromProject(ctx context.Context, targetUserID, roleID uuid.UUID) error {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return errors.New("auth session missing")
	}
	if sess.ProjectID == nil {
		return errors.New("project context missing")
	}

	if err := c.transactor.RunTx(ctx, func(ctx context.Context) error {
		userProjectPerms, err := c.roleStorer.ProjectPermissions(ctx, sess.User.UserID, *sess.ProjectID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return mdl.ErrNotFound
			}
			return fmt.Errorf("get actor project permissions: %w", err)
		}

		role, err := c.roleStorer.CustomRoleByExternalID(ctx, userProjectPerms.OrgID, roleID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("get custom role: %w", mdl.ErrNotFound)
			}
			return fmt.Errorf("get custom role: %w", err)
		}

		if !mdl.IsPermissionSuperset(permissionsFromPg(userProjectPerms.PermissionNames), permissionsFromPg(role.PermissionNames)) {
			return mdl.ErrPermissionDenied
		}

		if err := c.roleStorer.UnassignCustomRoleFromProject(ctx, targetUserID, roleID, *sess.ProjectID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("unassign custom role from project: %w", mdl.ErrNotFound)
			}
			return fmt.Errorf("unassign custom role from project: %w", err)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("run tx: %w", err)
	}

	return nil
}

// AssignCustomRoleToOrg grants targetUserID a custom role in the caller's project organization.
// Returns [mdl.ErrNotFound] if the caller, selected project, target user, role, organization, or
// membership does not exist, or the role belongs to a different organization.
// Returns [mdl.ErrAlreadyExists] if the assignment already exists.
// Returns [mdl.ErrPermissionDenied] if the caller does not hold every permission in the role at the org scope.
func (c *Core) AssignCustomRoleToOrg(ctx context.Context, targetUserID, roleID uuid.UUID) error {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return errors.New("auth session missing")
	}
	if sess.ProjectID == nil {
		return errors.New("project context missing")
	}

	if err := c.transactor.RunTx(ctx, func(ctx context.Context) error {
		userOrgPerms, err := c.roleStorer.OrgPermissionsByProjectID(ctx, sess.User.UserID, *sess.ProjectID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return mdl.ErrNotFound
			}
			return fmt.Errorf("get actor organization permissions: %w", err)
		}

		role, err := c.roleStorer.CustomRoleByExternalID(ctx, userOrgPerms.OrgID, roleID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("get custom role: %w", mdl.ErrNotFound)
			}
			return fmt.Errorf("get custom role: %w", err)
		}

		if !mdl.IsPermissionSuperset(permissionsFromPg(userOrgPerms.PermissionNames), permissionsFromPg(role.PermissionNames)) {
			return mdl.ErrPermissionDenied
		}

		if err := c.roleStorer.AssignCustomRoleToOrg(ctx, targetUserID, roleID, userOrgPerms.OrgID); err != nil {
			switch {
			case errors.Is(err, sql.ErrNoRows):
				return fmt.Errorf("assign custom role to org: %w", mdl.ErrNotFound)
			case errors.Is(err, pgdb.ErrAlreadyExists):
				return fmt.Errorf("assign custom role to org: %w", mdl.ErrAlreadyExists)
			default:
				return fmt.Errorf("assign custom role to org: %w", err)
			}
		}

		return nil
	}); err != nil {
		return fmt.Errorf("run tx: %w", err)
	}

	return nil
}

// UnassignCustomRoleFromOrg revokes targetUserID's custom role in the caller's organization.
// Returns [mdl.ErrNotFound] if the caller, selected project, membership, or assignment does not
// exist, or the role belongs to a different organization.
// Returns [mdl.ErrPermissionDenied] if the caller does not hold every permission in the role at the org scope.
func (c *Core) UnassignCustomRoleFromOrg(ctx context.Context, targetUserID, roleID uuid.UUID) error {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return errors.New("auth session missing")
	}
	if sess.ProjectID == nil {
		return errors.New("project context missing")
	}

	if err := c.transactor.RunTx(ctx, func(ctx context.Context) error {
		userOrgPerms, err := c.roleStorer.OrgPermissionsByProjectID(ctx, sess.User.UserID, *sess.ProjectID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return mdl.ErrNotFound
			}
			return fmt.Errorf("get actor organization permissions: %w", err)
		}

		role, err := c.roleStorer.CustomRoleByExternalID(ctx, userOrgPerms.OrgID, roleID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("get custom role: %w", mdl.ErrNotFound)
			}
			return fmt.Errorf("get custom role: %w", err)
		}

		if !mdl.IsPermissionSuperset(permissionsFromPg(userOrgPerms.PermissionNames), permissionsFromPg(role.PermissionNames)) {
			return mdl.ErrPermissionDenied
		}

		if err := c.roleStorer.UnassignCustomRoleFromOrg(ctx, targetUserID, roleID, userOrgPerms.OrgID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("unassign custom role from org: %w", mdl.ErrNotFound)
			}
			return fmt.Errorf("unassign custom role from org: %w", err)
		}

		return nil
	}); err != nil {
		return fmt.Errorf("run tx: %w", err)
	}

	return nil
}

func (c *Core) customRolePermChangeContext(ctx context.Context, actorUserID uuid.UUID, projectID int, roleID uuid.UUID) (pgrbac.OrgPermissions, pgrbac.CustomRole, error) {
	userOrgPerms, err := c.roleStorer.OrgPermissionsByProjectID(ctx, actorUserID, projectID)
	if err != nil {
		return pgrbac.OrgPermissions{}, pgrbac.CustomRole{}, fmt.Errorf("get actor organization permissions: %w", err)
	}

	if err := c.roleStorer.LockCustomRole(ctx, roleID); err != nil {
		return pgrbac.OrgPermissions{}, pgrbac.CustomRole{}, fmt.Errorf("lock custom role: %w", err)
	}

	role, err := c.roleStorer.CustomRoleByExternalID(ctx, userOrgPerms.OrgID, roleID)
	if err != nil {
		return pgrbac.OrgPermissions{}, pgrbac.CustomRole{}, fmt.Errorf("get custom role: %w", err)
	}

	return userOrgPerms, role, nil
}

func (c *Core) validateCustomRoleAssignmentScope(ctx context.Context, roleID uuid.UUID, permissions []mdl.Permission) error {
	if mdl.MinimumAssignmentScope(permissions) == mdl.AssignmentScopeProject {
		return nil
	}

	hasProjectAssignments, err := c.roleStorer.CustomRoleHasProjectAssignments(ctx, roleID)
	if err != nil {
		return fmt.Errorf("check custom role project assignments: %w", err)
	}
	if hasProjectAssignments {
		return mdl.ErrInvalidAssignmentScope
	}

	return nil
}

func changedPerms(curr, next []mdl.Permission) []mdl.Permission {
	return set.
		FromSlice(curr).
		SymmetricDifference(set.FromSlice(next)).
		Values()
}
