package grpc

import (
	"context"
	"fmt"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

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
	ListUsers(ctx context.Context, orderBys []order.By[mdl.UserOrderByField], pageSize, pageOffset int) (usrs []mdl.User, totalCount int, err error)
}

func (s *userService) ListUsers(ctx context.Context, req *pb.ListUsersRequest) (*pb.ListUsersResponse, error) {
	pageSize := int(req.GetPageSize())
	if pageSize <= 0 || pageSize > 100 {
		pageSize = 50 // sensible default/cap
	}

	pageToken, err := conv.DecodePageToken(req.GetPageToken())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", status.Errorf(codes.InvalidArgument, "invalid page_token"), err)
	}

	orderBys, err := conv.UserOrderBysFromPb(req.GetOrderBy())
	if err != nil {
		return nil, fmt.Errorf("%w: %w", status.Error(codes.InvalidArgument, "invalid order_by"), err)
	}

	// Validate caller hasn't changed sorting mid-pagination. The token is an
	// opaque cursor, so the caller is expected to echo back the exact order_by
	// it first sent; an exact string match is therefore correct, and we
	// deliberately do not normalize equivalent-but-different strings (e.g.
	// differing whitespace or an omitted default direction).
	if req.GetPageToken() != "" && pageToken.OrderBy != req.GetOrderBy() {
		return nil, status.Errorf(codes.InvalidArgument, "page_token order_by mismatch")
	}

	usrs, totalCount, err := s.userCore.ListUsers(ctx, orderBys, pageSize, pageToken.Offset)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	pbUsrs := conv.UsersToPb(usrs)

	var nextPageToken string
	nextPageOffset := pageToken.Offset + pageSize
	if nextPageOffset < totalCount {
		nextPageToken, err = conv.EncodePageToken(nextPageOffset, req.GetOrderBy())
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
