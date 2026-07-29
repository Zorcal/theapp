package validate

import (
	"testing"

	"google.golang.org/grpc/codes"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
)

func TestListProjects(t *testing.T) {
	if err := ListProjects(&pb.ListProjectsRequest{}); err != nil {
		t.Errorf("ListProjects() error = %v, want nil", err)
	}
}

func TestListProjects_error(t *testing.T) {
	tests := []validationTest[*pb.ListProjectsRequest]{
		{
			name: "nil request",
			in:   nil,
			want: wantInvalidArgument(codes.InvalidArgument.String(), violation{
				field:       "request",
				description: "required",
			}),
		},
		{
			name: "invalid page token",
			in:   &pb.ListProjectsRequest{PageToken: "invalid"},
			want: wantInvalidArgument("invalid page_token"),
		},
	}
	runValidationErrorTests(t, "ListProjects", ListProjects, tests)
}
