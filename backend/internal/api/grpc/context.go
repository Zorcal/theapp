package grpc

import "context"

type contextKeyProjectID struct{}

// contextWithProjectID returns a copy of ctx carrying id as the request's target project.
func contextWithProjectID(ctx context.Context, id int) context.Context {
	return context.WithValue(ctx, contextKeyProjectID{}, id)
}

// projectIDFromContext extracts the request's target project ID from ctx.
// Returns 0 and false when no project ID is present (e.g. a project-less request).
func projectIDFromContext(ctx context.Context) (int, bool) {
	id, ok := ctx.Value(contextKeyProjectID{}).(int)
	return id, ok
}
