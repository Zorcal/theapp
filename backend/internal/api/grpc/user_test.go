package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/data/order"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestUserService_GetUser(t *testing.T) {
	diffOpts := defaultDiffOpts()

	id, etag, now := uuid.New(), uuid.NewString(), time.Now()

	tests := []struct {
		name     string
		userCore UserCore
		in       *pb.GetUserRequest
		want     *pb.User
	}{
		{
			name: "returns user",
			userCore: &MockedUserCore{
				UserByIDFunc: func(_ context.Context, _ uuid.UUID) (mdl.User, error) {
					return mdl.User{ID: id, Email: "alice@test.com", Name: "Alice Smith", CreatedAt: now, ETag: etag}, nil
				},
			},
			in:   &pb.GetUserRequest{Id: id.String()},
			want: &pb.User{Id: id.String(), Email: "alice@test.com", Name: "Alice Smith", CreateTime: timestamppb.New(now), Etag: etag},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:      testingx.NewLogger(t),
				UserCore: tt.userCore,
			})

			got, err := srvTest.userServiceClient.GetUser(t.Context(), tt.in)
			if err != nil {
				t.Fatalf("GetUser() error = %q, want no error", err)
			}

			testingx.AssertDiff(t, got, tt.want, diffOpts)
		})
	}
}

func TestUserService_GetUser_error(t *testing.T) {
	tests := []struct {
		name     string
		userCore UserCore
		in       *pb.GetUserRequest
		want     *status.Status
	}{
		{
			name:     "invalid id",
			userCore: &MockedUserCore{},
			in:       &pb.GetUserRequest{Id: "not-a-uuid"},
			want:     status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name: "not found",
			userCore: &MockedUserCore{
				UserByIDFunc: func(_ context.Context, _ uuid.UUID) (mdl.User, error) {
					return mdl.User{}, mdl.ErrNotFound
				},
			},
			in:   &pb.GetUserRequest{Id: uuid.NewString()},
			want: status.New(codes.NotFound, "user \""+uuid.Nil.String()+"\" not found"),
		},
		{
			name: "core error",
			userCore: &MockedUserCore{
				UserByIDFunc: func(_ context.Context, _ uuid.UUID) (mdl.User, error) {
					return mdl.User{}, errors.New("boom")
				},
			},
			in:   &pb.GetUserRequest{Id: uuid.NewString()},
			want: status.New(codes.Internal, "Internal"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:      testingx.NewLogger(t),
				UserCore: tt.userCore,
			})

			_, err := srvTest.userServiceClient.GetUser(t.Context(), tt.in)
			if err == nil {
				t.Fatal("GetUser() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("GetUser() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Code(), tt.want.Code(), defaultDiffOpts())
		})
	}
}

func TestUserService_CreateUser(t *testing.T) {
	diffOpts := defaultDiffOpts()

	id, etag, now := uuid.New(), uuid.NewString(), time.Now()

	tests := []struct {
		name     string
		userCore UserCore
		in       *pb.CreateUserRequest
		want     *pb.User
	}{
		{
			name: "returns created user",
			userCore: &MockedUserCore{
				CreateUserFunc: func(_ context.Context, _ mdl.CreateUser) (mdl.User, error) {
					return mdl.User{ID: id, Email: "alice@test.com", Name: "Alice Smith", CreatedAt: now, ETag: etag}, nil
				},
			},
			in:   &pb.CreateUserRequest{User: &pb.User{Email: "alice@test.com", Name: "Alice Smith"}},
			want: &pb.User{Id: id.String(), Email: "alice@test.com", Name: "Alice Smith", CreateTime: timestamppb.New(now), Etag: etag},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:      testingx.NewLogger(t),
				UserCore: tt.userCore,
			})

			got, err := srvTest.userServiceClient.CreateUser(t.Context(), tt.in)
			if err != nil {
				t.Fatalf("CreateUser() error = %q, want no error", err)
			}

			testingx.AssertDiff(t, got, tt.want, diffOpts)
		})
	}
}

