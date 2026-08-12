package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/conv"
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestProjectService_ListProjects(t *testing.T) {
	diffOpts := defaultDiffOpts()
	now := time.Now()

	firstProject := mdl.Project{
		ID:        1,
		OrgID:     2,
		Name:      "control",
		IsControl: true,
		CreatedAt: now.AddDate(0, 0, -2),
		UpdatedAt: new(now.AddDate(0, 0, -1)),
		ETag:      uuid.New(),
	}
	secondProject := mdl.Project{
		ID:        3,
		OrgID:     2,
		Name:      "widgets",
		IsControl: false,
		CreatedAt: now,
		UpdatedAt: nil,
		ETag:      uuid.New(),
	}
	pbFirstProject := &pb.Project{
		Id:             1,
		OrganizationId: 2,
		Name:           firstProject.Name,
		IsControl:      firstProject.IsControl,
		CreateTime:     timestamppb.New(firstProject.CreatedAt),
		UpdateTime:     timestamppb.New(*firstProject.UpdatedAt),
		Etag:           firstProject.ETag.String(),
	}
	pbSecondProject := &pb.Project{
		Id:             3,
		OrganizationId: 2,
		Name:           secondProject.Name,
		IsControl:      secondProject.IsControl,
		CreateTime:     timestamppb.New(secondProject.CreatedAt),
		Etag:           secondProject.ETag.String(),
	}

	tests := []struct {
		name        string
		projectCore ProjectCore
		in          *pb.ListProjectsRequest
		want        *pb.ListProjectsResponse
	}{
		{
			name: "empty request",
			projectCore: &MockedProjectCore{
				AccessibleProjectsFunc: func(_ context.Context, _ mdl.ProjectFilter, _, _ int) ([]mdl.Project, int, error) {
					return []mdl.Project{firstProject, secondProject}, 2, nil
				},
			},
			in: &pb.ListProjectsRequest{},
			want: &pb.ListProjectsResponse{
				Projects:  []*pb.Project{pbFirstProject, pbSecondProject},
				TotalSize: 2,
			},
		},
		{
			name: "empty result",
			projectCore: &MockedProjectCore{
				AccessibleProjectsFunc: func(_ context.Context, _ mdl.ProjectFilter, _, _ int) ([]mdl.Project, int, error) {
					return nil, 0, nil
				},
			},
			in:   &pb.ListProjectsRequest{},
			want: &pb.ListProjectsResponse{},
		},
		{
			name: "first page returns next_page_token when more results exist",
			projectCore: &MockedProjectCore{
				AccessibleProjectsFunc: func(_ context.Context, _ mdl.ProjectFilter, _, _ int) ([]mdl.Project, int, error) {
					return []mdl.Project{firstProject}, 2, nil
				},
			},
			in: &pb.ListProjectsRequest{PageSize: 1},
			want: &pb.ListProjectsResponse{
				Projects:      []*pb.Project{pbFirstProject},
				TotalSize:     2,
				NextPageToken: "eyJvIjoxfQ==",
			},
		},
		{
			name: "single page returns no next_page_token",
			projectCore: &MockedProjectCore{
				AccessibleProjectsFunc: func(_ context.Context, _ mdl.ProjectFilter, _, _ int) ([]mdl.Project, int, error) {
					return []mdl.Project{firstProject, secondProject}, 2, nil
				},
			},
			in: &pb.ListProjectsRequest{PageSize: 10},
			want: &pb.ListProjectsResponse{
				Projects:  []*pb.Project{pbFirstProject, pbSecondProject},
				TotalSize: 2,
			},
		},
		{
			name: "page_token offset is honored",
			projectCore: &MockedProjectCore{
				AccessibleProjectsFunc: func(_ context.Context, _ mdl.ProjectFilter, _, _ int) ([]mdl.Project, int, error) {
					return []mdl.Project{secondProject}, 3, nil
				},
			},
			in: &pb.ListProjectsRequest{
				PageSize:  1,
				PageToken: "eyJvIjoxfQ==",
			},
			want: &pb.ListProjectsResponse{
				Projects:      []*pb.Project{pbSecondProject},
				TotalSize:     3,
				NextPageToken: "eyJvIjoyfQ==",
			},
		},
		{
			name: "last page exactly fills page size",
			projectCore: &MockedProjectCore{
				AccessibleProjectsFunc: func(_ context.Context, _ mdl.ProjectFilter, _, _ int) ([]mdl.Project, int, error) {
					return []mdl.Project{firstProject, secondProject}, 2, nil
				},
			},
			in: &pb.ListProjectsRequest{PageSize: 2},
			want: &pb.ListProjectsResponse{
				Projects:  []*pb.Project{pbFirstProject, pbSecondProject},
				TotalSize: 2,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), ProjectCore: tt.projectCore})

			got, err := srv.projectServiceClient.ListProjects(authCtxForTestUser(t, t.Context()), tt.in)
			if err != nil {
				t.Fatalf("ListProjects() error = %v", err)
			}

			testingx.AssertDiff(t, got.GetProjects(), tt.want.GetProjects(), diffOpts)

			if got.GetTotalSize() != tt.want.GetTotalSize() {
				t.Errorf("ListProjects() total_size = %d, want %d", got.GetTotalSize(), tt.want.GetTotalSize())
			}

			if got.GetNextPageToken() != tt.want.GetNextPageToken() {
				t.Errorf("ListProjects() next_page_token = %q, want %q", got.GetNextPageToken(), tt.want.GetNextPageToken())
			}
		})
	}
}

