// Package pguser provides user db access functionality.
package pguser

import (
	"context"
	"fmt"

	"github.com/google/uuid"
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

// GetOrCreateUserByEmail returns the user with the given email, creating one if none exists.
func (s *Store) GetOrCreateUserByEmail(ctx context.Context, email string) (User, error) {
	var user User

	q := getOrCreateUserByEmailQuery(email)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &user); err != nil {
			return fmt.Errorf("get or create user by email: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return User{}, err
	}

	return user, nil
}

// UserByEmail returns the user with the given email address.
// Returns [sql.ErrNoRows] if no such user exists.
func (s *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	var user User

	q := userByEmailQuery(email)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &user); err != nil {
			return fmt.Errorf("user by email: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return User{}, err
	}

	return user, nil
}

// UserByExternalID returns the user with the given external ID.
// Returns [sql.ErrNoRows] if no such user exists.
func (s *Store) UserByExternalID(ctx context.Context, id uuid.UUID) (User, error) {
	var user User

	q := userByExternalIDQuery(id)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &user); err != nil {
			return fmt.Errorf("user by external id: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return User{}, err
	}

	return user, nil
}

func (s *Store) Users(ctx context.Context, filter Filter, orderBys []order.By[OrderByField], pageSize, pageOffset int) ([]User, error) {
	var users []User

	usersQ := usersQuery(filter, orderBys, pageSize, pageOffset)

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

func (s *Store) UserCount(ctx context.Context, filter Filter) (int, error) {
	var count int

	countQ := userCountQuery(filter)

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

// UpdateUser updates the user with the given external ID and returns the updated user.
// Returns [sql.ErrNoRows] if no such user exists.
func (s *Store) UpdateUser(ctx context.Context, uu UpdateUser) (User, error) {
	var user User

	updateQ := updateUserQuery(uu)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := updateQ.Queue(ctx, b, &user); err != nil {
			return fmt.Errorf("update user: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return User{}, err
	}

	return user, nil
}

// MarkEmailVerified sets email_verified_at to the current time for the user with the given external ID.
// Returns [sql.ErrNoRows] if no such user exists.
func (s *Store) MarkEmailVerified(ctx context.Context, externalID uuid.UUID) error {
	var user User

	q := markEmailVerifiedQuery(externalID)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := q.Queue(ctx, b, &user); err != nil {
			return fmt.Errorf("mark email verified: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return err
	}

	return nil
}

func (s *Store) CreateUser(ctx context.Context, cu CreateUser) (User, error) {
	var user User

	insertQ := createUserQuery(cu)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := insertQ.Queue(ctx, b, &user); err != nil {
			return fmt.Errorf("create user: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return User{}, err
	}

	return user, nil
}
