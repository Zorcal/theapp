package conv

import (
	"fmt"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/data/order"
	"github.com/zorcal/theapp/backend/pkg/x/slicesx"
)

func CreateUserFromPb(u *pb.User) mdl.CreateUser {
	return mdl.CreateUser{Email: u.GetEmail()}
}

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

func UserOrderBysFromPb(s string) ([]order.By[mdl.UserOrderByField], error) {
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
