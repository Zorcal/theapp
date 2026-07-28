// Package rbac provides the core business logic for the permissions and roles domain.
package rbac

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
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
	// UserProjectCustomRoles returns a page of custom roles assigned directly to userID in projectID.
	// An empty page does not indicate whether the user, project, or organization membership exists.
	// Callers must use UserProjectCustomRoleCount with this method to validate that context.
	UserProjectCustomRoles(ctx context.Context, userID uuid.UUID, projectID, pageSize, pageOffset int) ([]pgrbac.CustomRole, error)
	// UserProjectCustomRoleCount returns the number of custom roles assigned directly to userID in projectID.
	// Returns [sql.ErrNoRows] if the user, project, or organization membership does not exist.
	UserProjectCustomRoleCount(ctx context.Context, userID uuid.UUID, projectID int) (int, error)
	// UserOrgCustomRoles returns a page of custom roles assigned to userID across orgID.
	// An empty page does not indicate whether the user or organization membership exists.
	// Callers must use UserOrgCustomRoleCount with this method to validate that context.
	UserOrgCustomRoles(ctx context.Context, userID uuid.UUID, orgID, pageSize, pageOffset int) ([]pgrbac.CustomRole, error)
	// UserOrgCustomRoleCount returns the number of custom roles assigned to userID across orgID.
	// Returns [sql.ErrNoRows] if the user or organization membership does not exist.
	UserOrgCustomRoleCount(ctx context.Context, userID uuid.UUID, orgID int) (int, error)
	// OrgPermissionsByProjectID returns projectID's org and the names of the permissions userID
	// holds there through organization- and system-scope role assignments.
	// Returns [sql.ErrNoRows] if no such user or project exists.
	OrgPermissionsByProjectID(ctx context.Context, userID uuid.UUID, projectID int) (pgrbac.OrgPermissions, error)
	// ProjectPermissions returns projectID's org and the names of the permissions userID holds
	// through project-, organization-, and system-scope role assignments.
	// Returns [sql.ErrNoRows] if no such user or project exists.
	ProjectPermissions(ctx context.Context, userID uuid.UUID, projectID int) (pgrbac.ProjectPermissions, error)
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
	// SystemPermissions returns the names of the permissions held through system-role assignments.
	// Returns [sql.ErrNoRows] if no such user exists.
	SystemPermissions(ctx context.Context, userID uuid.UUID) ([]string, error)
	// FullyPrivilegedUserRemainsAfterSystemRoleUnassign reports whether at least one user will hold
	// every registered permission through the remaining system-role assignments.
	// Returns [sql.ErrNoRows] if the assignment does not exist.
	FullyPrivilegedUserRemainsAfterSystemRoleUnassign(ctx context.Context, userID uuid.UUID, roleName string) (bool, error)
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
