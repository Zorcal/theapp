package user

import (
	"fmt"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/order"
	"github.com/zorcal/theapp/backend/pkg/x/slicesx"
)

func updateUserToPG(uu mdl.UpdateUser) pguser.UpdateUser {
	return pguser.UpdateUser{
		ExternalID: uu.ID,
		Fields: pguser.UserUpdateFields{
			Name: uu.Fields.Name,
		},
		Name: uu.Name,
	}
}

func createUserToPG(cu mdl.CreateUser) pguser.CreateUser {
	return pguser.CreateUser{
		Email: cu.Email,
		Name:  cu.Name,
	}
}

func userFromPG(u pguser.User) mdl.User {
	return mdl.User{
		ID:        u.ExternalID,
		Email:     u.Email,
		Name:      u.Name,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
		ETag:      u.ETag.String(),
	}
}

func usersFromPG(us []pguser.User) []mdl.User {
	return slicesx.Map(us, userFromPG)
}

func orderByToPG(o order.By[mdl.UserOrderByField]) (order.By[pguser.OrderByField], error) {
	switch o.Field {
	case mdl.UserOrderByFieldEmail:
		return order.NewBy(pguser.OrderByFieldEmail, o.Direction), nil
	case mdl.UserOrderByFieldCreatedAt:
		return order.NewBy(pguser.OrderByFieldCreatedAt, o.Direction), nil
	case mdl.UserOrderByFieldUpdatedAt:
		return order.NewBy(pguser.OrderByFieldUpdatedAt, o.Direction), nil
	default:
		return order.By[pguser.OrderByField]{}, fmt.Errorf("unknown order by field %q", o.Field)
	}
}

func orderBysToPG(os []order.By[mdl.UserOrderByField]) ([]order.By[pguser.OrderByField], error) {
	pgOrderBys := make([]order.By[pguser.OrderByField], len(os))
	for i, o := range os {
		pgOrderBy, err := orderByToPG(o)
		if err != nil {
			return nil, err
		}
		pgOrderBys[i] = pgOrderBy
	}
	return pgOrderBys, nil
}
