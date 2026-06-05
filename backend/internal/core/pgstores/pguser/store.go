// Package pguser provides user db access functionality.
package pguser

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{
		pool: pool,
	}
}

func (s *Store) Users(ctx context.Context, pageSize, pageOffset int) ([]User, error) {
	return nil, errors.New("not implemented")
}

func (s *Store) UserCount(ctx context.Context, pageSize, pageOffset int) ([]User, error) {
	return nil, errors.New("not implemented")
}
