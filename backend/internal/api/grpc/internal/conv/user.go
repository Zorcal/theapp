package conv

import (
	"fmt"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/pkg/x/slicesx"
)

func UsersToPb(usr []mdl.User) []*pb.User {
	return slicesx.Map(usr, UserToPb)
}

func UserToPb(usr mdl.User) *pb.User {
	return &pb.User{
		Id:         usr.ID.String(),
		Email:      usr.Email,
		UpdateTime: maybeNewTimestamppb(usr.UpdatedAt),
		CreateTime: timestamppb.New(usr.CreatedAt),
		Etag:       usr.ETag,
	}
}

func UserFilterFromPB(fltr *pb.ListUsersFilter) mdl.UserFilter {
	return mdl.UserFilter{}
}

func UserOrderBysFromPb(s string) ([]mdl.OrderBy[mdl.UserOrderByField], error) {
	fieldMapping := map[string]mdl.UserOrderByField{
		"email":      mdl.UserOrderByFieldEmail,
		"updated_at": mdl.UserOrderByFieldUpdatedAt,
	}

	orderBys, err := parseOrderBy(s, fieldMapping)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	return orderBys, nil
}
