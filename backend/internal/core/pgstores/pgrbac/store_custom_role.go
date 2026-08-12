package pgrbac

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

// CreateCustomRole inserts an organization-owned role and its permissions.
// Returns [sql.ErrNoRows] if the organization or any permission does not exist.
// Returns [pgdb.ErrAlreadyExists] if the organization already has a role with that name.
func (s *Store) CreateCustomRole(ctx context.Context, cr CreateCustomRole) (CustomRole, error) {
	var role CustomRole

	q := createCustomRoleQuery(cr, nil)

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

// CreateOrganizationAdminRole creates an organization's canonical managed administrator role.
// Returns [sql.ErrNoRows] if the organization or any permission does not exist.
// Returns [pgdb.ErrAlreadyExists] if the organization already has a managed administrator role.
func (s *Store) CreateOrganizationAdminRole(ctx context.Context, orgID int, permissionNames []string) (CustomRole, error) {
	cr := CreateCustomRole{
		OrgID:           orgID,
		Name:            "Organization Administrator",
		PermissionNames: permissionNames,
	}
	q := createCustomRoleQuery(cr, new("organization_admin"))

	var role CustomRole
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &role); err != nil {
			return fmt.Errorf("create organization administrator role: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return CustomRole{}, err
	}

	return role, nil
}

// UpdateCustomRole updates the selected fields on a custom role and returns the updated role.
// Returns [sql.ErrNoRows] if the organization does not own the role or any selected permission
// does not exist.
// Returns [pgdb.ErrAlreadyExists] if the organization already has a role with that name.
// Returns [pgdb.ErrETagMismatch] if the role has changed since it was read.
func (s *Store) UpdateCustomRole(ctx context.Context, ur UpdateCustomRole) (CustomRole, error) {
	updateQ := updateCustomRoleQuery(ur)
	deletePermsQ := deleteCustomRolePermissionsQuery(ur.OrgID, ur.ExternalID)
	insertPermsQ := insertCustomRolePermissionsQuery(ur.ExternalID, ur.PermissionNames)
	roleQ := customRoleByExternalIDQuery(ur.OrgID, ur.ExternalID)

	var role CustomRole
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		// The ID is only a result sink so ExpectOne returns sql.ErrNoRows when the role or a selected
		// permission does not exist.
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
// Returns [pgdb.ErrETagMismatch] if the role's ETag does not match.
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

// CustomRoles returns a page of an organization's custom roles and their permissions, along with
// the total count.
func (s *Store) CustomRoles(ctx context.Context, orgID, pageSize, pageOffset int) ([]CustomRole, int, error) {
	rolesQ := customRolesQuery(orgID, pageSize, pageOffset)
	countQ := customRoleCountQuery(orgID)

	var (
		roles []CustomRole
		count int
	)
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := rolesQ.QueueMany(ctx, b, &roles); err != nil {
			return fmt.Errorf("custom roles: %w", err)
		}
		if err := countQ.Queue(ctx, b, &count); err != nil {
			return fmt.Errorf("custom role count: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return nil, 0, err
	}

	return roles, count, nil
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

// CustomRoleHasProjectAssignments reports whether a custom role has any project-scope assignments.
// Returns [sql.ErrNoRows] if the role does not exist.
func (s *Store) CustomRoleHasProjectAssignments(ctx context.Context, roleID uuid.UUID) (bool, error) {
	q := customRoleHasProjectAssignmentsQuery(roleID)

	var hasAssignments bool
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &hasAssignments); err != nil {
			return fmt.Errorf("custom role has project assignments: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return false, err
	}

	return hasAssignments, nil
}

// LockCustomRole acquires a transaction-level advisory lock that serializes assignment and
// permission changes for roleID.
func (s *Store) LockCustomRole(ctx context.Context, roleID uuid.UUID) error {
	const query = `
		SELECT pg_advisory_xact_lock(hashtext('rbac.custom-role'), id)
		FROM rbac.custom_roles
		WHERE external_id = $1`
	if err := pgdb.RunExec(ctx, s.pool, query, roleID); err != nil {
		return fmt.Errorf("lock custom role: %w", err)
	}

	return nil
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

// UserProjectCustomRoles returns a page and total count of custom roles assigned directly to
// userID in projectID.
// Returns [sql.ErrNoRows] if the user, project, or organization membership does not exist.
func (s *Store) UserProjectCustomRoles(ctx context.Context, userID uuid.UUID, projectID, pageSize, pageOffset int) ([]CustomRole, int, error) {
	rolesQ := userProjectCustomRolesQuery(userID, projectID, pageSize, pageOffset)
	countQ := userProjectCustomRoleCountQuery(userID, projectID)

	var (
		roles []CustomRole
		count int
	)
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := rolesQ.QueueMany(ctx, b, &roles); err != nil {
			return fmt.Errorf("user project custom roles: %w", err)
		}
		if err := countQ.Queue(ctx, b, &count); err != nil {
			return fmt.Errorf("user project custom role count: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return nil, 0, err
	}

	return roles, count, nil
}

// UserOrgCustomRoles returns a page and total count of custom roles assigned to userID across
// orgID.
// Returns [sql.ErrNoRows] if the user or organization membership does not exist.
func (s *Store) UserOrgCustomRoles(ctx context.Context, userID uuid.UUID, orgID, pageSize, pageOffset int) ([]CustomRole, int, error) {
	rolesQ := userOrgCustomRolesQuery(userID, orgID, pageSize, pageOffset)
	countQ := userOrgCustomRoleCountQuery(userID, orgID)

	var (
		roles []CustomRole
		count int
	)
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := rolesQ.QueueMany(ctx, b, &roles); err != nil {
			return fmt.Errorf("user organization custom roles: %w", err)
		}
		if err := countQ.Queue(ctx, b, &count); err != nil {
			return fmt.Errorf("user organization custom role count: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return nil, 0, err
	}

	return roles, count, nil
}
