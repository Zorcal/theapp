package rbac

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgorg"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgrbac"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
)

type immediateTransactor struct{}

func (immediateTransactor) RunTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func contextWithAuthUser(ctx context.Context, userID uuid.UUID) context.Context {
	return mdl.ContextWithAuthSession(ctx, mdl.AuthSession{User: mdl.AuthUser{UserID: userID}})
}

func seededSystemRoles() []mdl.SystemRole {
	return []mdl.SystemRole{
		{Name: "superadmin", Permissions: mdl.AllPermissions()},
	}
}

func seededSystemRole(t *testing.T, name string) mdl.SystemRole {
	t.Helper()

	roles := seededSystemRoles()

	roleIdx := slices.IndexFunc(roles, func(role mdl.SystemRole) bool { return role.Name == name })
	if roleIdx == -1 {
		t.Fatalf("slices.IndexFunc(seededSystemRoles(), %q) = -1, want an index", name)
	}

	return roles[roleIdx]
}

func seedUser(t *testing.T, ctx context.Context, store *pguser.Store, email, name string) pguser.User {
	t.Helper()

	user, err := store.CreateUser(ctx, pguser.CreateUser{Email: email, Name: name})
	if err != nil {
		t.Fatalf("seed user %q: %v", email, err)
	}

	return user
}

func seedSystemRoleAssignment(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID int, roleName string) {
	t.Helper()

	params := pgx.NamedArgs{"user_id": userID, "role_name": roleName}
	const query = `
		INSERT INTO rbac.system_role_assignments (user_id, role_id)
		SELECT @user_id, id
		FROM rbac.system_roles
		WHERE name = @role_name`
	if _, err := pool.Exec(ctx, query, params); err != nil {
		t.Fatalf("seed system role assignment for user %d: %v", userID, err)
	}
}

func seedOrg(t *testing.T, orgStore *pgorg.Store, name string) pgorg.Organization {
	t.Helper()

	org, err := orgStore.CreateOrganization(t.Context(), pgorg.CreateOrganization{
		Name:               name,
		ControlProjectName: "control",
	})
	if err != nil {
		t.Fatalf("seed organization %q: %v", name, err)
	}

	return org
}

func seedProject(t *testing.T, ctx context.Context, orgStore *pgorg.Store, orgID int, name string) pgorg.Project {
	t.Helper()

	project, err := orgStore.CreateProject(ctx, pgorg.CreateProject{
		OrgID: orgID,
		Name:  name,
	})
	if err != nil {
		t.Fatalf("seed project %q: %v", name, err)
	}

	return project
}

func seedCustomRole(t *testing.T, ctx context.Context, roleStore *pgrbac.Store, orgID int, name string, perms []mdl.Permission) pgrbac.CustomRole {
	t.Helper()

	role, err := roleStore.CreateCustomRole(ctx, pgrbac.CreateCustomRole{
		OrgID:           orgID,
		Name:            name,
		PermissionNames: permissionsToPg(perms),
	})
	if err != nil {
		t.Fatalf("seed custom role %q: %v", name, err)
	}

	return role
}

func seedOrgMembership(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID, orgID int) {
	t.Helper()

	if _, err := pool.Exec(ctx, "INSERT INTO org.org_membership (user_id, org_id) VALUES ($1, $2)", userID, orgID); err != nil {
		t.Fatalf("seed org membership (user %d, org %d): %v", userID, orgID, err)
	}
}

func seedOrgScopedRoleWithAllPermissions(t *testing.T, ctx context.Context, pool *pgxpool.Pool, orgStore *pgorg.Store, rbacStore *pgrbac.Store, userID int) {
	t.Helper()

	org, err := orgStore.CreateOrganization(ctx, pgorg.CreateOrganization{Name: "acme", ControlProjectName: "control"})
	if err != nil {
		t.Fatalf("seed organization: %v", err)
	}
	seedOrgMembership(t, ctx, pool, userID, org.ID)

	// Give an org-scoped custom role every permission in the database, the same set seeded for superadmin.
	role, err := rbacStore.CreateCustomRole(ctx, pgrbac.CreateCustomRole{
		OrgID:           org.ID,
		Name:            "org-scoped-admin",
		PermissionNames: permissionsToPg(mdl.AllPermissions()),
	})
	if err != nil {
		t.Fatalf("seed org-scoped role: %v", err)
	}

	const query = `
		INSERT INTO rbac.org_role_assignments (user_id, role_id, org_id)
		VALUES ($1, $2, $3)`
	if _, err := pool.Exec(ctx, query, userID, role.ID, org.ID); err != nil {
		t.Fatalf("seed org-scoped role assignment: %v", err)
	}
}
