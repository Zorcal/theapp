// Package pgorg provides organization and project db access functionality.
package pgorg

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
	return &Store{
		pool: pool,
	}
}

// CreateOrganization inserts a new organization, along with its control project, and returns the
// organization.
// Returns [pgdb.ErrAlreadyExists] if an organization with the same name already exists.
func (s *Store) CreateOrganization(ctx context.Context, co CreateOrganization) (Organization, error) {
	var org Organization

	q := createOrganizationQuery(co)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &org); err != nil {
			return fmt.Errorf("create organization: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return Organization{}, err
	}

	return org, nil
}

// CreateProject inserts a new project owned by cp.OrgID and returns it.
// Returns [sql.ErrNoRows] if no organization with that ID exists.
// Returns [pgdb.ErrAlreadyExists] if a project with the same name already exists in the organization.
func (s *Store) CreateProject(ctx context.Context, cp CreateProject) (Project, error) {
	var project Project

	q := createProjectQuery(cp)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &project); err != nil {
			return fmt.Errorf("create project: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return Project{}, err
	}

	return project, nil
}

// AddOrganizationMember adds a user to an organization.
// Returns [sql.ErrNoRows] if the user or organization does not exist.
// Returns [pgdb.ErrAlreadyExists] if the user is already an organization member.
func (s *Store) AddOrganizationMember(ctx context.Context, userID uuid.UUID, orgID int) error {
	q := addOrganizationMemberQuery(userID, orgID)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		var orgIDSink int
		if err := q.Queue(ctx, b, &orgIDSink); err != nil {
			return fmt.Errorf("add organization member: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return err
	}

	return nil
}

// OrganizationByName returns the organization with the given name.
// Returns [sql.ErrNoRows] if no such organization exists.
func (s *Store) OrganizationByName(ctx context.Context, name string) (Organization, error) {
	var org Organization

	q := organizationByNameQuery(name)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &org); err != nil {
			return fmt.Errorf("organization by name: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return Organization{}, err
	}

	return org, nil
}

// ProjectByID returns the project with the given ID.
// Returns [sql.ErrNoRows] if no such project exists.
func (s *Store) ProjectByID(ctx context.Context, id int) (Project, error) {
	var project Project

	q := projectByIDQuery(id)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &project); err != nil {
			return fmt.Errorf("project by id: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return Project{}, err
	}

	return project, nil
}

// ProjectByName returns the project named name owned by orgID.
// Returns [sql.ErrNoRows] if no such project exists.
func (s *Store) ProjectByName(ctx context.Context, orgID int, name string) (Project, error) {
	var project Project

	q := projectByNameQuery(orgID, name)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &project); err != nil {
			return fmt.Errorf("project by name: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return Project{}, err
	}

	return project, nil
}

// AccessibleProjects returns a page and total count of projects reachable through any role
// assignment held by userID that match filter, ordered by organization ID and natural name.
// Returns [sql.ErrNoRows] if no such user exists.
func (s *Store) AccessibleProjects(ctx context.Context, userID uuid.UUID, filter ProjectFilter, pageSize, pageOffset int) ([]Project, int, error) {
	projectsQ := accessibleProjectsQuery(userID, filter, pageSize, pageOffset)
	countQ := accessibleProjectCountQuery(userID, filter)

	var (
		projects []Project
		count    int
	)
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := projectsQ.QueueMany(ctx, b, &projects); err != nil {
			return fmt.Errorf("accessible projects: %w", err)
		}
		if err := countQ.Queue(ctx, b, &count); err != nil {
			return fmt.Errorf("accessible project count: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return nil, 0, err
	}

	return projects, count, nil
}
