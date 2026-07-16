package org

import (
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgorg"
)

// defaultControlProjectName is the initial name given to a new organization's control project.
// It's only a label — the project's identity as the control project is tracked structurally
// (pgorg.Project.IsControl), so this is free to be renamed later.
const defaultControlProjectName = "control"

func createOrganizationToPg(co mdl.CreateOrganization) pgorg.CreateOrganization {
	return pgorg.CreateOrganization{
		Name:               co.Name,
		ControlProjectName: defaultControlProjectName,
	}
}

func organizationFromPg(o pgorg.Organization) mdl.Organization {
	return mdl.Organization{
		ID:               o.ID,
		Name:             o.Name,
		ControlProjectID: o.ControlProjectID,
		CreatedAt:        o.CreatedAt,
		UpdatedAt:        o.UpdatedAt,
	}
}

func createProjectToPg(cp mdl.CreateProject) pgorg.CreateProject {
	return pgorg.CreateProject{
		OrgID: cp.OrgID,
		Name:  cp.Name,
	}
}

func projectFromPg(p pgorg.Project) mdl.Project {
	return mdl.Project{
		ID:        p.ID,
		OrgID:     p.OrgID,
		Name:      p.Name,
		IsControl: p.IsControl,
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}
