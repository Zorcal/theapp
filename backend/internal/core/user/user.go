// Package user provides the core business logic for the user domain.
package user

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
)

var users = []mdl.User{
	{
		ID:        uuid.New(),
		Email:     "john.doe@test.com",
		CreatedAt: time.Now().AddDate(0, 0, -15),
		ETag:      uuid.NewString(),
	},
	{
		ID:        uuid.New(),
		Email:     "mary.doe@test.com",
		CreatedAt: time.Now().AddDate(0, 0, -12),
		UpdatedAt: new(time.Now().AddDate(0, 0, -3)),
		ETag:      uuid.NewString(),
	},
	{
		ID:        uuid.New(),
		Email:     "smith.brown@test.com",
		CreatedAt: time.Now().AddDate(0, 0, -10),
		UpdatedAt: new(time.Now().AddDate(0, 0, -1)),
		ETag:      uuid.NewString(),
	},
}

type Core struct{}

func NewCore() *Core {
	return &Core{}
}

func (c *Core) ListUsers(ctx context.Context, fltr mdl.UserFilter, orderBys []mdl.OrderBy[mdl.UserOrderByField], pageSize, pageOffset int) (usrs []mdl.User, totalCount int, err error) {
	if pageOffset > len(users) {
		return []mdl.User{}, len(users), nil
	}

	return users[pageOffset:min(pageOffset+pageSize, len(users))], len(users), nil
}
