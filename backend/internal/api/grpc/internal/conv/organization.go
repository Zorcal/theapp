package conv

import (
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/pkg/mustconv"
)

func CreateOrganizationFromPB(req *pb.CreateOrganizationRequest) mdl.CreateOrganization {
	return mdl.CreateOrganization{
		Name:        req.GetOrganization().GetName(),
		ProjectName: req.GetProjectName(),
	}
}

func CreateOrganizationUserFromPB(req *pb.CreateOrganizationUserRequest) mdl.CreateOrganizationUser {
	return mdl.CreateOrganizationUser{Email: req.GetEmail()}
}

func OrganizationToPB(organization mdl.Organization) *pb.Organization {
	return &pb.Organization{
		Id:               mustconv.Int32(organization.ID),
		Name:             organization.Name,
		ControlProjectId: mustconv.Int32(organization.ControlProjectID),
		CreateTime:       timestamppb.New(organization.CreatedAt),
		UpdateTime:       maybeNewTimestamppb(organization.UpdatedAt),
	}
}
