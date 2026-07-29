package validate

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/conv"
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
)

func ListProjects(req *pb.ListProjectsRequest) error {
	if req == nil {
		return requiredRequest("request")
	}

	if _, err := conv.DecodePageToken[*pb.ProjectFilter](req.GetPageToken()); err != nil {
		return status.Error(codes.InvalidArgument, "invalid page_token")
	}

	return nil
}
