package conv

import (
	"testing"

	"google.golang.org/protobuf/testing/protocmp"

	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestEncodePageToken(t *testing.T) {
	tests := []struct {
		name    string
		offset  int
		orderBy string
		filter  *pb.UserFilter
		want    string
	}{
		{
			name:   "zero offset, no order_by",
			offset: 0,
			want:   "eyJvIjowfQ==",
		},
		{
			name:   "non-zero offset, no order_by",
			offset: 2,
			want:   "eyJvIjoyfQ==",
		},
		{
			name:    "offset with order_by",
			offset:  4,
			orderBy: "email desc,updated_at",
			want:    "eyJvIjo0LCJvYiI6ImVtYWlsIGRlc2MsdXBkYXRlZF9hdCJ9",
		},
		{
			name:   "offset with filter",
			offset: 2,
			filter: &pb.UserFilter{Email: "alice"},
			want:   "eyJvIjoyLCJmIjoiQ2dWaGJHbGpaUT09In0=",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EncodePageToken(tt.offset, tt.orderBy, tt.filter)
			if err != nil {
				t.Fatalf("EncodePageToken(%d, %q, %v) error = %q, want no error", tt.offset, tt.orderBy, tt.filter, err)
			}
			if got != tt.want {
				t.Errorf("EncodePageToken(%d, %q, %v) = %q, want %q", tt.offset, tt.orderBy, tt.filter, got, tt.want)
			}
		})
	}
}

func TestDecodePageToken(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want PageToken[*pb.UserFilter]
	}{
		{
			name: "empty",
			in:   "",
			want: PageToken[*pb.UserFilter]{},
		},
		{
			name: "zero offset, no order_by",
			in:   "eyJvIjowfQ==",
			want: PageToken[*pb.UserFilter]{Offset: 0},
		},
		{
			name: "non-zero offset, no order_by",
			in:   "eyJvIjoyfQ==",
			want: PageToken[*pb.UserFilter]{Offset: 2},
		},
		{
			name: "offset with order_by",
			in:   "eyJvIjo0LCJvYiI6ImVtYWlsIGRlc2MsdXBkYXRlZF9hdCJ9",
			want: PageToken[*pb.UserFilter]{
				Offset:  4,
				OrderBy: "email desc,updated_at",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodePageToken[*pb.UserFilter](tt.in)
			if err != nil {
				t.Fatalf("DecodePageToken(%q) error = %q, want no error", tt.in, err)
			}
			testingx.AssertDiff(t, got, tt.want, protocmp.Transform())
		})
	}
}

func TestDecodePageToken_error(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{
			name: "invalid base64",
			in:   "!!!not-base64!!!",
		},
		{
			name: "valid base64, invalid json",
			in:   "bm90IGpzb24=", // "not json"
		},
		{
			name: "json array instead of object",
			in:   "WzEsMl0=", // "[1,2]"
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodePageToken[*pb.UserFilter](tt.in)
			if err == nil {
				t.Fatalf("DecodePageToken(%q) = %+v, want error", tt.in, got)
			}
		})
	}
}
