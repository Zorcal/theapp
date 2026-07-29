package conv

import (
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/pkg/mustconv"
	"github.com/zorcal/theapp/backend/pkg/x/slicesx"
)

func ProjectsToPB(projects []mdl.Project) []*pb.Project {
	return slicesx.Map(projects, ProjectToPB)
}

func ProjectFilterFromPB(filter *pb.ProjectFilter) mdl.ProjectFilter {
	return mdl.ProjectFilter{Name: filter.GetName()}
}

func ProjectToPB(project mdl.Project) *pb.Project {
	return &pb.Project{
		Id:             mustconv.Int32(project.ID),
		OrganizationId: mustconv.Int32(project.OrgID),
		Name:           project.Name,
		IsControl:      project.IsControl,
		CreateTime:     timestamppb.New(project.CreatedAt),
		UpdateTime:     maybeNewTimestamppb(project.UpdatedAt),
		Etag:           project.ETag.String(),
	}
}
