// Package rbac provides the core business logic for the permissions and roles domain.
package rbac

import (
	"bytes"
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

//go:generate moq -rm -fmt goimports -out role_storer_moq_test.go . RoleStorer:MockedRoleStorer

// RoleStorer defines the database operations the Core requires.
type RoleStorer interface {
	// CreateCustomRole inserts an organization-owned role and its permissions.
	// Returns [sql.ErrNoRows] if the organization or any permission does not exist.
	// Returns [pgdb.ErrAlreadyExists] if the organization already has a role with that name.
	CreateCustomRole(ctx context.Context, cr pgrbac.CreateCustomRole) (pgrbac.CustomRole, error)
	// UpdateCustomRole updates selected fields on an organization-owned role.
	// Returns [sql.ErrNoRows] if the organization does not own the role or any selected permission
	// does not exist.
	// Returns [pgdb.ErrAlreadyExists] if the organization already has a role with that name.
	UpdateCustomRole(ctx context.Context, ur pgrbac.UpdateCustomRole) (pgrbac.CustomRole, error)
	// ModifyCustomRolePermissions atomically changes selected permissions on an organization-owned role.
	// Returns [sql.ErrNoRows] if the organization does not own the role or any permission does not exist.
	ModifyCustomRolePermissions(ctx context.Context, mp pgrbac.ModifyCustomRolePermissions) (pgrbac.CustomRole, error)
	// DeleteCustomRole deletes an organization-owned role and its assignments.
	// Returns [sql.ErrNoRows] if the organization does not own the role.
	DeleteCustomRole(ctx context.Context, orgID int, roleID uuid.UUID) error
	// CustomRoleByExternalID returns an organization's role with the given external ID.
	// Returns [sql.ErrNoRows] if the organization does not own such a role.
	CustomRoleByExternalID(ctx context.Context, orgID int, roleID uuid.UUID) (pgrbac.CustomRole, error)
	// CustomRoles returns a page of an organization's custom roles.
	CustomRoles(ctx context.Context, orgID, pageSize, pageOffset int) ([]pgrbac.CustomRole, error)
	// CustomRoleCount returns the number of custom roles owned by an organization.
	CustomRoleCount(ctx context.Context, orgID int) (int, error)
	// AssignCustomRoleToProject grants an organization member an organization-owned role in a
	// project.
	// Returns [sql.ErrNoRows] if the user, role, project, or membership does not exist, or the role
	// and project belong to different organizations.
	// Returns [pgdb.ErrAlreadyExists] if the assignment already exists.
	AssignCustomRoleToProject(ctx context.Context, userID, roleID uuid.UUID, projectID int) error
	// UnassignCustomRoleFromProject revokes an organization member's role assignment in a project.
	// Returns [sql.ErrNoRows] if the membership or assignment does not exist, or the role and
	// project belong to different organizations.
	UnassignCustomRoleFromProject(ctx context.Context, userID, roleID uuid.UUID, projectID int) error
	// AssignCustomRoleToOrg grants an organization member an organization-owned role at org scope.
	// Returns [sql.ErrNoRows] if the user, role, organization, or membership does not exist, or the
	// role belongs to a different organization.
	// Returns [pgdb.ErrAlreadyExists] if the assignment already exists.
	AssignCustomRoleToOrg(ctx context.Context, userID, roleID uuid.UUID, orgID int) error
	// UnassignCustomRoleFromOrg revokes an organization member's role assignment at org scope.
	// Returns [sql.ErrNoRows] if the membership or assignment does not exist, or the role belongs to
	// a different organization.
	UnassignCustomRoleFromOrg(ctx context.Context, userID, roleID uuid.UUID, orgID int) error
	// LockSystemRoleManagement serializes system-role revokes that could remove management access.
	LockSystemRoleManagement(ctx context.Context) error
	// LockSystemRoleUser acquires a transaction-level lock that serializes system-role assignment
	// changes for userID.
	LockSystemRoleUser(ctx context.Context, userID uuid.UUID) error
	// SystemRoles returns a page of system roles and their permissions.
	SystemRoles(ctx context.Context, pageSize, pageOffset int) ([]pgrbac.SystemRole, error)
	// SystemRoleByName returns the system role named name and its permissions.
	// Returns [sql.ErrNoRows] if no such system role exists.
	SystemRoleByName(ctx context.Context, name string) (pgrbac.SystemRole, error)
	// SystemRoleCount returns the number of system roles.
	SystemRoleCount(ctx context.Context) (int, error)
	// UserSystemRolesByExternalID returns a page of system roles assigned to userID.
	UserSystemRolesByExternalID(ctx context.Context, userID uuid.UUID, pageSize, pageOffset int) ([]pgrbac.SystemRole, error)
	// UserSystemRoleCountByExternalID returns the number of system roles assigned to userID.
	// Returns [sql.ErrNoRows] if no such user exists.
	UserSystemRoleCountByExternalID(ctx context.Context, userID uuid.UUID) (int, error)
	// UserSystemPermissionsByExternalID returns the names of the permissions userID holds through
	// system-role assignments.
	UserSystemPermissionsByExternalID(ctx context.Context, userID uuid.UUID) ([]string, error)
	// SystemPermissionsRemainAfterUnassign reports whether every permission in permissionNames is
	// carried by another system-role assignment.
	// Returns [sql.ErrNoRows] if the assignment does not exist.
	SystemPermissionsRemainAfterUnassign(ctx context.Context, userID uuid.UUID, roleName string, permissionNames []string) (bool, error)
	// AssignSystemRole grants userID the system role named roleName at system scope.
	// Returns [sql.ErrNoRows] if no user with that ID or system role named roleName exists.
	// Returns [pgdb.ErrAlreadyExists] if userID already has the system role.
	AssignSystemRole(ctx context.Context, userID uuid.UUID, roleName string) error
	// UnassignSystemRole revokes the system role named roleName from userID.
	// Returns [sql.ErrNoRows] if userID does not have that system role or no such user exists.
	UnassignSystemRole(ctx context.Context, userID uuid.UUID, roleName string) error
}

// Transactor runs a function inside a database transaction.
type Transactor interface {
	RunTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Core holds the business logic for the permissions and roles domain.
type Core struct {
	roleStorer RoleStorer
	transactor Transactor
}

// NewCore constructs a Core backed by the provided role store and transactor.
func NewCore(rs RoleStorer, tr Transactor) *Core {
	return &Core{roleStorer: rs, transactor: tr}
}

// SystemRoles returns a page of system roles and their permissions, along with the total count.
func (c *Core) SystemRoles(ctx context.Context, pageSize, pageOffset int) ([]mdl.SystemRole, int, error) {
	rs, err := c.roleStorer.SystemRoles(ctx, pageSize, pageOffset)
	if err != nil {
		return nil, 0, fmt.Errorf("system roles: %w", err)
	}

	count, err := c.roleStorer.SystemRoleCount(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("system role count: %w", err)
	}

	return systemRolesFromPg(rs), count, nil
}

// UserSystemRoles returns a page of system roles assigned to userID, along with the total count.
// Returns [mdl.ErrNotFound] if no user with that ID exists.
func (c *Core) UserSystemRoles(ctx context.Context, userID uuid.UUID, pageSize, pageOffset int) ([]mdl.SystemRole, int, error) {
	rs, err := c.roleStorer.UserSystemRolesByExternalID(ctx, userID, pageSize, pageOffset)
	if err != nil {
		return nil, 0, fmt.Errorf("user system roles: %w", err)
	}

	count, err := c.roleStorer.UserSystemRoleCountByExternalID(ctx, userID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, 0, mdl.ErrNotFound
		}
		return nil, 0, fmt.Errorf("user system role count: %w", err)
	}

	return systemRolesFromPg(rs), count, nil
}

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

// AssignSystemRole grants targetUserID the system role named roleName at system scope.
// The actor is read from the auth session in ctx.
// Returns [mdl.ErrNotFound] if the target user or system role does not exist.
// Returns [mdl.ErrPermissionDenied] if the actor's system-scope permissions are not a superset of the role's.
// Returns [mdl.ErrAlreadyExists] if the target user already has the system role.
func (c *Core) AssignSystemRole(ctx context.Context, targetUserID uuid.UUID, roleName string) error {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return errors.New("auth session missing")
	}

	if err := c.transactor.RunTx(ctx, func(ctx context.Context) error {
		if err := c.lockSystemRoleUsers(ctx, sess.User.UserID, targetUserID); err != nil {
			return fmt.Errorf("lock users: %w", err)
		}

		if _, err := c.authorizeSystemRoleChange(ctx, sess.User.UserID, roleName); err != nil {
			return fmt.Errorf("authorize system role change: %w", err)
		}

		if err := c.roleStorer.AssignSystemRole(ctx, targetUserID, roleName); err != nil {
			return fmt.Errorf("assign system role: %w", handleAssignmentError(err))
		}

		return nil
	}); err != nil {
		return fmt.Errorf("run tx: %w", err)
	}

	return nil
}

