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

func UpdateUserFromPB(req *pb.UpdateUserRequest, id uuid.UUID) mdl.UpdateUser {
	paths := req.GetUpdateMask().GetPaths()
	return mdl.UpdateUser{
		ID:   id,
		Name: req.GetUser().GetName(),
		Fields: mdl.UserUpdateFields{
			Name: slices.Contains(paths, "name"),
		},
	}
}

func CreateUserFromPB(u *pb.User) mdl.CreateUser {
	return mdl.CreateUser{Email: u.GetEmail(), Name: u.GetName()}
}

func UsersToPB(usr []mdl.User) []*pb.User {
	return slicesx.Map(usr, UserToPB)
}

func UserToPB(usr mdl.User) *pb.User {
	return &pb.User{
		Id:                usr.ID.String(),
		Email:             usr.Email,
		Name:              usr.Name,
		UpdateTime:        maybeNewTimestamppb(usr.UpdatedAt),
		CreateTime:        timestamppb.New(usr.CreatedAt),
		EmailVerifiedTime: maybeNewTimestamppb(usr.EmailVerifiedAt),
		Etag:              usr.ETag,
	}
}

// UserFilterFromPB converts a typed UserFilter proto message to a mdl.UserFilter.
func UserFilterFromPB(f *pb.UserFilter) mdl.UserFilter {
	return mdl.UserFilter{
		Email: f.GetEmail(),
		Name:  f.GetName(),
	}
}

func UserOrderBysFromPB(s string) ([]order.By[mdl.UserOrderByField], error) {
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