func TestUserService_CreateUser_error(t *testing.T) {
	invalidArgWithViolation := func(field, desc string) *status.Status {
		st, err := status.New(codes.InvalidArgument, codes.InvalidArgument.String()).WithDetails(
			&errdetails.BadRequest{FieldViolations: []*errdetails.BadRequest_FieldViolation{
				{Field: field, Description: desc},
			}},
		)
		if err != nil {
			t.Fatalf("invalidArgWithViolation(%q, %q) build status error = %v", field, desc, err)
		}
		return st
	}

	tests := []struct {
		name     string
		userCore UserCore
		in       *pb.CreateUserRequest
		want     *status.Status
	}{
		{
			name:     "missing user field",
			userCore: &MockedUserCore{},
			in:       &pb.CreateUserRequest{},
			want:     invalidArgWithViolation("user", "required"),
		},
		{
			name:     "empty email",
			userCore: &MockedUserCore{},
			in:       &pb.CreateUserRequest{User: &pb.User{Name: "Alice Smith"}},
			want:     invalidArgWithViolation("user.email", "required"),
		},
		{
			name:     "empty name",
			userCore: &MockedUserCore{},
			in:       &pb.CreateUserRequest{User: &pb.User{Email: "alice@test.com"}},
			want:     invalidArgWithViolation("user.name", "required"),
		},
		{
			name: "core error",
			userCore: &MockedUserCore{
				CreateUserFunc: func(_ context.Context, _ mdl.CreateUser) (mdl.User, error) {
					return mdl.User{}, errors.New("boom")
				},
			},
			in:   &pb.CreateUserRequest{User: &pb.User{Email: "alice@test.com", Name: "Alice Smith"}},
			want: status.New(codes.Internal, "Internal"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:      testingx.NewLogger(t),
				UserCore: tt.userCore,
			})

			_, err := srvTest.userServiceClient.CreateUser(t.Context(), tt.in)
			if err == nil {
				t.Fatal("CreateUser() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("CreateUser() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Proto(), tt.want.Proto(), defaultDiffOpts())
		})
	}
}

func TestUserService_UpdateUser(t *testing.T) {
	diffOpts := defaultDiffOpts()

	id, etag, now := uuid.New(), uuid.NewString(), time.Now()

	tests := []struct {
		name     string
		userCore UserCore
		in       *pb.UpdateUserRequest
		want     *pb.User
	}{
		{
			name: "explicit mask with name",
			userCore: &MockedUserCore{
				UpdateUserFunc: func(_ context.Context, _ mdl.UpdateUser) (mdl.User, error) {
					return mdl.User{ID: id, Email: "alice@test.com", Name: "Alice Updated", CreatedAt: now, ETag: etag}, nil
				},
			},
			in: &pb.UpdateUserRequest{
				User:       &pb.User{Id: id.String(), Name: "Alice Updated"},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
			},
			want: &pb.User{Id: id.String(), Email: "alice@test.com", Name: "Alice Updated", CreateTime: timestamppb.New(now), Etag: etag},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:      testingx.NewLogger(t),
				UserCore: tt.userCore,
			})

			got, err := srvTest.userServiceClient.UpdateUser(t.Context(), tt.in)
			if err != nil {
				t.Fatalf("UpdateUser() error = %q, want no error", err)
			}

			testingx.AssertDiff(t, got, tt.want, diffOpts)
		})
	}
}

func TestUserService_UpdateUser_error(t *testing.T) {
	tests := []struct {
		name     string
		userCore UserCore
		in       *pb.UpdateUserRequest
		want     *status.Status
	}{
		{
			name:     "missing user field",
			userCore: &MockedUserCore{},
			in:       &pb.UpdateUserRequest{},
			want:     status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name:     "invalid id",
			userCore: &MockedUserCore{},
			in:       &pb.UpdateUserRequest{User: &pb.User{Id: "not-a-uuid", Name: "Alice Updated"}},
			want:     status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name:     "no mask",
			userCore: &MockedUserCore{},
			in:       &pb.UpdateUserRequest{User: &pb.User{Id: uuid.NewString(), Name: "Alice Updated"}},
			want:     status.New(codes.InvalidArgument, "update_mask is required"),
		},
		{
			name:     "mask has name but name is empty",
			userCore: &MockedUserCore{},
			in: &pb.UpdateUserRequest{
				User:       &pb.User{Id: uuid.NewString()},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
			},
			want: status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name:     "non-updatable field in update_mask",
			userCore: &MockedUserCore{},
			in: &pb.UpdateUserRequest{
				User:       &pb.User{Id: uuid.NewString(), Name: "Alice Updated"},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"email"}},
			},
			want: status.New(codes.InvalidArgument, codes.InvalidArgument.String()),
		},
		{
			name: "not found",
			userCore: &MockedUserCore{
				UpdateUserFunc: func(_ context.Context, _ mdl.UpdateUser) (mdl.User, error) {
					return mdl.User{}, mdl.ErrNotFound
				},
			},
			in: &pb.UpdateUserRequest{
				User:       &pb.User{Id: uuid.NewString(), Name: "Alice Updated"},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
			},
			want: status.New(codes.NotFound, "user \""+uuid.Nil.String()+"\" not found"),
		},
		{
			name: "core error",
			userCore: &MockedUserCore{
				UpdateUserFunc: func(_ context.Context, _ mdl.UpdateUser) (mdl.User, error) {
					return mdl.User{}, errors.New("boom")
				},
			},
			in: &pb.UpdateUserRequest{
				User:       &pb.User{Id: uuid.NewString(), Name: "Alice Updated"},
				UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
			},
			want: status.New(codes.Internal, "Internal"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srvTest := NewServerTest(t, ServerConfig{
				Log:      testingx.NewLogger(t),
				UserCore: tt.userCore,
			})

			_, err := srvTest.userServiceClient.UpdateUser(t.Context(), tt.in)
			if err == nil {
				t.Fatal("UpdateUser() error = nil, want error")
			}

			got, ok := status.FromError(err)
			if !ok {
				t.Fatalf("UpdateUser() error = %q, want a gRPC status error", err)
			}

			testingx.AssertDiff(t, got.Code(), tt.want.Code(), defaultDiffOpts())
		})
	}
}

