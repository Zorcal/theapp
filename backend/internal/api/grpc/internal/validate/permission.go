package validate

import "github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"

func ListPermissions(req *pb.ListPermissionsRequest) error {
	if req == nil {
		return requiredRequest("request")
	}

	return nil
}
