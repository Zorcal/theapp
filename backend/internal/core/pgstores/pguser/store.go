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

// SoftDeleteUser marks userID as deleted.
// Returns [sql.ErrNoRows] if userID does not identify an active user.
func (s *Store) SoftDeleteUser(ctx context.Context, userID uuid.UUID) error {
	q := softDeleteUserQuery(userID)

	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		var idSink int
		if err := q.Queue(ctx, b, &idSink); err != nil {
			return fmt.Errorf("soft delete user: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return err
	}

	return nil
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{
		pool: pool,
	}
}

// GetOrCreateUserByEmail returns the active user with the given email, creating one if none exists.
// Returns [sql.ErrNoRows] if the email belongs to a deleted user.
func (s *Store) GetOrCreateUserByEmail(ctx context.Context, email string) (User, error) {
	q := getOrCreateUserByEmailQuery(email)

	var user User
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

// UserByEmail returns the active user with the given email address.
// Returns [sql.ErrNoRows] if no such active user exists.
func (s *Store) UserByEmail(ctx context.Context, email string) (User, error) {
	q := userByEmailQuery(email)

	var user User
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

// UserByExternalID returns the active user with the given external ID.
// Returns [sql.ErrNoRows] if no such active user exists.
func (s *Store) UserByExternalID(ctx context.Context, id uuid.UUID) (User, error) {
	q := userByExternalIDQuery(id)

	var user User
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

// Users returns a page of active users and the total count of matching active users.
func (s *Store) Users(ctx context.Context, filter Filter, orderBys []order.By[OrderByField], pageSize, pageOffset int) ([]User, int, error) {
	usersQ := usersQuery(filter, orderBys, pageSize, pageOffset)
	countQ := userCountQuery(filter)

	var (
		users []User
		count int
	)
	doInBatch := func(ctx context.Context, b *pgdb.Batch) error {
		if err := usersQ.QueueMany(ctx, b, &users); err != nil {
			return fmt.Errorf("query users: %w", err)
		}
		if err := countQ.Queue(ctx, b, &count); err != nil {
			return fmt.Errorf("user count: %w", err)
		}
		return nil
	}

	if err := pgdb.RunBatch(ctx, s.pool, doInBatch); err != nil {
		return nil, 0, err
	}

	return users, count, nil
}

// UpdateUser updates the active user with the given external ID and returns the updated user.
// Returns [sql.ErrNoRows] if no such active user exists.
func (s *Store) UpdateUser(ctx context.Context, uu UpdateUser) (User, error) {
	updateQ := updateUserQuery(uu)

	var user User
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

// MarkEmailVerified sets email_verified_at to the current time for the active user with the given
// external ID. Returns [sql.ErrNoRows] if no such active user exists.
func (s *Store) MarkEmailVerified(ctx context.Context, externalID uuid.UUID) error {
	q := markEmailVerifiedQuery(externalID)

	var user User
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
	insertQ := createUserQuery(cu)

	var user User
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
