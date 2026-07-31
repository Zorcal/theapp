// Package org provides the core business logic for the organization and project domain.
package org

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgorg"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

// errOrgCreatorNotFound distinguishes a missing authenticated creator from other sql.ErrNoRows
// failures so CreateOrganization can map it at the exported core boundary.
var errOrgCreatorNotFound = errors.New("organization creator not found")

//go:generate moq -rm -fmt goimports -out org_storer_moq_test.go . OrgStorer:MockedOrgStorer RoleBootstrapperStore:MockedRoleBootstrapperStore

// OrgStorer defines the database operations the Core requires.
type OrgStorer interface {
	// CreateOrganization inserts a new organization, along with its control project, and returns
	// the organization.
	// Returns [pgdb.ErrAlreadyExists] if an organization with the same name already exists.
	CreateOrganization(ctx context.Context, co pgorg.CreateOrganization) (pgorg.Organization, error)
	// CreateProject inserts a new project owned by cp.OrgID and returns it.
	// Returns [sql.ErrNoRows] if no organization with that ID exists.
	// Returns [pgdb.ErrAlreadyExists] if a project with the same name already exists in the organization.
	CreateProject(ctx context.Context, cp pgorg.CreateProject) (pgorg.Project, error)
	// AddOrganizationMember adds a user to an organization.
	// Returns [sql.ErrNoRows] if the user or organization does not exist.
	// Returns [pgdb.ErrAlreadyExists] if the user is already an organization member.
	AddOrganizationMember(ctx context.Context, userID uuid.UUID, orgID int) error
	// OrganizationByName returns the organization with the given name.
	// Returns [sql.ErrNoRows] if no such organization exists.
	OrganizationByName(ctx context.Context, name string) (pgorg.Organization, error)
	// ProjectByName returns the project named name owned by orgID.
	// Returns [sql.ErrNoRows] if no such project exists.
	ProjectByName(ctx context.Context, orgID int, name string) (pgorg.Project, error)
	// AccessibleProjects returns a page and total count of projects reachable through any role
	// assignment held by userID that match filter, ordered by organization ID and natural name.
	// Returns [sql.ErrNoRows] if no such user exists.
	AccessibleProjects(ctx context.Context, userID uuid.UUID, filter pgorg.ProjectFilter, pageSize, pageOffset int) ([]pgorg.Project, int, error)
}

// RoleBootstrapperStore defines the managed-role persistence operations required during
// organization creation.
type RoleBootstrapperStore interface {
	// CreateOrganizationAdminRole creates an organization's canonical managed administrator role.
	CreateOrganizationAdminRole(ctx context.Context, orgID int, permissionNames []string) (pgrbac.CustomRole, error)
	// AssignCustomRoleToOrg assigns an organization-owned role to an organization member.
	AssignCustomRoleToOrg(ctx context.Context, userID, roleID uuid.UUID, orgID int) error
}

// Transactor runs a function inside a database transaction.
type Transactor interface {
	RunTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Core holds the business logic for the organization and project domain.
type Core struct {
	orgStorer             OrgStorer
	roleBootstrapperStore RoleBootstrapperStore
	transactor            Transactor
}

// NewCore constructs a Core backed by the provided stores and Transactor.
func NewCore(os OrgStorer, rb RoleBootstrapperStore, tr Transactor) *Core {
	return &Core{orgStorer: os, roleBootstrapperStore: rb, transactor: tr}
}

// CreateOrganization creates a new organization, along with a default project named
// co.ProjectName and a control project. It also adds the authenticated user as a member,
// creates the managed organization administrator role, assigns it to that user, and returns
// the created organization.
// Returns [mdl.ErrAlreadyExists] if an organization with the same name already exists.
// Returns [mdl.ErrControlProjectNameConflict] if co.ProjectName collides with the org's control project.
// Returns [mdl.ErrNotFound] if the authenticated creator no longer exists.
// Returns [mdl.ErrValidation] if co is invalid.
func (c *Core) CreateOrganization(ctx context.Context, co mdl.CreateOrganization) (mdl.Organization, error) {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return mdl.Organization{}, errors.New("auth session missing")
	}

	organization, err := c.createOrganization(ctx, co, &sess.User.UserID)
	if err != nil {
		if errors.Is(err, errOrgCreatorNotFound) {
			return mdl.Organization{}, fmt.Errorf("create organization: %w", mdl.ErrNotFound)
		}
		return mdl.Organization{}, fmt.Errorf("create organization: %w", err)
	}

	return organization, nil
}

// BootstrapOrganization creates an organization without creator membership or managed role state.
// It is reserved for establishing the system organization before its bootstrap user exists.
func (c *Core) BootstrapOrganization(ctx context.Context, co mdl.CreateOrganization) (mdl.Organization, error) {
	return c.createOrganization(ctx, co, nil)
}

