package grpc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"github.com/google/uuid"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/conv"
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/data/order"
	"github.com/zorcal/theapp/backend/pkg/mustconv"
)

type userService struct {
	pb.UnimplementedUserServiceServer

	log      *slog.Logger
	userCore UserCore
}

//go:generate moq -rm -fmt goimports -out user_core_moq_test.go . UserCore:MockedUserCore

type UserCore interface {
	// UserByID returns the user with the given ID.
	// Returns [mdl.ErrNotFound] if no user with that ID exists.
	UserByID(ctx context.Context, id uuid.UUID) (mdl.User, error)
	Users(ctx context.Context, filter mdl.UserFilter, orderBys []order.By[mdl.UserOrderByField], pageSize, pageOffset int) (usrs []mdl.User, totalCount int, err error)
	// CreateUser creates a new user and returns the created user.
	// Returns [mdl.ErrAlreadyExists] if a user with the same email already exists.
	// Returns [mdl.ErrValidation] if cu is invalid.
	CreateUser(ctx context.Context, cu mdl.CreateUser) (mdl.User, error)
	// UpdateUser updates the name of the user with the given ID and returns the updated user.
	// Returns [mdl.ErrNotFound] if no user with that ID exists.
	// Returns [mdl.ErrValidation] if uu is invalid.
	UpdateUser(ctx context.Context, uu mdl.UpdateUser) (mdl.User, error)
}

func (s *userService) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.User, error) {
	id, err := uuid.Parse(req.GetId())
	if err != nil {
		return nil, invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
			{Field: "id", Description: "must be a valid UUID"},
		})
	}

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
	if err := validateCreateUserRequest(req); err != nil {
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
	if req.GetUser() == nil {
		return nil, invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
			{Field: "user", Description: "required"},
		})
	}

	id, err := uuid.Parse(req.GetUser().GetId())
	if err != nil {
		return nil, invalidArgumentStatus([]*errdetails.BadRequest_FieldViolation{
			{Field: "user.id", Description: "must be a valid UUID"},
		})
	}

	if err := validateUpdateUserRequest(req); err != nil {
		return nil, fmt.Errorf("validate update user request: %w", err)
	}

	usr, err := s.userCore.UpdateUser(ctx, conv.UpdateUserFromPB(req, id))
	if err != nil {
		if errors.Is(err, mdl.ErrNotFound) {
			return nil, status.Errorf(codes.NotFound, "user %q not found", req.GetUser().GetId())
		}
		return nil, fmt.Errorf("update user: %w", err)
	}

	return conv.UserToPB(usr), nil
}

func validateUpdateUserRequest(req *pb.UpdateUserRequest) error {
	maskPaths := req.GetUpdateMask().GetPaths()

	if len(maskPaths) == 0 {
		return status.Error(codes.InvalidArgument, "update_mask is required")
	}

	var violations []*errdetails.BadRequest_FieldViolation

	updatableFields := []string{"name"}
	for _, path := range maskPaths {
		if !slices.Contains(updatableFields, path) {
			violations = append(violations, &errdetails.BadRequest_FieldViolation{
				Field:       "update_mask",
				Description: fmt.Sprintf("field %q is not updatable", path),
			})
		}
	}

	if slices.Contains(maskPaths, "name") && req.GetUser().GetName() == "" {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field:       "user.name",
			Description: "required",
		})
	}

	if len(violations) > 0 {
		return invalidArgumentStatus(violations)
	}
	return nil
}

func validateCreateUserRequest(req *pb.CreateUserRequest) error {
	var violations []*errdetails.BadRequest_FieldViolation

	if req.GetUser() == nil {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field:       "user",
			Description: "required",
		})
		return invalidArgumentStatus(violations)
	}

	if req.GetUser().GetEmail() == "" {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field:       "user.email",
			Description: "required",
		})
	} else if !mdl.IsValidEmail(req.GetUser().GetEmail()) {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field:       "user.email",
			Description: "must be a valid email address",
		})
	}

	if req.GetUser().GetName() == "" {
		violations = append(violations, &errdetails.BadRequest_FieldViolation{
			Field:       "user.name",
			Description: "required",
		})
	}

	if len(violations) > 0 {
		return invalidArgumentStatus(violations)
	}
	return nil
}

func (s *userService) ListUsers(ctx context.Context, req *pb.ListUsersRequest) (*pb.ListUsersResponse, error) {
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

	// Validate caller hasn't changed sorting or filter criteria mid-pagination.
	if req.GetPageToken() != "" {
		if pageToken.OrderBy != req.GetOrderBy() {
			return nil, status.Errorf(codes.InvalidArgument, "page_token order_by mismatch")
		}
		if !proto.Equal(pageToken.Filter, req.GetFilter()) {
			return nil, status.Errorf(codes.InvalidArgument, "page_token filter mismatch")
		}
	}

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