// UnassignSystemRole revokes the system role named roleName from targetUserID.
// The actor is read from the auth session in ctx.
// Returns [mdl.ErrNotFound] if the target user, role, or assignment does not exist.
// Returns [mdl.ErrPermissionDenied] if the actor's system-scope permissions are not a superset of the role's.
// Returns [mdl.ErrLastRoleManager] if the change would remove the last system-role management assignment.
func (c *Core) UnassignSystemRole(ctx context.Context, targetUserID uuid.UUID, roleName string) error {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return errors.New("auth session missing")
	}

	if err := c.transactor.RunTx(ctx, func(ctx context.Context) error {
		if err := c.roleStorer.LockSystemRoleManagement(ctx); err != nil {
			return fmt.Errorf("lock system role management: %w", err)
		}

		if err := c.lockSystemRoleUsers(ctx, sess.User.UserID, targetUserID); err != nil {
			return fmt.Errorf("lock users: %w", err)
		}

		role, err := c.authorizeSystemRoleChange(ctx, sess.User.UserID, roleName)
		if err != nil {
			return fmt.Errorf("authorize system role change: %w", err)
		}

		if err := c.ensureSystemManagementAccessRemains(ctx, targetUserID, role); err != nil {
			return fmt.Errorf("ensure system management access remains: %w", err)
		}

		if err := c.roleStorer.UnassignSystemRole(ctx, targetUserID, roleName); err != nil {
			return fmt.Errorf("unassign system role: %w", handleAssignmentError(err))
		}

		return nil
	}); err != nil {
		return fmt.Errorf("run tx: %w", err)
	}

	return nil
}

