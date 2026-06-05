// Package pguser provides user db access functionality.
package pguser

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/zorcal/theapp/backend/internal/data/order"
	"github.com/zorcal/theapp/backend/internal/data/pgdb"
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
	var users []User

	usersQ := queryUsersQuery(orderBys, pageSize, pageOffset)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := usersQ.QueueMany(ctx, b, &users); err != nil {
			return fmt.Errorf("query users: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return nil, err
	}

	return users, nil
}

func (s *Store) UserCount(ctx context.Context) (int, error) {
	var count int

	countQ := userCountQuery()

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := countQ.Queue(ctx, b, &count); err != nil {
			return fmt.Errorf("user count: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return 0, err
	}

	return count, nil
}

func (s *Store) InsertUser(ctx context.Context, cu CreateUser) (User, error) {
	var user User

	insertQ := insertUserQuery(cu)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := insertQ.Queue(ctx, b, &user); err != nil {
			return fmt.Errorf("insert user: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return User{}, err
	}

	return user, nil
}