func TestUserService_ListUsers(t *testing.T) {
	diffOpts := defaultDiffOpts()

	now := time.Now()

	johnDoe := mdl.User{
		ID:        uuid.New(),
		Email:     "john.doe@test.com",
		Name:      "John Doe",
		CreatedAt: now.AddDate(0, 0, -15),
		ETag:      uuid.NewString(),
	}
	maryDoe := mdl.User{
		ID:        uuid.New(),
		Email:     "mary.doe@test.com",
		Name:      "Mary Doe",
		CreatedAt: now.AddDate(0, 0, -12),
		UpdatedAt: new(now.AddDate(0, 0, -3)),
		ETag:      uuid.NewString(),
	}
	smithBrown := mdl.User{
		ID:        uuid.New(),
		Email:     "smith.brown@test.com",
		Name:      "Smith Brown",
		CreatedAt: now.AddDate(0, 0, -10),
		UpdatedAt: new(now.AddDate(0, 0, -1)),
		ETag:      uuid.NewString(),
	}

	pbJohnDoe := &pb.User{
		Id:         johnDoe.ID.String(),
		Email:      "john.doe@test.com",
		Name:       "John Doe",
		CreateTime: timestamppb.New(now.AddDate(0, 0, -15)),
		Etag:       johnDoe.ETag,
	}
	pbMaryDoe := &pb.User{
		Id:         maryDoe.ID.String(),
		Email:      "mary.doe@test.com",
		Name:       "Mary Doe",
		CreateTime: timestamppb.New(now.AddDate(0, 0, -12)),
		UpdateTime: timestamppb.New(now.AddDate(0, 0, -3)),
		Etag:       maryDoe.ETag,
	}
	pbSmithBrown := &pb.User{
		Id:         smithBrown.ID.String(),
		Email:      "smith.brown@test.com",
		Name:       "Smith Brown",
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
				UsersFunc: func(ctx context.Context, orderBys []order.By[mdl.UserOrderByField], pageSize, pageOffset int) ([]mdl.User, int, error) {
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
				UsersFunc: func(ctx context.Context, orderBys []order.By[mdl.UserOrderByField], pageSize, pageOffset int) ([]mdl.User, int, error) {
					return nil, 0, nil
				},
			},
			in:   &pb.ListUsersRequest{},
			want: &pb.ListUsersResponse{},
		},
		{
			name: "first page returns next_page_token when more results exist",
			userCore: &MockedUserCore{
				UsersFunc: func(ctx context.Context, orderBys []order.By[mdl.UserOrderByField], pageSize, pageOffset int) ([]mdl.User, int, error) {
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
				UsersFunc: func(ctx context.Context, orderBys []order.By[mdl.UserOrderByField], pageSize, pageOffset int) ([]mdl.User, int, error) {
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
				UsersFunc: func(ctx context.Context, orderBys []order.By[mdl.UserOrderByField], pageSize, pageOffset int) ([]mdl.User, int, error) {
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
				UsersFunc: func(ctx context.Context, orderBys []order.By[mdl.UserOrderByField], pageSize, pageOffset int) ([]mdl.User, int, error) {
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
				UsersFunc: func(ctx context.Context, orderBys []order.By[mdl.UserOrderByField], pageSize, pageOffset int) ([]mdl.User, int, error) {
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

func TestUserService_ListUsers_error(t *testing.T) {
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
				UsersFunc: func(ctx context.Context, orderBys []order.By[mdl.UserOrderByField], pageSize, pageOffset int) ([]mdl.User, int, error) {
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
