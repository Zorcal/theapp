package validate

import (
	"testing"

	"google.golang.org/grpc/codes"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/conv"
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
)

func TestListProjects(t *testing.T) {
	if err := ListProjects(&pb.ListProjectsRequest{}); err != nil {
		t.Errorf("ListProjects() error = %v, want nil", err)
	}
}

func TestListProjects_error(t *testing.T) {
	pageToken, err := conv.EncodePageToken(1, "", &pb.ProjectFilter{Name: "first"})
	if err != nil {
		t.Fatalf("EncodePageToken() error = %v", err)
	}

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
		{
			name: "page token filter mismatch",
			in: &pb.ListProjectsRequest{
				PageToken: pageToken,
				Filter:    &pb.ProjectFilter{Name: "second"},
			},
			want: wantInvalidArgument("page_token filter mismatch"),
		},
	}
	runValidationErrorTests(t, "ListProjects", ListProjects, tests)
}
