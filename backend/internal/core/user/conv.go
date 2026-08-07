package user

import (
	"fmt"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pguser"
	"github.com/zorcal/theapp/backend/internal/data/order"
	"github.com/zorcal/theapp/backend/pkg/x/slicesx"
)

func updateUserToPg(uu mdl.UpdateUser) pguser.UpdateUser {
	return pguser.UpdateUser{
		ExternalID: uu.ID,
		Fields: pguser.UserUpdateFields{
			Name: uu.Fields.Name,
		},
		Name: uu.Name,
	}
}

func createUserToPg(cu mdl.CreateUser) pguser.CreateUser {
	return pguser.CreateUser{
		Email: cu.Email,
		Name:  cu.Name,
	}
}

func userFromPg(u pguser.User) mdl.User {
	return mdl.User{
		ID:              u.ExternalID,
		Email:           u.Email,
		Name:            u.Name,
		EmailVerifiedAt: u.EmailVerifiedAt,
		CreatedAt:       u.CreatedAt,
		UpdatedAt:       u.UpdatedAt,
		ETag:            u.ETag.String(),
	}
}

func usersFromPg(us []pguser.User) []mdl.User {
	return slicesx.Map(us, userFromPg)
}

func orderByToPg(o order.By[mdl.UserOrderByField]) (order.By[pguser.OrderByField], error) {
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

func filterToPg(f mdl.UserFilter) pguser.Filter {
	return pguser.Filter{Email: f.Email, Name: f.Name}
}

func orderBysToPg(os []order.By[mdl.UserOrderByField]) ([]order.By[pguser.OrderByField], error) {
	pgOrderBys := make([]order.By[pguser.OrderByField], len(os))
	for i, o := range os {
		pgOrderBy, err := orderByToPg(o)
		if err != nil {
			return nil, err
		}
		pgOrderBys[i] = pgOrderBy
	}
	return pgOrderBys, nil
}
