package mdl

import "time"

// SystemOrgName is the name of the well-known organization that anchors system administration.
const SystemOrgName = "theapp"

// Organization represents a tenant.
type Organization struct {
	ID               int
	Name             string
	ControlProjectID int
	CreatedAt        time.Time
	UpdatedAt        *time.Time
}

// CreateOrganization holds the fields needed to create a new organization. Creating an
// organization also seeds a default project, named ProjectName, and a control project.
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

// Project represents a project within an organization. IsControl marks the one project every
// organization is automatically given alongside itself, used to anchor permission checks (e.g.
// org:create, project:create) that have no project of their own to check against yet.
type Project struct {
	ID        int
	OrgID     int
	Name      string
	IsControl bool
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
