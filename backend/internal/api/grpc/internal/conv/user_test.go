package conv

import (
	"testing"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestUserFilterFromPB(t *testing.T) {
	tests := []struct {
		name string
		in   *pb.UserFilter
		want mdl.UserFilter
	}{
		{"nil", nil, mdl.UserFilter{}},
		{"empty", &pb.UserFilter{}, mdl.UserFilter{}},
		{"email only", &pb.UserFilter{Email: "alice"}, mdl.UserFilter{Email: "alice"}},
		{"name only", &pb.UserFilter{Name: "Smith"}, mdl.UserFilter{Name: "Smith"}},
		{"email and name", &pb.UserFilter{Email: "alice", Name: "Smith"}, mdl.UserFilter{Email: "alice", Name: "Smith"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UserFilterFromPB(tt.in)
			testingx.AssertDiff(t, got, tt.want)
		})
	}
}
