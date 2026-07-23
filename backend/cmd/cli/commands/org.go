package commands

import (
	"context"
	"errors"
	"fmt"

	"github.com/urfave/cli/v3"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/org"
	"github.com/zorcal/theapp/backend/internal/core/user"
)

const bootstrapDevProjectName = "dev"

// NewOrgCommand returns the "org" command group, backed by userCore and orgCore.
func NewOrgCommand(userCore *user.Core, orgCore *org.Core) *cli.Command {
	return &cli.Command{
		Name:  "org",
		Usage: "Manage organizations",
		Commands: []*cli.Command{
			newOrgBootstrapCommand(userCore, orgCore),
		},
	}
}

func newOrgBootstrapCommand(userCore *user.Core, orgCore *org.Core) *cli.Command {
	return &cli.Command{
		Name:  "bootstrap",
		Usage: fmt.Sprintf("Ensure the %s organization, its %s project, and its control project exist", mdl.SystemOrgName, bootstrapDevProjectName),
		Flags: []cli.Flag{
			operatorFlag(),
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			if err := validateOperator(ctx, cmd, userCore); err != nil {
				return fmt.Errorf("resolve operator: %w", err)
			}

			theapp, err := ensureOrganization(ctx, orgCore, mdl.SystemOrgName, bootstrapDevProjectName)
			if err != nil {
				return fmt.Errorf("ensure organization: %w", err)
			}

			_, err = fmt.Fprintf(cmd.Writer, "organization %s (%d), control project %d\n", theapp.Name, theapp.ID, theapp.ControlProjectID)
			return err
		},
	}
}

// ensureOrganization returns the organization named name, creating it (with a default project
// named defaultProjectName and a control project) if it doesn't already exist.
func ensureOrganization(ctx context.Context, orgCore *org.Core, name, defaultProjectName string) (mdl.Organization, error) {
	o, err := orgCore.OrganizationByName(ctx, name)
	if err != nil {
		if !errors.Is(err, mdl.ErrNotFound) {
			return mdl.Organization{}, fmt.Errorf("look up organization %q: %w", name, err)
		}

		o, err = orgCore.CreateOrganization(ctx, mdl.CreateOrganization{Name: name, ProjectName: defaultProjectName})
		if err != nil {
			return mdl.Organization{}, fmt.Errorf("create organization %q: %w", name, err)
		}
	}

	return o, nil
}
