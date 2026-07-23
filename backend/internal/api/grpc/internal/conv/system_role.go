package conv

import (
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/pkg/x/slicesx"
)

func SystemRolesToPB(roles []mdl.SystemRole) []*pb.SystemRole {
	return slicesx.Map(roles, SystemRoleToPB)
}

func SystemRoleToPB(role mdl.SystemRole) *pb.SystemRole {
	return &pb.SystemRole{
		Name:        role.Name,
		Permissions: slicesx.Map(role.Permissions, func(permission mdl.Permission) string { return string(permission) }),
	}
}
