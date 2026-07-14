package commands

import (
	"context"
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/user"
)

const defaultUserListPageSize = 20

// NewUserCommand returns the "user" command group, backed by userCore.
func NewUserCommand(userCore *user.Core) *cli.Command {
	return &cli.Command{
		Name:  "user",
		Usage: "Manage users",
		Commands: []*cli.Command{
			newUserCreateCommand(userCore),
			newUserListCommand(userCore),
		},
	}
}

func newUserCreateCommand(userCore *user.Core) *cli.Command {
	return &cli.Command{
		Name:  "create",
		Usage: "Create a new user",
		Flags: []cli.Flag{
			operatorFlag(),
			&cli.StringFlag{
				Name:     "email",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "name",
				Required: true,
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if err := validateOperator(ctx, cmd, userCore); err != nil {
				return fmt.Errorf("resolve operator: %w", err)
			}

			u, err := userCore.CreateUser(ctx, mdl.CreateUser{
				Email: cmd.String("email"),
				Name:  cmd.String("name"),
			})
			if err != nil {
				return fmt.Errorf("create user: %w", err)
			}

			_, err = fmt.Fprintf(cmd.Writer, "created user %s (%s)\n", u.Email, u.ID)
			return err
		},
	}
}

func newUserListCommand(userCore *user.Core) *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List users",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "page-size",
				Usage: "max users to return",
				Value: defaultUserListPageSize,
			},
			&cli.IntFlag{
				Name:  "page",
				Usage: "page number, starting at 1",
				Value: 1,
			},
			&cli.StringFlag{
				Name:  "email",
				Usage: "filter by email prefix",
			},
			&cli.StringFlag{
				Name:  "name",
				Usage: "filter by name prefix",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			pageSize := cmd.Int("page-size")
			page := cmd.Int("page")
			if page < 1 {
				return fmt.Errorf("page %d: must be >= 1", page)
			}
			offset := (page - 1) * pageSize

			fltr := mdl.UserFilter{
				Email: cmd.String("email"),
				Name:  cmd.String("name"),
			}
			usrs, total, err := userCore.Users(ctx, fltr, nil, pageSize, offset)
			if err != nil {
				return fmt.Errorf("list users: %w", err)
			}

			w := tabwriter.NewWriter(cmd.Writer, 0, 0, 2, ' ', 0)
			if _, err := fmt.Fprintln(w, "EMAIL\tNAME\tID\tCREATED AT"); err != nil {
				return fmt.Errorf("write header: %w", err)
			}
			for _, u := range usrs {
				if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", u.Email, u.Name, u.ID, u.CreatedAt.Format(time.RFC3339)); err != nil {
					return fmt.Errorf("write user row: %w", err)
				}
			}
			if err := w.Flush(); err != nil {
				return fmt.Errorf("flush output: %w", err)
			}

			if len(usrs) == 0 {
				_, err = fmt.Fprintf(cmd.Writer, "\n0 of %d\n", total)
				return err
			}

			_, err = fmt.Fprintf(cmd.Writer, "\n%d-%d of %d\n", offset+1, offset+len(usrs), total)
			return err
		},
	}
}
