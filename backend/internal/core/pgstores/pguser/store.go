// Package pguser provides user db access functionality.
package pguser

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/zorcal/theapp/backend/internal/core/data/order"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{
		pool: pool,
	}
}

func (s *Store) QueryUsers(ctx context.Context, orderBys []order.By[OrderByField], pageSize, pageOffset int) ([]User, error) {
	return nil, errors.New("not implemented")
}

func (s *Store) UserCount(ctx context.Context) ([]User, error) {
	return nil, errors.New("not implemented")
}

func (s *Store) InsertUser(ctx context.Context, cu CreateUser) (User, error) {
	return User{}, errors.New("not implemented")
}