// BootstrapAssignSystemRole grants userID a system role without an actor permission check.
// It is reserved for the bootstrap CLI, which must be able to establish the first system administrator.
func (c *Core) BootstrapAssignSystemRole(ctx context.Context, userID uuid.UUID, roleName string) error {
	if err := c.transactor.RunTx(ctx, func(ctx context.Context) error {
		if err := c.roleStorer.LockSystemRoleUser(ctx, userID); err != nil {
			return fmt.Errorf("lock user: %w", err)
		}

		if err := c.roleStorer.AssignSystemRole(ctx, userID, roleName); err != nil {
			return fmt.Errorf("assign system role: %w", handleAssignmentError(err))
		}

		return nil
	}); err != nil {
		return fmt.Errorf("run tx: %w", err)
	}

	return nil
}

// authorizeSystemRoleChange verifies that the actor may change assignments for roleName.
// It must run inside the write transaction after the actor and target user locks are acquired.
func (c *Core) authorizeSystemRoleChange(ctx context.Context, actorUserID uuid.UUID, roleName string) (pgrbac.SystemRole, error) {
	// Resolve both sides of the superset check after locking the actor and target. Every
	// assignment change takes the same per-user locks, so the actor's authority cannot change
	// between this check and the target's update.
	role, err := c.roleStorer.SystemRoleByName(ctx, roleName)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return pgrbac.SystemRole{}, mdl.ErrNotFound
		}
		return pgrbac.SystemRole{}, fmt.Errorf("system role: %w", err)
	}

	actorPerms, err := c.roleStorer.UserSystemPermissionsByExternalID(ctx, actorUserID)
	if err != nil {
		return pgrbac.SystemRole{}, fmt.Errorf("actor system permissions: %w", err)
	}

	// Requiring every permission carried by the role prevents the actor from granting or
	// revoking authority they do not hold themselves.
	if !mdl.IsPermissionSuperset(permissionsFromPg(actorPerms), permissionsFromPg(role.PermissionNames)) {
		return pgrbac.SystemRole{}, mdl.ErrPermissionDenied
	}

	return role, nil
}

// ensureSystemManagementAccessRemains rejects a revoke that would remove the last system-role
// management permission. It must run inside the write transaction after the management and target
// user locks are acquired.
func (c *Core) ensureSystemManagementAccessRemains(ctx context.Context, targetUserID uuid.UUID, role pgrbac.SystemRole) error {
	rolePerms := set.FromSlice(permissionsFromPg(role.PermissionNames))
	managementPerms := set.FromSlice(mdl.SystemRoleManagementPermissions())
	removedPerms := rolePerms.Intersection(managementPerms)
	if removedPerms.Len() == 0 {
		return nil
	}

	remain, err := c.roleStorer.SystemPermissionsRemainAfterUnassign(ctx, targetUserID, role.Name, permissionsToPg(removedPerms.Values()))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.ErrNotFound
		}
		return fmt.Errorf("system management permissions after unassign: %w", err)
	}

	if !remain {
		return mdl.ErrLastRoleManager
	}

	return nil
}

// lockSystemRoleUsers locks the actor and target in ascending UUID byte order so concurrent
// changes acquire shared locks consistently. It must run inside the write transaction.
func (c *Core) lockSystemRoleUsers(ctx context.Context, actorUserID, targetUserID uuid.UUID) error {
	firstUserID, secondUserID := actorUserID, targetUserID
	if bytes.Compare(firstUserID[:], secondUserID[:]) > 0 {
		firstUserID, secondUserID = secondUserID, firstUserID
	}

	if err := c.roleStorer.LockSystemRoleUser(ctx, firstUserID); err != nil {
		return fmt.Errorf("lock first user: %w", err)
	}
	if firstUserID != secondUserID {
		if err := c.roleStorer.LockSystemRoleUser(ctx, secondUserID); err != nil {
			return fmt.Errorf("lock second user: %w", err)
		}
	}

	return nil
}

func handleAssignmentError(err error) error {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return mdl.ErrNotFound
	case errors.Is(err, pgdb.ErrAlreadyExists):
		return mdl.ErrAlreadyExists
	default:
		return err
	}
}
