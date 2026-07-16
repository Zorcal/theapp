package org

import (
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgorg"
)

func createOrganizationToPg(co mdl.CreateOrganization) pgorg.CreateOrganization {
	return pgorg.CreateOrganization{
		Name: co.Name,
	}
}

func organizationFromPg(o pgorg.Organization) mdl.Organization {
	return mdl.Organization{
		ID:        o.ID,
		Name:      o.Name,
		CreatedAt: o.CreatedAt,
		UpdatedAt: o.UpdatedAt,
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
		CreatedAt: p.CreatedAt,
		UpdatedAt: p.UpdatedAt,
	}
}
