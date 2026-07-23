package commands

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/zorcal/theapp/backend/internal/core/rbac"
	"github.com/zorcal/theapp/backend/internal/core/user"
)

// NewRoleCommand returns the "role" command group, backed by userCore and rbacCore.
func NewRoleCommand(userCore *user.Core, rbacCore *rbac.Core) *cli.Command {
	return &cli.Command{
		Name:  "role",
		Usage: "Manage role assignments",
		Commands: []*cli.Command{
			newRoleAssignSystemCommand(userCore, rbacCore),
		},
	}
}

func newRoleAssignSystemCommand(userCore *user.Core, rbacCore *rbac.Core) *cli.Command {
	return &cli.Command{
		Name:  "assign-system",
		Usage: "Assign a system role to a user",
		Flags: []cli.Flag{
			operatorFlag(),
			&cli.StringFlag{
				Name:     "user",
				Usage:    "user to assign the role to, identified by UUID or email",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "role",
				Usage:    "name of the system role to assign, e.g. superadmin",
				Required: true,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if err := validateOperator(ctx, cmd, userCore); err != nil {
				return fmt.Errorf("resolve operator: %w", err)
			}

			ref := cmd.String("user")
			u, err := resolveUser(ctx, userCore, ref)
			if err != nil {
				return fmt.Errorf("resolve user %q: %w", ref, err)
			}

			roleName := cmd.String("role")
			if err := rbacCore.AssignSystemRole(ctx, u.ID, roleName); err != nil {
				return fmt.Errorf("assign system role: %w", err)
			}

			_, err = fmt.Fprintf(cmd.Writer, "assigned %s to %s (%s)\n", roleName, u.Email, u.ID)
			return err
		},
	}
}
