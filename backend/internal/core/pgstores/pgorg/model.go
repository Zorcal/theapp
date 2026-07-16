package pgorg

import "time"

// Organization represents an organization in the database.
type Organization struct {
	ID               int        `db:"id"`
	Name             string     `db:"name"`
	ControlProjectID int        `db:"control_project_id"`
	CreatedAt        time.Time  `db:"created_at"`
	UpdatedAt        *time.Time `db:"updated_at"`
}

// CreateOrganization holds the fields required to create a new organization in the database. It
// also seeds a control project named ControlProjectName, owned by the new organization.
type CreateOrganization struct {
	Name               string
	ControlProjectName string
}

// Project represents a project in the database.
type Project struct {
	ID        int        `db:"id"`
	OrgID     int        `db:"org_id"`
	Name      string     `db:"name"`
	IsControl bool       `db:"is_control"`
	CreatedAt time.Time  `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

// CreateProject holds the fields required to create a new project in the database.
type CreateProject struct {
	OrgID int
	Name  string
}
