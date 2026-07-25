// Package pgrbac provides role/permission db access functionality.
package pgrbac

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// CreateCustomRole inserts an organization-owned role and its permissions.
// Returns [sql.ErrNoRows] if the organization or any permission does not exist.
// Returns [pgdb.ErrAlreadyExists] if the organization already has a role with that name.
func (s *Store) CreateCustomRole(ctx context.Context, cr CreateCustomRole) (CustomRole, error) {
	var role CustomRole

	q := createCustomRoleQuery(cr)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &role); err != nil {
			return fmt.Errorf("create custom role: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatchTx(ctx, s.pool, doInBatch); err != nil {
		return CustomRole{}, err
	}

	return role, nil
}

// UpdateCustomRole updates the selected fields on a custom role and returns the updated role.
// Returns [sql.ErrNoRows] if the organization does not own the role or any selected permission
// does not exist.
// Returns [pgdb.ErrAlreadyExists] if the organization already has a role with that name.
func (s *Store) UpdateCustomRole(ctx context.Context, ur UpdateCustomRole) (CustomRole, error) {
	validatePermsQ := validateCustomRolePermsQuery(ur.OrgID, ur.ExternalID, ur.PermissionNames)
	updateQ := updateCustomRoleQuery(ur)
	deletePermsQ := deleteCustomRolePermissionsQuery(ur.OrgID, ur.ExternalID)
	insertPermsQ := insertCustomRolePermissionsQuery(ur.ExternalID, ur.PermissionNames)
	roleQ := customRoleByExternalIDQuery(ur.OrgID, ur.ExternalID)

	var role CustomRole
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if ur.Fields.PermissionNames {
			// The ID is only a result sink so ExpectOne returns sql.ErrNoRows on failed validation.
			var validatedRoleIDSink int
			if err := validatePermsQ.Queue(ctx, b, &validatedRoleIDSink); err != nil {
				return fmt.Errorf("validate custom role permissions: %w", err)
			}
		}
		// The ID is only a result sink so ExpectOne returns sql.ErrNoRows when no role was updated.
		var updatedRoleIDSink int
		if err := updateQ.Queue(ctx, b, &updatedRoleIDSink); err != nil {
			return fmt.Errorf("update custom role: %w", err)
		}
		if ur.Fields.PermissionNames {
			// The deleted IDs are only a result sink required by QueueMany.
			var deletedPermIDsSink []int
			if err := deletePermsQ.QueueMany(ctx, b, &deletedPermIDsSink); err != nil {
				return fmt.Errorf("delete custom role permissions: %w", err)
			}
			// The inserted IDs are only a result sink required by QueueMany.
			var insertedPermIDsSink []int
			if err := insertPermsQ.QueueMany(ctx, b, &insertedPermIDsSink); err != nil {
				return fmt.Errorf("insert custom role permissions: %w", err)
			}
		}
		if err := roleQ.Queue(ctx, b, &role); err != nil {
			return fmt.Errorf("updated custom role: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatchTx(ctx, s.pool, doInBatch); err != nil {
		return CustomRole{}, err
	}

	return role, nil
}

// ModifyCustomRolePermissions atomically adds and removes permissions and returns the complete
// role. Adding an existing permission or removing an absent permission is a no-op.
// Returns [sql.ErrNoRows] if the organization does not own the role or any permission does not
// exist.
func (s *Store) ModifyCustomRolePermissions(ctx context.Context, mp ModifyCustomRolePermissions) (CustomRole, error) {
	modifyQ := modifyCustomRolePermissionsQuery(mp)
	roleQ := customRoleByExternalIDQuery(mp.OrgID, mp.ExternalID)

	var role CustomRole
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		// The ID is only a result sink so ExpectOne returns sql.ErrNoRows for a missing target.
		var roleIDSink int
		if err := modifyQ.Queue(ctx, b, &roleIDSink); err != nil {
			return fmt.Errorf("modify custom role permissions: %w", err)
		}
		if err := roleQ.Queue(ctx, b, &role); err != nil {
			return fmt.Errorf("modified custom role: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatchTx(ctx, s.pool, doInBatch); err != nil {
		return CustomRole{}, err
	}

	return role, nil
}

// DeleteCustomRole deletes an organization-owned custom role.
// Returns [sql.ErrNoRows] if the organization does not own the role.
func (s *Store) DeleteCustomRole(ctx context.Context, orgID int, roleID uuid.UUID) error {
	deleteProjectAssignmentsQ := deleteCustomRoleProjectAssignmentsQuery(orgID, roleID)
	deleteOrgAssignmentsQ := deleteCustomRoleOrgAssignmentsQuery(orgID, roleID)
	deletePermsQ := deleteCustomRolePermissionsQuery(orgID, roleID)
	deleteRoleQ := deleteCustomRoleQuery(orgID, roleID)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		// The IDs are only a result sink required by QueueMany.
		var projectAssignmentIDsSink []int
		if err := deleteProjectAssignmentsQ.QueueMany(ctx, b, &projectAssignmentIDsSink); err != nil {
			return fmt.Errorf("delete custom role project assignments: %w", err)
		}
		// The IDs are only a result sink required by QueueMany.
		var orgAssignmentIDsSink []int
		if err := deleteOrgAssignmentsQ.QueueMany(ctx, b, &orgAssignmentIDsSink); err != nil {
			return fmt.Errorf("delete custom role org assignments: %w", err)
		}
		// The IDs are only a result sink required by QueueMany.
		var permIDsSink []int
		if err := deletePermsQ.QueueMany(ctx, b, &permIDsSink); err != nil {
			return fmt.Errorf("delete custom role permissions: %w", err)
		}
		// The ID is only a result sink so ExpectOne returns sql.ErrNoRows when no role was deleted.
		var roleIDSink int
		if err := deleteRoleQ.Queue(ctx, b, &roleIDSink); err != nil {
			return fmt.Errorf("delete custom role: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatchTx(ctx, s.pool, doInBatch); err != nil {
		return err
	}

	return nil
}

// CustomRoles returns a page of an organization's custom roles and their permissions.
func (s *Store) CustomRoles(ctx context.Context, orgID, pageSize, pageOffset int) ([]CustomRole, error) {
	q := customRolesQuery(orgID, pageSize, pageOffset)

	var roles []CustomRole
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.QueueMany(ctx, b, &roles); err != nil {
			return fmt.Errorf("custom roles: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return nil, err
	}

	return roles, nil
}

// CustomRoleByExternalID returns an organization's custom role with the given external ID.
// Returns [sql.ErrNoRows] if the organization does not own such a role.
func (s *Store) CustomRoleByExternalID(ctx context.Context, orgID int, roleID uuid.UUID) (CustomRole, error) {
	q := customRoleByExternalIDQuery(orgID, roleID)

	var role CustomRole
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &role); err != nil {
			return fmt.Errorf("custom role: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return CustomRole{}, err
	}

	return role, nil
}

// CustomRoleCount returns the number of custom roles owned by an organization.
func (s *Store) CustomRoleCount(ctx context.Context, orgID int) (int, error) {
	q := customRoleCountQuery(orgID)

	var count int
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &count); err != nil {
			return fmt.Errorf("custom role count: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return 0, err
	}

	return count, nil
}

// LockSystemRoleManagement serializes system-role revokes that could remove management access.
// It must be called within a transaction.
func (s *Store) LockSystemRoleManagement(ctx context.Context) error {
	if err := pgdb.RunExec(ctx, s.pool, "SELECT pg_advisory_xact_lock(hashtext('rbac.system-role-management'), 0)"); err != nil {
		return fmt.Errorf("lock system-role management: %w", err)
	}

	return nil
}

// LockSystemRoleUser acquires a transaction-level advisory lock that serializes system-role
// assignment changes for userID. It must be called within a transaction.
func (s *Store) LockSystemRoleUser(ctx context.Context, userID uuid.UUID) error {
	const query = `
		SELECT pg_advisory_xact_lock(hashtext('rbac.system-role-user'), id)
		FROM useraccess.users
		WHERE external_id = $1`

	if err := pgdb.RunExec(ctx, s.pool, query, userID); err != nil {
		return fmt.Errorf("lock system-role user: %w", err)
	}

	return nil
}

// SystemRoles returns a page of system roles and their permissions, ordered by role name.
func (s *Store) SystemRoles(ctx context.Context, pageSize, pageOffset int) ([]SystemRole, error) {
	q := systemRolesQuery(pageSize, pageOffset)

	var roles []SystemRole
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.QueueMany(ctx, b, &roles); err != nil {
			return fmt.Errorf("system roles: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return nil, err
	}

	return roles, nil
}

// SystemRoleByName returns the system role named name and its permissions.
// Returns [sql.ErrNoRows] if no such system role exists.
func (s *Store) SystemRoleByName(ctx context.Context, name string) (SystemRole, error) {
	q := systemRoleByNameQuery(name)

	var role SystemRole
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &role); err != nil {
			return fmt.Errorf("system role: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return SystemRole{}, err
	}

	return role, nil
}

// SystemRoleCount returns the number of system roles.
func (s *Store) SystemRoleCount(ctx context.Context) (int, error) {
	q := systemRoleCountQuery()

	var count int
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &count); err != nil {
			return fmt.Errorf("system role count: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return 0, err
	}

	return count, nil
}

// UserSystemRolesByExternalID returns a page of system roles assigned to userID, ordered by role name.
func (s *Store) UserSystemRolesByExternalID(ctx context.Context, userID uuid.UUID, pageSize, pageOffset int) ([]SystemRole, error) {
	q := userSystemRolesByExternalIDQuery(userID, pageSize, pageOffset)

	var roles []SystemRole
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.QueueMany(ctx, b, &roles); err != nil {
			return fmt.Errorf("user system roles: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return nil, err
	}

	return roles, nil
}

// UserSystemRoleCountByExternalID returns the number of system roles assigned to userID.
// Returns [sql.ErrNoRows] if no such user exists.
func (s *Store) UserSystemRoleCountByExternalID(ctx context.Context, userID uuid.UUID) (int, error) {
	q := userSystemRoleCountByExternalIDQuery(userID)

	var count int
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &count); err != nil {
			return fmt.Errorf("user system role count: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return 0, err
	}

	return count, nil
}

// UserSystemPermissionsByExternalID returns the names of the permissions userID holds through
// system-scope role assignments only.
func (s *Store) UserSystemPermissionsByExternalID(ctx context.Context, userID uuid.UUID) ([]string, error) {
	q := userSystemPermissionsByExternalIDQuery(userID)

	var names []string
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.QueueMany(ctx, b, &names); err != nil {
			return fmt.Errorf("system permissions: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return nil, err
	}

	return names, nil
}

// SystemPermissionsRemainAfterUnassign reports whether every permission in permNames is
// carried by another system-role assignment.
// Returns [sql.ErrNoRows] if the assignment does not exist.
func (s *Store) SystemPermissionsRemainAfterUnassign(ctx context.Context, userID uuid.UUID, roleName string, permNames []string) (bool, error) {
	q := systemPermissionsRemainAfterUnassignQuery(userID, roleName, permNames)

	var remain bool
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &remain); err != nil {
			return fmt.Errorf("system permissions remain after unassign: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return false, err
	}

	return remain, nil
}

// ProjectPermissions returns projectID's org and the names of the permissions userID holds for
// projectID, resolved from project-, org-, and system-scope role assignments.
// Returns [sql.ErrNoRows] if no such project exists.
func (s *Store) ProjectPermissions(ctx context.Context, userID, projectID int) (ProjectPermissions, error) {
	q := projectPermissionsQuery(userID, projectID)

	var perms ProjectPermissions
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &perms); err != nil {
			return fmt.Errorf("project permissions: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return ProjectPermissions{}, err
	}

	return perms, nil
}

// AssignSystemRole grants userID the system role named roleName at system scope.
// Returns [sql.ErrNoRows] if no user with that ID or system role named roleName exists.
// Returns [pgdb.ErrAlreadyExists] if userID already has the system role.
func (s *Store) AssignSystemRole(ctx context.Context, userID uuid.UUID, roleName string) error {
	q := assignSystemRoleQuery(userID, roleName)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		// The ID is only a result sink so ExpectOne returns sql.ErrNoRows when no role was assigned.
		var roleIDSink int
		if err := q.Queue(ctx, b, &roleIDSink); err != nil {
			return fmt.Errorf("assign system role: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return err
	}

	return nil
}

// UnassignSystemRole revokes the system role named roleName from userID.
// Returns [sql.ErrNoRows] if userID does not have that system role or no such user exists.
func (s *Store) UnassignSystemRole(ctx context.Context, userID uuid.UUID, roleName string) error {
	q := unassignSystemRoleQuery(userID, roleName)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		// The ID is only a result sink so ExpectOne returns sql.ErrNoRows when no role was unassigned.
		var roleIDSink int
		if err := q.Queue(ctx, b, &roleIDSink); err != nil {
			return fmt.Errorf("unassign system role: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return err
	}

	return nil
}
