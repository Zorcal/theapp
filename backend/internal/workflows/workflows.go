// Package workflows provides shared infrastructure for DBOS workflow execution.
package workflows

import "context"

type contextKeyWorkflowID struct{}

// WithWorkflowID returns a copy of ctx carrying id as the workflow ID. When id is non-empty, workflow implementations
// should use it as the DBOS workflow ID so that retrying with the same key deduplicates execution.
func WithWorkflowID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, contextKeyWorkflowID{}, id)
}

// WorkflowID returns the workflow ID stored in ctx, or "" if none is set.
func WorkflowID(ctx context.Context) string {
	v, ok := ctx.Value(contextKeyWorkflowID{}).(string)
	if !ok {
		return ""
	}
	return v
}
