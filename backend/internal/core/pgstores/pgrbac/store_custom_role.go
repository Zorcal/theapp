package pgrbac

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

// LockCustomRoleManagement serializes custom-role changes that could remove management access.
// It must be called within a transaction.
func (s *Store) LockCustomRoleManagement(ctx context.Context) error {
	if err := pgdb.RunExec(ctx, s.pool, "SELECT pg_advisory_xact_lock(hashtext('rbac.custom-role-management'), 0)"); err != nil {
		return fmt.Errorf("lock custom-role management: %w", err)
	}

	return nil
}

// ProjectCustomRolePermissionsRemainAfterUnassign reports whether every permission in permNames
// is carried by another assignment in projectID.
// Returns [sql.ErrNoRows] if the assignment does not exist.
func (s *Store) ProjectCustomRolePermissionsRemainAfterUnassign(ctx context.Context, userID, roleID uuid.UUID, projectID int, permNames []string) (bool, error) {
	q := projectCustomRolePermissionsRemainAfterUnassignQuery(userID, roleID, projectID, permNames)

	var remain bool
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &remain); err != nil {
			return fmt.Errorf("project custom-role permissions remain after unassign: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return false, err
	}

	return remain, nil
}

// OrgCustomRolePermissionsRemainAfterUnassign reports whether every permission in permNames is
// carried by another assignment in orgID.
// Returns [sql.ErrNoRows] if the assignment does not exist.
func (s *Store) OrgCustomRolePermissionsRemainAfterUnassign(ctx context.Context, userID, roleID uuid.UUID, orgID int, permNames []string) (bool, error) {
	q := orgCustomRolePermissionsRemainAfterUnassignQuery(userID, roleID, orgID, permNames)

	var remain bool
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &remain); err != nil {
			return fmt.Errorf("organization custom-role permissions remain after unassign: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return false, err
	}

	return remain, nil
}

// CustomRoleManagementPermissionsRemainAfterRemoval reports whether removing the given permissions
// from a role leaves every affected project and organization with another holder.
// Returns [sql.ErrNoRows] if the organization does not own the role.
func (s *Store) CustomRoleManagementPermissionsRemainAfterRemoval(
	ctx context.Context,
	orgID int,
	roleID uuid.UUID,
	projectPermNames, orgPermNames []string,
) (bool, error) {
	q := customRoleManagementPermissionsRemainAfterRemovalQuery(orgID, roleID, projectPermNames, orgPermNames)

	var remain bool
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &remain); err != nil {
			return fmt.Errorf("custom-role management permissions remain after removal: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return false, err
	}

	return remain, nil
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

// AssignCustomRoleToProject grants an organization member an organization-owned role in projectID.
// Returns [sql.ErrNoRows] if the user, role, project, or membership does not exist, or the role and
// project belong to different organizations.
// Returns [pgdb.ErrAlreadyExists] if the assignment already exists.
func (s *Store) AssignCustomRoleToProject(ctx context.Context, userID, roleID uuid.UUID, projectID int) error {
	q := assignCustomRoleToProjectQuery(userID, roleID, projectID)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		var roleIDSink int
		if err := q.Queue(ctx, b, &roleIDSink); err != nil {
			return fmt.Errorf("assign custom role to project: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return err
	}

	return nil
}

// UnassignCustomRoleFromProject revokes an organization member's role assignment in projectID.
// Returns [sql.ErrNoRows] if the membership or assignment does not exist, or the role and project
// belong to different organizations.
func (s *Store) UnassignCustomRoleFromProject(ctx context.Context, userID, roleID uuid.UUID, projectID int) error {
	q := unassignCustomRoleFromProjectQuery(userID, roleID, projectID)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		var roleIDSink int
		if err := q.Queue(ctx, b, &roleIDSink); err != nil {
			return fmt.Errorf("unassign custom role from project: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return err
	}

	return nil
}

// AssignCustomRoleToOrg grants an organization member an organization-owned role at org scope.
// Returns [sql.ErrNoRows] if the user, role, organization, or membership does not exist, or the
// role belongs to a different organization.
// Returns [pgdb.ErrAlreadyExists] if the assignment already exists.
func (s *Store) AssignCustomRoleToOrg(ctx context.Context, userID, roleID uuid.UUID, orgID int) error {
	q := assignCustomRoleToOrgQuery(userID, roleID, orgID)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		var roleIDSink int
		if err := q.Queue(ctx, b, &roleIDSink); err != nil {
			return fmt.Errorf("assign custom role to org: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return err
	}

	return nil
}

// UnassignCustomRoleFromOrg revokes an organization member's role assignment at org scope.
// Returns [sql.ErrNoRows] if the membership or assignment does not exist, or the role belongs to a
// different organization.
func (s *Store) UnassignCustomRoleFromOrg(ctx context.Context, userID, roleID uuid.UUID, orgID int) error {
	q := unassignCustomRoleFromOrgQuery(userID, roleID, orgID)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		var roleIDSink int
		if err := q.Queue(ctx, b, &roleIDSink); err != nil {
			return fmt.Errorf("unassign custom role from org: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return err
	}

	return nil
}

// UserProjectCustomRoles returns a page of custom roles assigned directly to userID in projectID.
// An empty page does not indicate whether the user, project, or organization membership exists.
// Callers must use UserProjectCustomRoleCount with this method to validate that context.
func (s *Store) UserProjectCustomRoles(ctx context.Context, userID uuid.UUID, projectID, pageSize, pageOffset int) ([]CustomRole, error) {
	q := userProjectCustomRolesQuery(userID, projectID, pageSize, pageOffset)

	var roles []CustomRole
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.QueueMany(ctx, b, &roles); err != nil {
			return fmt.Errorf("user project custom roles: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return nil, err
	}

	return roles, nil
}

// UserProjectCustomRoleCount returns the number of custom roles assigned directly to userID in projectID.
// Returns [sql.ErrNoRows] if the user, project, or organization membership does not exist.
func (s *Store) UserProjectCustomRoleCount(ctx context.Context, userID uuid.UUID, projectID int) (int, error) {
	q := userProjectCustomRoleCountQuery(userID, projectID)

	var count int
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &count); err != nil {
			return fmt.Errorf("user project custom role count: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return 0, err
	}

	return count, nil
}

// UserOrgCustomRoles returns a page of custom roles assigned to userID across orgID.
// An empty page does not indicate whether the user or organization membership exists.
// Callers must use UserOrgCustomRoleCount with this method to validate that context.
func (s *Store) UserOrgCustomRoles(ctx context.Context, userID uuid.UUID, orgID, pageSize, pageOffset int) ([]CustomRole, error) {
	q := userOrgCustomRolesQuery(userID, orgID, pageSize, pageOffset)

	var roles []CustomRole
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.QueueMany(ctx, b, &roles); err != nil {
			return fmt.Errorf("user organization custom roles: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return nil, err
	}

	return roles, nil
}

// UserOrgCustomRoleCount returns the number of custom roles assigned to userID across orgID.
// Returns [sql.ErrNoRows] if the user or organization membership does not exist.
func (s *Store) UserOrgCustomRoleCount(ctx context.Context, userID uuid.UUID, orgID int) (int, error) {
	q := userOrgCustomRoleCountQuery(userID, orgID)

	var count int
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &count); err != nil {
			return fmt.Errorf("user organization custom role count: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return 0, err
	}

	return count, nil
}
