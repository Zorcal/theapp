package grpc

import (
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
)

func TestErrorStatus(t *testing.T) {
	err := errorStatus(codes.FailedPrecondition, pb.ErrorCode_ERROR_CODE_MANAGED_ROLE, "managed role")
	st := status.Convert(err)

	if got, want := st.Code(), codes.FailedPrecondition; got != want {
		t.Errorf("errorStatus() code = %v, want %v", got, want)
	}
	if got, want := st.Message(), "managed role"; got != want {
		t.Errorf("errorStatus() message = %q, want %q", got, want)
	}

	details := st.Details()
	if got, want := len(details), 1; got != want {
		t.Fatalf("errorStatus() detail count = %d, want %d", got, want)
	}

	detail, ok := details[0].(*pb.ErrorDetail)
	if !ok {
		t.Fatalf("errorStatus() detail type = %T, want *pb.ErrorDetail", details[0])
	}
	if got, want := detail.GetCode(), pb.ErrorCode_ERROR_CODE_MANAGED_ROLE; got != want {
		t.Errorf("errorStatus() detail code = %v, want %v", got, want)
	}
}
