package conv

import (
	"fmt"
	"slices"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/data/order"
	"github.com/zorcal/theapp/backend/pkg/x/slicesx"
)

func UpdateUserFromPb(req *pb.UpdateUserRequest, id uuid.UUID) mdl.UpdateUser {
	paths := req.GetUpdateMask().GetPaths()
	return mdl.UpdateUser{
		ID:   id,
		Name: req.GetUser().GetName(),
		Fields: mdl.UserUpdateFields{
			Name: slices.Contains(paths, "name"),
		},
	}
}

func CreateUserFromPb(u *pb.User) mdl.CreateUser {
	return mdl.CreateUser{Email: u.GetEmail(), Name: u.GetName()}
}

func UsersToPb(usr []mdl.User) []*pb.User {
	return slicesx.Map(usr, UserToPb)
}

func UserToPb(usr mdl.User) *pb.User {
	return &pb.User{
		Id:         usr.ID.String(),
		Email:      usr.Email,
		Name:       usr.Name,
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
