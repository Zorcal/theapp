package validate

import (
	"testing"

	"google.golang.org/grpc/codes"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
)

func TestListPermissions(t *testing.T) {
	if err := ListPermissions(&pb.ListPermissionsRequest{}); err != nil {
		t.Errorf("ListPermissions() error = %v, want nil", err)
	}
}

func TestListPermissions_error(t *testing.T) {
	tests := []validationTest[*pb.ListPermissionsRequest]{
		{
			name: "nil request",
			in:   nil,
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "request",
				description: "required",
			}),
		},
	}
	runValidationErrorTests(t, "ListPermissions", ListPermissions, tests)
}