func TestProjectService_ListProjects_error(t *testing.T) {
	diffOpts := defaultDiffOpts()

	filterPageToken, err := conv.EncodePageToken(1, "", &pb.ProjectFilter{Name: "wid"})
	if err != nil {
		t.Fatalf("EncodePageToken() error = %v", err)
	}

	tests := []struct {
		name        string
		projectCore ProjectCore
		in          *pb.ListProjectsRequest
		want        *status.Status
	}{
		{
			name:        "validated request",
			projectCore: &MockedProjectCore{},
			in:          &pb.ListProjectsRequest{PageToken: "invalid"},
			want:        status.New(codes.InvalidArgument, "invalid page_token"),
		},
		{
			name: "page token filter mismatch",
			projectCore: &MockedProjectCore{
				AccessibleProjectsFunc: func(_ context.Context, _ mdl.ProjectFilter, _, _ int) ([]mdl.Project, int, error) {
					return nil, 0, nil
				},
			},
			in: &pb.ListProjectsRequest{
				PageToken: filterPageToken,
				Filter:    &pb.ProjectFilter{Name: "other"},
			},
			want: status.New(codes.InvalidArgument, "page_token filter mismatch"),
		},
		{
			name: "authenticated user not found",
			projectCore: &MockedProjectCore{
				AccessibleProjectsFunc: func(_ context.Context, _ mdl.ProjectFilter, _, _ int) ([]mdl.Project, int, error) {
					return nil, 0, mdl.ErrNotFound
				},
			},
			in:   &pb.ListProjectsRequest{},
			want: status.New(codes.NotFound, "user not found"),
		},
		{
			name: "core error",
			projectCore: &MockedProjectCore{
				AccessibleProjectsFunc: func(_ context.Context, _ mdl.ProjectFilter, _, _ int) ([]mdl.Project, int, error) {
					return nil, 0, errors.New("boom")
				},
			},
			in:   &pb.ListProjectsRequest{},
			want: status.New(codes.Internal, "Internal"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := NewServerTest(t, ServerConfig{Log: testingx.NewLogger(t), ProjectCore: tt.projectCore})

			_, err := srv.projectServiceClient.ListProjects(authCtxForTestUser(t, t.Context()), tt.in)
			if err == nil {
				t.Fatal("ListProjects() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("ListProjects() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Proto(), tt.want.Proto(), diffOpts)
		})
	}
}
