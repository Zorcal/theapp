package rbac

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
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

	rs, err := c.roleStorer.CustomRoles(ctx, *sess.OrgID, pageSize, pageOffset)
	if err != nil {
		return nil, 0, fmt.Errorf("custom roles: %w", err)
	}

	count, err := c.roleStorer.CustomRoleCount(ctx, *sess.OrgID)
	if err != nil {
		return nil, 0, fmt.Errorf("custom role count: %w", err)
	}

	return customRolesFromPg(rs), count, nil
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
// Returns [mdl.ErrAlreadyExists] if the organization already has a role with that name.
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

	role, err := c.roleStorer.CreateCustomRole(ctx, createCustomRoleToPg(cr, *sess.OrgID))
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return mdl.CustomRole{}, fmt.Errorf("create custom role: %w", mdl.ErrNotFound)
		case errors.Is(err, pgdb.ErrAlreadyExists):
			return mdl.CustomRole{}, fmt.Errorf("create custom role: %w", mdl.ErrAlreadyExists)
		case errors.Is(err, pgdb.ErrCheckConstraintViolated):
			return mdl.CustomRole{}, fmt.Errorf("create custom role: %w", mdl.ErrValidation)
		default:
			return mdl.CustomRole{}, fmt.Errorf("create custom role: %w", err)
		}
	}

	return customRoleFromPg(role), nil
}

// UpdateCustomRole updates selected fields on a custom role in the caller's organization.
// Returns [mdl.ErrNotFound] if the role is not owned by the caller's organization.
// Returns [mdl.ErrValidation] if the input is invalid or contains a system-only permission.
// Returns [mdl.ErrAlreadyExists] if the organization already has a role with that name.
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
// Returns [mdl.ErrNotFound] if the role is not owned by the caller's organization.
// Returns [mdl.ErrValidation] if the input contains overlapping or system-only permissions.
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

	role, err := c.roleStorer.ModifyCustomRolePermissions(ctx, modifyCustomRolePermissionsToPg(mrp, *sess.OrgID))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.CustomRole{}, fmt.Errorf("modify custom role permissions: %w", mdl.ErrNotFound)
		}
		return mdl.CustomRole{}, fmt.Errorf("modify custom role permissions: %w", err)
	}

	return customRoleFromPg(role), nil
}

// DeleteCustomRole deletes a custom role in the caller's organization.
// Returns [mdl.ErrNotFound] if the role is not owned by the caller's organization.
func (c *Core) DeleteCustomRole(ctx context.Context, roleID uuid.UUID) error {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return errors.New("auth session missing")
	}
	if sess.OrgID == nil {
		return errors.New("organization context missing")
	}

	if err := c.roleStorer.DeleteCustomRole(ctx, *sess.OrgID, roleID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("delete custom role: %w", mdl.ErrNotFound)
		}
		return fmt.Errorf("delete custom role: %w", err)
	}

	return nil
}

// AssignCustomRoleToProject grants targetUserID a custom role in the caller's project.
// Returns [mdl.ErrNotFound] if the target user, role, project, or membership does not exist, or the
// role belongs to a different organization.
// Returns [mdl.ErrAlreadyExists] if the assignment already exists.
func (c *Core) AssignCustomRoleToProject(ctx context.Context, targetUserID, roleID uuid.UUID) error {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return errors.New("auth session missing")
	}
	if sess.ProjectID == nil {
		return errors.New("project context missing")
	}

	if err := c.roleStorer.AssignCustomRoleToProject(ctx, targetUserID, roleID, *sess.ProjectID); err != nil {
		return fmt.Errorf("assign custom role to project: %w", handleAssignmentError(err))
	}

	return nil
}

// UnassignCustomRoleFromProject revokes targetUserID's custom role in the caller's project.
// Returns [mdl.ErrNotFound] if the membership or assignment does not exist, or the role belongs to
// a different organization.
func (c *Core) UnassignCustomRoleFromProject(ctx context.Context, targetUserID, roleID uuid.UUID) error {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return errors.New("auth session missing")
	}
	if sess.ProjectID == nil {
		return errors.New("project context missing")
	}

	if err := c.roleStorer.UnassignCustomRoleFromProject(ctx, targetUserID, roleID, *sess.ProjectID); err != nil {
		return fmt.Errorf("unassign custom role from project: %w", handleAssignmentError(err))
	}

	return nil
}

// AssignCustomRoleToOrg grants targetUserID a custom role in the caller's organization.
// Returns [mdl.ErrNotFound] if the target user, role, organization, or membership does not exist,
// or the role belongs to a different organization.
// Returns [mdl.ErrAlreadyExists] if the assignment already exists.
func (c *Core) AssignCustomRoleToOrg(ctx context.Context, targetUserID, roleID uuid.UUID) error {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return errors.New("auth session missing")
	}
	if sess.OrgID == nil {
		return errors.New("organization context missing")
	}

	if err := c.roleStorer.AssignCustomRoleToOrg(ctx, targetUserID, roleID, *sess.OrgID); err != nil {
		return fmt.Errorf("assign custom role to org: %w", handleAssignmentError(err))
	}

	return nil
}

// UnassignCustomRoleFromOrg revokes targetUserID's custom role in the caller's organization.
// Returns [mdl.ErrNotFound] if the membership or assignment does not exist, or the role belongs to
// a different organization.
func (c *Core) UnassignCustomRoleFromOrg(ctx context.Context, targetUserID, roleID uuid.UUID) error {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return errors.New("auth session missing")
	}
	if sess.OrgID == nil {
		return errors.New("organization context missing")
	}

	if err := c.roleStorer.UnassignCustomRoleFromOrg(ctx, targetUserID, roleID, *sess.OrgID); err != nil {
		return fmt.Errorf("unassign custom role from org: %w", handleAssignmentError(err))
	}

	return nil
}
