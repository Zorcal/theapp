package mdl

import "time"

// Organization represents a tenant.
type Organization struct {
	ID        int
	Name      string
	CreatedAt time.Time
	UpdatedAt *time.Time
}

// CreateOrganization holds the fields needed to create a new organization. Creating an
// organization also seeds a default project, named ProjectName.
type CreateOrganization struct {
	Name        string
	ProjectName string
}

func (co CreateOrganization) Validate() error {
	if co.Name == "" {
		return validationError("name required")
	}
	if co.ProjectName == "" {
		return validationError("project name required")
	}
	return nil
}

// Project represents a project within an organization.
type Project struct {
	ID        int
	OrgID     int
	Name      string
	CreatedAt time.Time
	UpdatedAt *time.Time
}

// CreateProject holds the fields needed to create a new project.
type CreateProject struct {
	OrgID int
	Name  string
}

func (cp CreateProject) Validate() error {
	if cp.OrgID <= 0 {
		return validationError("org id required")
	}
	if cp.Name == "" {
		return validationError("name required")
	}
	return nil
}
