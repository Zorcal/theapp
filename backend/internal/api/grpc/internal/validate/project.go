package validate

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/conv"
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
)

func ListProjects(req *pb.ListProjectsRequest) error {
	if req == nil {
		return requiredRequest("request")
	}

	pageToken, err := conv.DecodePageToken[*pb.ProjectFilter](req.GetPageToken())
	if err != nil {
		return status.Error(codes.InvalidArgument, "invalid page_token")
	}
	if req.GetPageToken() != "" && !proto.Equal(pageToken.Filter, req.GetFilter()) {
		return status.Error(codes.InvalidArgument, "page_token filter mismatch")
	}

	return nil
}
