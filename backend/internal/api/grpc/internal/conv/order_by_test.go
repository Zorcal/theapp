package conv

import (
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/zorcal/theapp/backend/internal/data/order"
)

func TestParseOrderBy(t *testing.T) {
	fieldMapping := map[string]string{"k1": "v1", "k2": "v2", "k3": "v3"}

	tests := []struct {
		name string
		in   string
		want []order.By[string]
	}{
		{
			name: "empty",
			in:   "",
			want: nil,
		},
		{
			name: "all fields",
			in:   "k1,k2 desc,k3 asc",
			want: []order.By[string]{
				{Direction: order.DirectionAsc, Field: "v1"},
				{Direction: order.DirectionDesc, Field: "v2"},
				{Direction: order.DirectionAsc, Field: "v3"},
			},
		},
		{
			name: "subset of fields",
			in:   "k3 asc,k2 desc",
			want: []order.By[string]{
				{Direction: order.DirectionAsc, Field: "v3"},
				{Direction: order.DirectionDesc, Field: "v2"},
			},
		},
		{
			name: "ignores redundant space",
			in:   "   k3   asc    ,   k2     desc    ",
			want: []order.By[string]{
				{Direction: order.DirectionAsc, Field: "v3"},
				{Direction: order.DirectionDesc, Field: "v2"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOrderBy(tt.in, fieldMapping)
			if err != nil {
				t.Fatalf("Parse(%q, %+v) error = %q, want no error", tt.in, fieldMapping, err)
			}
			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("Parse(%q, %+v) diff mismatch (-got +want):\n%s", tt.in, fieldMapping, diff)
			}
		})
	}
}

func TestParseOrderBy_error(t *testing.T) {
	fieldMapping := map[string]string{"k1": "v1", "k2": "v2", "k3": "v3"}

	tests := []struct {
		name         string
		in           string
		fieldMapping map[string]string
	}{
		{
			name: "invalid format",
			in:   "k1 desc k2 asc",
		},
		{
			name: "unknown field",
			in:   "k1 asc,unknown desc",
		},
		{
			name: "unknown direction",
			in:   "k2 desc,k3 unknown",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseOrderBy(tt.in, fieldMapping)
			if err == nil {
				t.Fatalf("Parse(%q, %+v) = %+v, want error", tt.in, fieldMapping, got)
			}
		})
	}
}