// createOrganization creates the organization, control project, and default project in one
// transaction. When creatorID is provided, the same transaction also adds the creator as an
// organization member and assigns the canonical managed organization-administrator role.
func (c *Core) createOrganization(ctx context.Context, co mdl.CreateOrganization, creatorID *uuid.UUID) (mdl.Organization, error) {
	if err := co.Validate(); err != nil {
		return mdl.Organization{}, fmt.Errorf("validate: %w", err)
	}

	var pgOrg pgorg.Organization
	if err := c.transactor.RunTx(ctx, func(ctx context.Context) error {
		var err error
		pgOrg, err = c.orgStorer.CreateOrganization(ctx, createOrganizationToPg(co))
		if err != nil {
			if errors.Is(err, pgdb.ErrAlreadyExists) {
				return mdl.ErrAlreadyExists
			}
			return fmt.Errorf("create organization: %w", err)
		}

		if _, err := c.orgStorer.CreateProject(ctx, pgorg.CreateProject{OrgID: pgOrg.ID, Name: co.ProjectName}); err != nil {
			if errors.Is(err, pgdb.ErrAlreadyExists) {
				return fmt.Errorf("create default project: %w", mdl.ErrControlProjectNameConflict)
			}
			// The organization was created earlier in this transaction, so sql.ErrNoRows is an
			// impossible state that must remain an internal error.
			return fmt.Errorf("create default project: %w", err)
		}

		if creatorID == nil {
			return nil
		}

		if err := c.orgStorer.AddOrganizationMember(ctx, *creatorID, pgOrg.ID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: %w", errOrgCreatorNotFound, err)
			}
			// pgdb.ErrAlreadyExists is impossible because a new organization cannot already
			// contain its creator membership, so it must remain an internal error.
			return fmt.Errorf("add organization creator as member: %w", err)
		}

		adminRole, err := c.roleBootstrapperStore.CreateOrganizationAdminRole(ctx, pgOrg.ID, permissionsToPg(mdl.OrganizationAdminPermissions()))
		if err != nil {
			// sql.ErrNoRows is impossible because the organization and canonical permissions were
			// established earlier. pgdb.ErrAlreadyExists is impossible because a new organization
			// cannot already contain its managed role. Both must remain internal errors.
			return fmt.Errorf("create organization administrator role: %w", err)
		}

		if err := c.roleBootstrapperStore.AssignCustomRoleToOrg(ctx, *creatorID, adminRole.ExternalID, pgOrg.ID); err != nil {
			// sql.ErrNoRows is impossible because the assignment dependencies were established
			// earlier. pgdb.ErrAlreadyExists is impossible because a new role cannot already be
			// assigned to its creator. Both must remain internal errors.
			return fmt.Errorf("assign organization administrator role: %w", err)
		}

		return nil
	}); err != nil {
		return mdl.Organization{}, err
	}

	return organizationFromPg(pgOrg), nil
}

// CreateProject creates a new project owned by cp.OrgID and returns it.
// Returns [mdl.ErrNotFound] if no organization with that ID exists.
// Returns [mdl.ErrAlreadyExists] if a project with the same name already exists in the organization.
// Returns [mdl.ErrValidation] if cp is invalid.
func (c *Core) CreateProject(ctx context.Context, cp mdl.CreateProject) (mdl.Project, error) {
	if err := cp.Validate(); err != nil {
		return mdl.Project{}, fmt.Errorf("validate: %w", err)
	}

	pgProject, err := c.orgStorer.CreateProject(ctx, createProjectToPg(cp))
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return mdl.Project{}, mdl.ErrNotFound
		case errors.Is(err, pgdb.ErrAlreadyExists):
			return mdl.Project{}, mdl.ErrAlreadyExists
		default:
			return mdl.Project{}, fmt.Errorf("create project: %w", err)
		}
	}

	return projectFromPg(pgProject), nil
}

// OrganizationByName returns the organization with the given name.
// Returns [mdl.ErrNotFound] if no such organization exists.
func (c *Core) OrganizationByName(ctx context.Context, name string) (mdl.Organization, error) {
	pgOrg, err := c.orgStorer.OrganizationByName(ctx, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.Organization{}, mdl.ErrNotFound
		}
		return mdl.Organization{}, fmt.Errorf("organization by name: %w", err)
	}

	return organizationFromPg(pgOrg), nil
}

// ProjectByName returns the project named name owned by orgID.
// Returns [mdl.ErrNotFound] if no such project exists.
func (c *Core) ProjectByName(ctx context.Context, orgID int, name string) (mdl.Project, error) {
	pgProject, err := c.orgStorer.ProjectByName(ctx, orgID, name)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mdl.Project{}, mdl.ErrNotFound
		}
		return mdl.Project{}, fmt.Errorf("project by name: %w", err)
	}

	return projectFromPg(pgProject), nil
}

// AccessibleProjects returns the projects reachable through any role assignment held by the
// authenticated user and the total number of reachable projects.
// Returns [mdl.ErrNotFound] if the authenticated user no longer exists.
func (c *Core) AccessibleProjects(ctx context.Context, filter mdl.ProjectFilter, pageSize, pageOffset int) ([]mdl.Project, int, error) {
	sess, ok := mdl.AuthSessionFromContext(ctx)
	if !ok {
		return nil, 0, errors.New("auth session missing from context")
	}

	pgFilter := projectFilterToPg(filter)

	projects, count, err := c.orgStorer.AccessibleProjects(ctx, sess.User.UserID, pgFilter, pageSize, pageOffset)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, 0, mdl.ErrNotFound
		}
		return nil, 0, fmt.Errorf("accessible projects: %w", err)
	}

	return projectsFromPg(projects), count, nil
}
