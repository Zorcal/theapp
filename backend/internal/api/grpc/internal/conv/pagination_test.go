package conv

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestEncodePageToken(t *testing.T) {
	tests := []struct {
		name    string
		offset  int
		orderBy string
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := EncodePageToken(tt.offset, tt.orderBy)
			if err != nil {
				t.Fatalf("EncodePageToken(%d, %q) error = %q, want no error", tt.offset, tt.orderBy, err)
			}
			if got != tt.want {
				t.Errorf("EncodePageToken(%d, %q) = %q, want %q", tt.offset, tt.orderBy, got, tt.want)
			}
		})
	}
}

func TestDecodePageToken(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want PageToken
	}{
		{
			name: "empty",
			in:   "",
			want: PageToken{},
		},
		{
			name: "zero offset, no order_by",
			in:   "eyJvIjowfQ==",
			want: PageToken{Offset: 0},
		},
		{
			name: "non-zero offset, no order_by",
			in:   "eyJvIjoyfQ==",
			want: PageToken{Offset: 2},
		},
		{
			name: "offset with order_by",
			in:   "eyJvIjo0LCJvYiI6ImVtYWlsIGRlc2MsdXBkYXRlZF9hdCJ9",
			want: PageToken{
				Offset:  4,
				OrderBy: "email desc,updated_at",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DecodePageToken(tt.in)
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
			got, err := DecodePageToken(tt.in)
			if err == nil {
				t.Fatalf("DecodePageToken(%q) = %+v, want error", tt.in, got)
			}
		})
	}
}
