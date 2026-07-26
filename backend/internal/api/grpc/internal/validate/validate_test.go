package validate

import (
	"testing"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/testing/protocmp"

	"github.com/zorcal/theapp/backend/internal/testingx"
)

type validationTest[T any] struct {
	name string
	in   T
	want *status.Status
}

func runValidationErrorTests[T any](
	t *testing.T,
	funcName string,
	fn func(T) error,
	tests []validationTest[T],
) {
	t.Helper()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fn(tt.in)
			if err == nil {
				t.Fatalf("%s() error = nil, want error", funcName)
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("%s() error = %q, want a gRPC status error", funcName, err)
			}

			testingx.AssertDiff(t, got.Proto(), tt.want.Proto(), protocmp.Transform())
		})
	}
}

type violation struct {
	field       string
	description string
}

func wantInvalidArgument(message string, violations ...violation) *status.Status {
	st := status.New(codes.InvalidArgument, message)
	if len(violations) == 0 {
		return st
	}

	fieldViolations := make([]*errdetails.BadRequest_FieldViolation, 0, len(violations))
	for _, violation := range violations {
		fieldViolations = append(fieldViolations, &errdetails.BadRequest_FieldViolation{
			Field:       violation.field,
			Description: violation.description,
		})
	}

	withDetails, err := st.WithDetails(&errdetails.BadRequest{FieldViolations: fieldViolations})
	if err != nil {
		panic(err)
	}

	return withDetails
}

func idValidationTests[T any](nilRequest, invalidID T) []validationTest[T] {
	return []validationTest[T]{
		{
			name: "nil request",
			in:   nilRequest,
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "id",
				description: "required",
			}),
		},
		{
			name: "invalid id",
			in:   invalidID,
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "id",
				description: "must be a valid UUID",
			}),
		},
	}
}
