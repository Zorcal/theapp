// Package commands holds the cmd/cli CLI's subcommands, one file per resource.
package commands

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/urfave/cli/v3"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/user"
)

const operatorFlagName = "operator"

// operatorFlag is required on every mutating command for actor attribution.
func operatorFlag() *cli.StringFlag {
	return &cli.StringFlag{
		Name:     operatorFlagName,
		Usage:    "user performing this action, identified by UUID or email",
		Required: true,
	}
}

// contextWithOperator resolves the --operator flag and returns a context carrying that user as the
// current actor.
// Returns [mdl.ErrNotFound] if no such user exists.
func contextWithOperator(ctx context.Context, cmd *cli.Command, userCore *user.Core) (context.Context, error) {
	ref := cmd.String(operatorFlagName)

	operator, err := resolveUser(ctx, userCore, ref)
	if err != nil {
		return nil, fmt.Errorf("resolve user %q: %w", ref, err)
	}

	return mdl.ContextWithAuthSession(ctx, mdl.AuthSession{
		User: mdl.AuthUser{UserID: operator.ID, Email: operator.Email},
	}), nil
}

// resolveUser looks up a user by UUID or email, trying UUID first.
// Returns [mdl.ErrNotFound] if no such user exists.
func resolveUser(ctx context.Context, userCore *user.Core, ref string) (mdl.User, error) {
	if id, err := uuid.Parse(ref); err == nil {
		u, err := userCore.UserByID(ctx, id)
		if err != nil {
			return mdl.User{}, fmt.Errorf("look up user by id: %w", err)
		}
		return u, nil
	}

	u, err := userCore.UserByEmail(ctx, ref)
	if err != nil {
		return mdl.User{}, fmt.Errorf("look up user by email: %w", err)
	}
	return u, nil
}
