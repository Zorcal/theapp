package conv

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
)

func TestEncodePageToken(t *testing.T) {
	tests := []struct {
		name     string
		offset   int
		orderBys []mdl.OrderBy[string]
		want     string
	}{
		{
			name:   "zero offset, no order_bys",
			offset: 0,
			want:   "eyJvIjowfQ==",
		},
		{
			name:   "non-zero offset, no order_bys",
			offset: 2,
			want:   "eyJvIjoyfQ==",
		},
		{
			name:   "offset with order_bys",
			offset: 4,
			orderBys: []mdl.OrderBy[string]{
				{Field: "email", Direction: mdl.DirectionDesc},
				{Field: "updated_at", Direction: mdl.DirectionAsc},
			},
			want: "eyJvIjo0LCJvYiI6W3siRmllbGQiOiJlbWFpbCIsIkRpcmVjdGlvbiI6IkRFU0MifSx7IkZpZWxkIjoidXBkYXRlZF9hdCIsIkRpcmVjdGlvbiI6IkFTQyJ9XX0=",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EncodePageToken(tt.offset, tt.orderBys)
			if err != nil {
				t.Fatalf("EncodePageToken(%d, %+v) error = %q, want no error", tt.offset, tt.orderBys, err)
			}
			if got != tt.want {
				t.Errorf("EncodePageToken(%d, %+v) = %q, want %q", tt.offset, tt.orderBys, got, tt.want)
			}
		})
	}
}

func TestDecodePageToken(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want PageToken[string]
	}{
		{
			name: "empty",
			in:   "",
			want: PageToken[string]{},
		},
		{
			name: "zero offset, no order_bys",
			in:   "eyJvIjowfQ==",
			want: PageToken[string]{Offset: 0},
		},
		{
			name: "non-zero offset, no order_bys",
			in:   "eyJvIjoyfQ==",
			want: PageToken[string]{Offset: 2},
		},
		{
			name: "offset with order_bys",
			in:   "eyJvIjo0LCJvYiI6W3siRmllbGQiOiJlbWFpbCIsIkRpcmVjdGlvbiI6IkRFU0MifSx7IkZpZWxkIjoidXBkYXRlZF9hdCIsIkRpcmVjdGlvbiI6IkFTQyJ9XX0=",
			want: PageToken[string]{
				Offset: 4,
				OrderBys: []mdl.OrderBy[string]{
					{Field: "email", Direction: mdl.DirectionDesc},
					{Field: "updated_at", Direction: mdl.DirectionAsc},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodePageToken[string](tt.in)
			if err != nil {
				t.Fatalf("DecodePageToken(%q) error = %q, want no error", tt.in, err)
			}
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("DecodePageToken(%q) diff mismatch (-got +want):\n%s", tt.in, diff)
			}
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
			got, err := DecodePageToken[string](tt.in)
			if err == nil {
				t.Fatalf("DecodePageToken(%q) = %+v, want error", tt.in, got)
			}
		})
	}
}
