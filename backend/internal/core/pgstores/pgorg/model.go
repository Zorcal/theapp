package pgorg

import "time"

// Organization represents an organization in the database.
type Organization struct {
	ID        int        `db:"id"`
	Name      string     `db:"name"`
	CreatedAt time.Time  `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

// CreateOrganization holds the fields required to create a new organization in the database.
type CreateOrganization struct {
	Name string `db:"name"`
}

// Project represents a project in the database.
type Project struct {
	ID        int        `db:"id"`
	OrgID     int        `db:"org_id"`
	Name      string     `db:"name"`
	CreatedAt time.Time  `db:"created_at"`
	UpdatedAt *time.Time `db:"updated_at"`
}

// CreateProject holds the fields required to create a new project in the database.
type CreateProject struct {
	OrgID int    `db:"org_id"`
	Name  string `db:"name"`
}
