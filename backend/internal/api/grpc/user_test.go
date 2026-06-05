package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/data/order"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestUserService_listUsers(t *testing.T) {
	diffOpts := defaultDiffOpts()

	now := time.Now()

	johnDoe := mdl.User{
		ID:        uuid.New(),
		Email:     "john.doe@test.com",
		CreatedAt: now.AddDate(0, 0, -15),
		ETag:      uuid.NewString(),
	}
	maryDoe := mdl.User{
		ID:        uuid.New(),
		Email:     "mary.doe@test.com",
		CreatedAt: now.AddDate(0, 0, -12),
		UpdatedAt: new(now.AddDate(0, 0, -3)),
		ETag:      uuid.NewString(),
	}
	smithBrown := mdl.User{
		ID:        uuid.New(),
		Email:     "smith.brown@test.com",
		CreatedAt: now.AddDate(0, 0, -10),
		UpdatedAt: new(now.AddDate(0, 0, -1)),
		ETag:      uuid.NewString(),
	}

	pbJohnDoe := &pb.User{
		Id:         johnDoe.ID.String(),
		Email:      "john.doe@test.com",
		CreateTime: timestamppb.New(now.AddDate(0, 0, -15)),
		Etag:       johnDoe.ETag,
	}
	pbMaryDoe := &pb.User{
		Id:         maryDoe.ID.String(),
		Email:      "mary.doe@test.com",
		CreateTime: timestamppb.New(now.AddDate(0, 0, -12)),
		UpdateTime: timestamppb.New(now.AddDate(0, 0, -3)),
		Etag:       maryDoe.ETag,
	}
	pbSmithBrown := &pb.User{
		Id:         smithBrown.ID.String(),
		Email:      "smith.brown@test.com",
		CreateTime: timestamppb.New(now.AddDate(0, 0, -10)),
		UpdateTime: timestamppb.New(now.AddDate(0, 0, -1)),
		Etag:       smithBrown.ETag,
	}

	tests := []struct {
		name     string
		userCore UserCore
		in       *pb.ListUsersRequest
		want     *pb.ListUsersResponse
	}{
		{
			name: "empty request",
			userCore: &MockedUserCore{
				ListUsersFunc: func(ctx context.Context, fltr mdl.UserFilter, orderBys []order.By[mdl.UserOrderByField], pageSize, pageOffset int) ([]mdl.User, int, error) {
					return []mdl.User{johnDoe, maryDoe, smithBrown}, 15, nil
				},
			},
			in: &pb.ListUsersRequest{},
			want: &pb.ListUsersResponse{
				Users:     []*pb.User{pbJohnDoe, pbMaryDoe, pbSmithBrown},
				TotalSize: 15,
			},
		},
		{
			name: "empty result",
			userCore: &MockedUserCore{
				ListUsersFunc: func(ctx context.Context, fltr mdl.UserFilter, orderBys []order.By[mdl.UserOrderByField], pageSize, pageOffset int) ([]mdl.User, int, error) {
					return nil, 0, nil
				},
			},
			in:   &pb.ListUsersRequest{},
			want: &pb.ListUsersResponse{},
		},
		{
			name: "first page returns next_page_token when more results exist",
			userCore: &MockedUserCore{
				ListUsersFunc: func(ctx context.Context, fltr mdl.UserFilter, orderBys []order.By[mdl.UserOrderByField], pageSize, pageOffset int) ([]mdl.User, int, error) {
					return []mdl.User{johnDoe, maryDoe}, 5, nil
				},
			},
			in: &pb.ListUsersRequest{PageSize: 2},
			want: &pb.ListUsersResponse{
				Users:         []*pb.User{pbJohnDoe, pbMaryDoe},
				TotalSize:     5,
				NextPageToken: "eyJvIjoyfQ==",
			},
		},
		{
			name: "single page returns no next_page_token",
			userCore: &MockedUserCore{
				ListUsersFunc: func(ctx context.Context, fltr mdl.UserFilter, orderBys []order.By[mdl.UserOrderByField], pageSize, pageOffset int) ([]mdl.User, int, error) {
					return []mdl.User{johnDoe, maryDoe, smithBrown}, 3, nil
				},
			},
			in: &pb.ListUsersRequest{PageSize: 10},
			want: &pb.ListUsersResponse{
				Users:     []*pb.User{pbJohnDoe, pbMaryDoe, pbSmithBrown},
				TotalSize: 3,
			},
		},
		{
			name: "order_by carried into next_page_token",
			userCore: &MockedUserCore{
				ListUsersFunc: func(ctx context.Context, fltr mdl.UserFilter, orderBys []order.By[mdl.UserOrderByField], pageSize, pageOffset int) ([]mdl.User, int, error) {
					return []mdl.User{johnDoe, maryDoe}, 5, nil
				},
			},
			in: &pb.ListUsersRequest{
				PageSize: 2,
				OrderBy:  "email desc,updated_at",
			},
			want: &pb.ListUsersResponse{
				Users:         []*pb.User{pbJohnDoe, pbMaryDoe},
				TotalSize:     5,
				NextPageToken: "eyJvIjoyLCJvYiI6ImVtYWlsIGRlc2MsdXBkYXRlZF9hdCJ9",
			},
		},
		{
			name: "page_token offset is honored",
			userCore: &MockedUserCore{
				ListUsersFunc: func(ctx context.Context, fltr mdl.UserFilter, orderBys []order.By[mdl.UserOrderByField], pageSize, pageOffset int) ([]mdl.User, int, error) {
					return []mdl.User{smithBrown}, 10, nil
				},
			},
			in: &pb.ListUsersRequest{
				PageSize:  2,
				PageToken: "eyJvIjoyfQ==",
			},
			want: &pb.ListUsersResponse{
				Users:         []*pb.User{pbSmithBrown},
				TotalSize:     10,
				NextPageToken: "eyJvIjo0fQ==",
			},
		},
		{
			name: "last page exactly fills page size",
			userCore: &MockedUserCore{
				ListUsersFunc: func(ctx context.Context, fltr mdl.UserFilter, orderBys []order.By[mdl.UserOrderByField], pageSize, pageOffset int) ([]mdl.User, int, error) {
					return []mdl.User{johnDoe, maryDoe, smithBrown}, 3, nil
				},
			},
			in: &pb.ListUsersRequest{PageSize: 3},
			want: &pb.ListUsersResponse{
				Users:     []*pb.User{pbJohnDoe, pbMaryDoe, pbSmithBrown},
				TotalSize: 3,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:      testingx.NewLogger(t),
				UserCore: tt.userCore,
			})

			got, err := srvTest.userServiceClient.ListUsers(t.Context(), tt.in)
			if err != nil {
				t.Fatalf("ListUsers() error = %q, want no error", err)
			}

			testingx.AssertDiff(t, got.GetUsers(), tt.want.GetUsers(), diffOpts)

			if got.GetTotalSize() != tt.want.GetTotalSize() {
				t.Errorf("ListUsers() total_size = %d, want %d", got.GetTotalSize(), tt.want.GetTotalSize())
			}

			if got.GetNextPageToken() != tt.want.GetNextPageToken() {
				t.Errorf("ListUsers() next_page_token = %q, want %q", got.GetNextPageToken(), tt.want.GetNextPageToken())
			}
		})
	}
}

