// Package org provides the core business logic for the organization and project domain.
package org

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgorg"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
)

//go:generate moq -rm -fmt goimports -out org_storer_moq_test.go . OrgStorer:MockedOrgStorer

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
	// OrganizationByName returns the organization with the given name.
	// Returns [sql.ErrNoRows] if no such organization exists.
	OrganizationByName(ctx context.Context, name string) (pgorg.Organization, error)
	// ProjectByName returns the project named name owned by orgID.
	// Returns [sql.ErrNoRows] if no such project exists.
	ProjectByName(ctx context.Context, orgID int, name string) (pgorg.Project, error)
}

// Transactor runs a function inside a database transaction.
type Transactor interface {
	RunTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// Core holds the business logic for the organization and project domain.
type Core struct {
	orgStorer  OrgStorer
	transactor Transactor
}

// NewCore constructs a Core backed by the provided OrgStorer and Transactor.
func NewCore(os OrgStorer, tr Transactor) *Core {
	return &Core{orgStorer: os, transactor: tr}
}

// CreateOrganization creates a new organization, along with a default project named
// co.ProjectName and a control project, and returns the created organization.
// Returns [mdl.ErrAlreadyExists] if an organization with the same name already exists.
// Returns [mdl.ErrControlProjectNameConflict] if co.ProjectName collides with the org's control project.
// Returns [mdl.ErrValidation] if co is invalid.
func (c *Core) CreateOrganization(ctx context.Context, co mdl.CreateOrganization) (mdl.Organization, error) {
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
				return mdl.ErrControlProjectNameConflict
			}
			return fmt.Errorf("create default project: %w", err)
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
