package grpc

import (
	"context"

	"github.com/google/uuid"
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
