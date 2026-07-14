package grpc

import (
	"context"

	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
)

type contextKeyUserID struct{}

func contextWithUserID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, contextKeyUserID{}, id)
}

// UserIDFromContext extracts the authenticated user's ID from ctx.
// Returns the zero UUID and false when no user ID is present (unauthenticated request).
func UserIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	id, ok := ctx.Value(contextKeyUserID{}).(uuid.UUID)
	return id, ok && id != uuid.Nil
}

type contextKeyAuthSession struct{}

func contextWithAuthSession(ctx context.Context, s mdl.AuthSession) context.Context {
	return context.WithValue(ctx, contextKeyAuthSession{}, s)
}

// AuthSessionFromContext extracts the resolved auth session from ctx.
// Returns the zero AuthSession and false when no session is present (unauthenticated request).
func AuthSessionFromContext(ctx context.Context) (mdl.AuthSession, bool) {
	s, ok := ctx.Value(contextKeyAuthSession{}).(mdl.AuthSession)
	return s, ok
}
