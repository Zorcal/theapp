package grpc

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/conv"
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/validate"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/pkg/mustconv"
)

type projectService struct {
	pb.UnimplementedProjectServiceServer

	projectCore ProjectCore
}

//go:generate moq -rm -fmt goimports -out project_core_moq_test.go . ProjectCore:MockedProjectCore

type ProjectCore interface {
	// AccessibleProjects returns the projects reachable through any role assignment held by the
	// authenticated user and the total number of reachable projects.
	// Returns [mdl.ErrNotFound] if the authenticated user no longer exists.
	AccessibleProjects(ctx context.Context, filter mdl.ProjectFilter, pageSize, pageOffset int) ([]mdl.Project, int, error)
}

func (s *projectService) ListProjects(ctx context.Context, req *pb.ListProjectsRequest) (*pb.ListProjectsResponse, error) {
	if err := validate.ListProjects(req); err != nil {
		return nil, fmt.Errorf("validate list projects request: %w", err)
	}

	pageSize := int(req.GetPageSize())
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 50
	}

	pageToken, err := conv.DecodePageToken[*pb.ProjectFilter](req.GetPageToken())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", status.Error(codes.InvalidArgument, "invalid page_token"), err)
	}

	if req.GetPageToken() != "" && !proto.Equal(pageToken.Filter, req.GetFilter()) {
		return nil, status.Error(codes.InvalidArgument, "page_token filter mismatch")
	}

	filter := conv.ProjectFilterFromPB(req.GetFilter())

	projects, totalCount, err := s.projectCore.AccessibleProjects(ctx, filter, pageSize, pageToken.Offset)
	if err != nil {
		if errors.Is(err, mdl.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		return nil, fmt.Errorf("list accessible projects: %w", err)
	}

	var nextPageToken string
	nextPageOffset := pageToken.Offset + pageSize
	if nextPageOffset < totalCount {
		nextPageToken, err = conv.EncodePageToken(nextPageOffset, "", req.GetFilter())
		if err != nil {
			return nil, fmt.Errorf("encode next_page_token: %w", err)
		}
	}

	return &pb.ListProjectsResponse{
		Projects:      conv.ProjectsToPB(projects),
		TotalSize:     mustconv.Int32(totalCount),
		NextPageToken: nextPageToken,
	}, nil
}