func TestUserService_listUsers_error(t *testing.T) {
	diffOpts := defaultDiffOpts()

	tests := []struct {
		name     string
		userCore UserCore
		in       *pb.ListUsersRequest
		want     *status.Status
	}{
		{
			name:     "invalid page_token",
			userCore: &MockedUserCore{},
			in:       &pb.ListUsersRequest{PageToken: "!!!not-base64!!!"},
			want:     status.New(codes.InvalidArgument, "invalid page_token"),
		},
		{
			name:     "unknown order_by field",
			userCore: &MockedUserCore{},
			in:       &pb.ListUsersRequest{OrderBy: "nope"},
			want:     status.New(codes.InvalidArgument, "invalid order_by"),
		},
		{
			name:     "order_by mismatch with page_token",
			userCore: &MockedUserCore{},
			in:       &pb.ListUsersRequest{PageToken: "eyJvIjoyfQ==", OrderBy: "email"},
			want:     status.New(codes.InvalidArgument, "page_token order_by mismatch"),
		},
		{
			name: "core error",
			userCore: &MockedUserCore{
				ListUsersFunc: func(ctx context.Context, fltr mdl.UserFilter, orderBys []order.By[mdl.UserOrderByField], pageSize, pageOffset int) ([]mdl.User, int, error) {
					return nil, 0, errors.New("boom")
				},
			},
			in:   &pb.ListUsersRequest{},
			want: status.New(codes.Internal, "Internal"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:      testingx.NewLogger(t),
				UserCore: tt.userCore,
			})

			_, err := srvTest.userServiceClient.ListUsers(t.Context(), tt.in)
			if err == nil {
				t.Fatal("ListUsers() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("Listusers() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Proto(), tt.want.Proto(), diffOpts)
		})
	}
}
