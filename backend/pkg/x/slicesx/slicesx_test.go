package slicesx

import (
	"strconv"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestMap(t *testing.T) {
	t.Run("maps elements", func(t *testing.T) {
		got := Map([]int{1, 2, 3}, func(v int) int { return v * v })
		want := []int{1, 4, 9}

		if diff := cmp.Diff(got, want); diff != "" {
			t.Errorf("Map() mismatch (-got +want):\n%s", diff)
		}
	})

	t.Run("converts element type", func(t *testing.T) {
		got := Map([]int{1, 2}, strconv.Itoa)
		want := []string{"1", "2"}

		if diff := cmp.Diff(got, want); diff != "" {
			t.Errorf("Map() mismatch (-got +want):\n%s", diff)
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		got := Map([]int{}, func(v int) int { return v })
		want := []int{}

		if diff := cmp.Diff(got, want); diff != "" {
			t.Errorf("Map() mismatch (-got +want):\n%s", diff)
		}
	})
}

func TestCountFunc(t *testing.T) {
	tests := []struct {
		name string
		in   []int
		f    func(int) bool
		want int
	}{
		{
			name: "matching elements",
			in:   []int{1, 2, 3, 4},
			f:    func(v int) bool { return v%2 == 0 },
			want: 2,
		},
		{
			name: "no matching elements",
			in:   []int{1, 3},
			f:    func(v int) bool { return v%2 == 0 },
			want: 0,
		},
		{
			name: "empty slice",
			in:   []int{},
			f:    func(int) bool { return true },
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CountFunc(tt.in, tt.f); got != tt.want {
				t.Errorf("CountFunc(%v) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}
