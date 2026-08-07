package grpc

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/conv"
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/validate"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/data/order"
	"github.com/zorcal/theapp/backend/pkg/mustconv"
)

type userService struct {
	pb.UnimplementedUserServiceServer

	userCore UserCore
}

//go:generate moq -rm -fmt goimports -out user_core_moq_test.go . UserCore:MockedUserCore

type UserCore interface {
	// UserByID returns the active user with the given ID.
	// Returns [mdl.ErrNotFound] if no active user with that ID exists.
	UserByID(ctx context.Context, id uuid.UUID) (mdl.User, error)
	// Users returns a page of active users matching filter, ordered by orderBys, and the total count.
	Users(ctx context.Context, filter mdl.UserFilter, orderBys []order.By[mdl.UserOrderByField], pageSize, pageOffset int) (usrs []mdl.User, totalCount int, err error)
	// CreateUser creates a new user and returns the created user.
	// Returns [mdl.ErrAlreadyExists] if a user with the same email already exists.
	// Returns [mdl.ErrValidation] if cu is invalid.
	CreateUser(ctx context.Context, cu mdl.CreateUser) (mdl.User, error)
	// UpdateUser updates the name of the active user with the given ID and returns the updated user.
	// Returns [mdl.ErrNotFound] if no active user with that ID exists.
	// Returns [mdl.ErrValidation] if uu is invalid.
	UpdateUser(ctx context.Context, uu mdl.UpdateUser) (mdl.User, error)
	// DeleteUser soft-deletes the user with the given ID.
	// Returns [mdl.ErrNotFound] if no active user with that ID exists.
	// Returns [mdl.ErrLastFullyPrivilegedSystemAdmin] if deletion would leave no fully privileged
	// active system administrator.
	DeleteUser(ctx context.Context, id uuid.UUID) error
}

func (s *userService) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.User, error) {
	if err := validate.GetUser(req); err != nil {
		return nil, fmt.Errorf("validate get user request: %w", err)
	}

	id := uuid.MustParse(req.GetId())

	usr, err := s.userCore.UserByID(ctx, id)
	if err != nil {
		if errors.Is(err, mdl.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "user %q not found", req.GetId())
		}
		return nil, fmt.Errorf("get user: %w", err)
	}

	return conv.UserToPB(usr), nil
}

func (s *userService) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.User, error) {
	if err := validate.CreateUser(req); err != nil {
		return nil, fmt.Errorf("validate create user request: %w", err)
	}

	cu := conv.CreateUserFromPB(req.GetUser())

	usr, err := s.userCore.CreateUser(ctx, cu)
	if err != nil {
		if errors.Is(err, mdl.ErrAlreadyExists) {
			return nil, invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
				{Field: "user.email", Description: "a user with this email already exists"},
			})
		}
		return nil, fmt.Errorf("create user: %w", err)
	}

	return conv.UserToPB(usr), nil
}

func (s *userService) UpdateUser(ctx context.Context, req *pb.UpdateUserRequest) (*pb.User, error) {
	if err := validate.UpdateUser(req); err != nil {
		return nil, fmt.Errorf("validate update user request: %w", err)
	}

	id := uuid.MustParse(req.GetUser().GetId())

	usr, err := s.userCore.UpdateUser(ctx, conv.UpdateUserFromPB(req, id))
	if err != nil {
		if errors.Is(err, mdl.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "user %q not found", req.GetUser().GetId())
		}
		return nil, fmt.Errorf("update user: %w", err)
	}

	return conv.UserToPB(usr), nil
}

func (s *userService) DeleteUser(ctx context.Context, req *pb.DeleteUserRequest) (*emptypb.Empty, error) {
	if err := validate.DeleteUser(req); err != nil {
		return nil, fmt.Errorf("validate delete user request: %w", err)
	}

	id := uuid.MustParse(req.GetId())
	if err := s.userCore.DeleteUser(ctx, id); err != nil {
		switch {
		case errors.Is(err, mdl.ErrNotFound):
			return nil, status.Errorf(codes.NotFound, "active user %q not found", req.GetId())
		case errors.Is(err, mdl.ErrLastFullyPrivilegedSystemAdmin):
			return nil, status.Error(codes.FailedPrecondition, "cannot delete the last fully privileged system administrator")
		default:
			return nil, fmt.Errorf("delete user: %w", err)
		}
	}

	return &emptypb.Empty{}, nil
}

func (s *userService) ListUsers(ctx context.Context, req *pb.ListUsersRequest) (*pb.ListUsersResponse, error) {
	if err := validate.ListUsers(req); err != nil {
		return nil, fmt.Errorf("validate list users request: %w", err)
	}

	pageSize := int(req.GetPageSize())
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 50 // sensible default/cap
	}

	pageToken, err := conv.DecodePageToken[*pb.UserFilter](req.GetPageToken())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", status.Errorf(codes.InvalidArgument, "invalid page_token"), err)
	}

	orderBys, err := conv.UserOrderBysFromPB(req.GetOrderBy())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", status.Error(codes.InvalidArgument, "invalid order_by"), err)
	}

	filter := conv.UserFilterFromPB(req.GetFilter())

	usrs, totalCount, err := s.userCore.Users(ctx, filter, orderBys, pageSize, pageToken.Offset)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	pbUsrs := conv.UsersToPB(usrs)

	var nextPageToken string
	nextPageOffset := pageToken.Offset + pageSize
	if nextPageOffset < totalCount {
		nextPageToken, err = conv.EncodePageToken(nextPageOffset, req.GetOrderBy(), req.GetFilter())
		if err != nil {
			return nil, fmt.Errorf("encode next_page_token: %w", err)
		}
	}

	return &pb.ListUsersResponse{
		Users:         pbUsrs,
		TotalSize:     mustconv.Int32(totalCount),
		NextPageToken: nextPageToken,
	}, nil
}
