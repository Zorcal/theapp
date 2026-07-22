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

// StaticRoles returns every static role and the names of the permissions currently granted to
// it, ordered by role name.
func (s *Store) StaticRoles(ctx context.Context) ([]RoleStatic, error) {
	var roles []RoleStatic

	q := staticRolesQuery()

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.QueueMany(ctx, b, &roles); err != nil {
			return fmt.Errorf("static roles: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return nil, err
	}

	return roles, nil
}

// StaticRoleByName returns the static role named name.
// Returns [sql.ErrNoRows] if no such static role exists.
func (s *Store) StaticRoleByName(ctx context.Context, name string) (RoleStatic, error) {
	var role RoleStatic

	q := staticRoleByNameQuery(name)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &role); err != nil {
			return fmt.Errorf("static role by name: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return RoleStatic{}, err
	}

	return role, nil
}

// SystemRoleAssignmentsForUser returns the names of every static role userID holds at system
// scope, ordered by role name.
func (s *Store) SystemRoleAssignmentsForUser(ctx context.Context, userID int) ([]string, error) {
	var names []string

	q := systemRoleAssignmentsForUserQuery(userID)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.QueueMany(ctx, b, &names); err != nil {
			return fmt.Errorf("system role assignments for user: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return nil, err
	}

	return names, nil
}

// SystemPermissions returns the names of the permissions userID holds through system-scope role
// assignments only.
func (s *Store) SystemPermissions(ctx context.Context, userID int) ([]string, error) {
	var names []string

	q := systemPermissionsQuery(userID)

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

// ProjectPermissions returns projectID's org and the names of the permissions userID holds for
// projectID, resolved from project-, org-, and system-scope role assignments.
// Returns [sql.ErrNoRows] if no such project exists.
func (s *Store) ProjectPermissions(ctx context.Context, userID, projectID int) (ProjectPermissions, error) {
	var perms ProjectPermissions

	q := projectPermissionsQuery(userID, projectID)

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

// OrgRoles returns orgID's own custom roles, ordered by role name.
func (s *Store) OrgRoles(ctx context.Context, orgID int) ([]RoleCustom, error) {
	var roles []RoleCustom

	q := orgRolesQuery(orgID)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.QueueMany(ctx, b, &roles); err != nil {
			return fmt.Errorf("org roles: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return nil, err
	}

	return roles, nil
}

// RoleByExternalID returns the custom role with the given external ID.
// Returns [sql.ErrNoRows] if no such role exists.
func (s *Store) RoleByExternalID(ctx context.Context, externalID uuid.UUID) (RoleCustom, error) {
	var role RoleCustom

	q := roleByExternalIDQuery(externalID)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &role); err != nil {
			return fmt.Errorf("role by external id: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return RoleCustom{}, err
	}

	return role, nil
}

// CreateRole inserts a new custom role owned by cr.OrgID and returns it.
func (s *Store) CreateRole(ctx context.Context, cr CreateRole) (RoleCustom, error) {
	var role RoleCustom

	q := createRoleQuery(cr)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &role); err != nil {
			return fmt.Errorf("create role: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return RoleCustom{}, err
	}

	return role, nil
}

// UpdateRole replaces the name and permission set of the custom role identified by ur.ID and
// owned by ur.OrgID, and returns the updated role.
// Returns [sql.ErrNoRows] if no such role, owned by ur.OrgID, exists.
func (s *Store) UpdateRole(ctx context.Context, ur UpdateRole) (RoleCustom, error) {
	var role RoleCustom

	q := updateRoleQuery(ur)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &role); err != nil {
			return fmt.Errorf("update role: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return RoleCustom{}, err
	}

	return role, nil
}

// DeleteRole deletes the custom role identified by id and owned by orgID.
// Returns [sql.ErrNoRows] if no such role, owned by orgID, exists.
func (s *Store) DeleteRole(ctx context.Context, id, orgID int) error {
	var deletedID int

	q := deleteRoleQuery(id, orgID)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &deletedID); err != nil {
			return fmt.Errorf("delete role: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return err
	}

	return nil
}

// AssignProjectRole grants userID roleID for projectID. A no-op if the grant already exists.
func (s *Store) AssignProjectRole(ctx context.Context, userID, roleID, projectID int) error {
	if err := pgdb.RunExec(ctx, s.pool, assignProjectRoleSQL, userID, roleID, projectID); err != nil {
		return fmt.Errorf("assign project role: %w", err)
	}
	return nil
}

// AssignOrgRole grants userID roleID for orgID. A no-op if the grant already exists.
func (s *Store) AssignOrgRole(ctx context.Context, userID, roleID, orgID int) error {
	if err := pgdb.RunExec(ctx, s.pool, assignOrgRoleSQL, userID, roleID, orgID); err != nil {
		return fmt.Errorf("assign org role: %w", err)
	}
	return nil
}

// UnassignProjectRole revokes userID's roleID grant for projectID. A no-op if no such grant exists.
func (s *Store) UnassignProjectRole(ctx context.Context, userID, roleID, projectID int) error {
	if err := pgdb.RunExec(ctx, s.pool, unassignProjectRoleSQL, userID, roleID, projectID); err != nil {
		return fmt.Errorf("unassign project role: %w", err)
	}
	return nil
}

// UnassignOrgRole revokes userID's roleID grant for orgID. A no-op if no such grant exists.
func (s *Store) UnassignOrgRole(ctx context.Context, userID, roleID, orgID int) error {
	if err := pgdb.RunExec(ctx, s.pool, unassignOrgRoleSQL, userID, roleID, orgID); err != nil {
		return fmt.Errorf("unassign org role: %w", err)
	}
	return nil
}

// OrgAssignmentExists reports whether userID already holds roleID at orgID's org scope.
func (s *Store) OrgAssignmentExists(ctx context.Context, userID, roleID, orgID int) (bool, error) {
	var exists bool

	q := orgAssignmentExistsQuery(userID, roleID, orgID)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &exists); err != nil {
			return fmt.Errorf("org assignment exists: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return false, err
	}

	return exists, nil
}

// DeleteProjectAssignmentsForOrg deletes every project-scope assignment of roleID held by userID
// across every project under orgID.
func (s *Store) DeleteProjectAssignmentsForOrg(ctx context.Context, userID, roleID, orgID int) error {
	if err := pgdb.RunExec(ctx, s.pool, deleteProjectAssignmentsForOrgSQL, userID, roleID, orgID); err != nil {
		return fmt.Errorf("delete project assignments for org: %w", err)
	}
	return nil
}

// RoleAssignmentsForUser returns every role userID holds within orgID, project-scope rows for
// projects under orgID unioned with the org-scope row for orgID itself.
func (s *Store) RoleAssignmentsForUser(ctx context.Context, userID, orgID int) ([]RoleAssignment, error) {
	var assignments []RoleAssignment

	q := roleAssignmentsForUserQuery(userID, orgID)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.QueueMany(ctx, b, &assignments); err != nil {
			return fmt.Errorf("role assignments for user: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return nil, err
	}

	return assignments, nil
}

// AssignSystemRole grants userID the static role named roleName at system scope.
// Returns [sql.ErrNoRows] if no static role named roleName exists.
func (s *Store) AssignSystemRole(ctx context.Context, userID int, roleName string) error {
	var roleID int

	q := assignSystemRoleQuery(userID, roleName)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &roleID); err != nil {
			return fmt.Errorf("assign system role: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return err
	}

	return nil
}
