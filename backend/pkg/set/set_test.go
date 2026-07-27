package set

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestFromSlice(t *testing.T) {
	tests := []struct {
		name string
		in   []int
		want Set[int]
	}{
		{
			name: "empty",
			in:   []int{},
			want: Set[int]{},
		},
		{
			name: "values",
			in:   []int{1, 2, 3},
			want: Set[int]{1: {}, 2: {}, 3: {}},
		},
		{
			name: "duplicates",
			in:   []int{1, 2, 1},
			want: Set[int]{1: {}, 2: {}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(FromSlice(tt.in), tt.want); diff != "" {
				t.Errorf("FromSlice(%v) mismatch (-got +want):\n%s", tt.in, diff)
			}
		})
	}
}

func TestAdd(t *testing.T) {
	tests := []struct {
		name string
		in   []int
		want Set[int]
	}{
		{
			name: "none",
			in:   nil,
			want: Set[int]{},
		},
		{
			name: "single",
			in:   []int{1},
			want: Set[int]{1: {}},
		},
		{
			name: "multiple",
			in:   []int{1, 2, 3},
			want: Set[int]{1: {}, 2: {}, 3: {}},
		},
		{
			name: "duplicate",
			in:   []int{1, 2, 1},
			want: Set[int]{1: {}, 2: {}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := New[int]()
			got = got.Add(tt.in...)

			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("Add(%v) mismatch (-got +want):\n%s", tt.in, diff)
			}
		})
	}
}

func TestRemove(t *testing.T) {
	tests := []struct {
		name    string
		initial Set[int]
		remove  []int
		want    Set[int]
	}{
		{
			name:    "none",
			initial: Set[int]{1: {}, 2: {}},
			remove:  nil,
			want:    Set[int]{1: {}, 2: {}},
		},
		{
			name:    "present",
			initial: Set[int]{1: {}, 2: {}},
			remove:  []int{1},
			want:    Set[int]{2: {}},
		},
		{
			name:    "multiple",
			initial: Set[int]{1: {}, 2: {}, 3: {}},
			remove:  []int{1, 3},
			want:    Set[int]{2: {}},
		},
		{
			name:    "missing",
			initial: Set[int]{1: {}},
			remove:  []int{2},
			want:    Set[int]{1: {}},
		},
		{
			name:    "present and missing",
			initial: Set[int]{1: {}, 2: {}},
			remove:  []int{1, 3},
			want:    Set[int]{2: {}},
		},
		{
			name:    "duplicates",
			initial: Set[int]{1: {}, 2: {}},
			remove:  []int{1, 1},
			want:    Set[int]{2: {}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.initial.Clone()
			got = got.Remove(tt.remove...)

			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("Remove(%v) mismatch (-got +want):\n%s", tt.remove, diff)
			}
		})
	}
}

func TestLen(t *testing.T) {
	tests := []struct {
		name string
		set  Set[int]
		want int
	}{
		{
			name: "empty",
			set:  Set[int]{},
			want: 0,
		},
		{
			name: "multiple",
			set:  Set[int]{1: {}, 2: {}},
			want: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.set.Len(); got != tt.want {
				t.Errorf("Len(%v) = %d, want %d", tt.set, got, tt.want)
			}
		})
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name string
		set  Set[int]
		in   int
		want bool
	}{
		{
			name: "present",
			set:  Set[int]{1: {}},
			in:   1,
			want: true,
		},
		{
			name: "missing",
			set:  Set[int]{1: {}},
			in:   2,
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.set.Contains(tt.in); got != tt.want {
				t.Errorf("Contains(%d) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsSuperset(t *testing.T) {
	tests := []struct {
		name  string
		set   Set[int]
		other Set[int]
		want  bool
	}{
		{
			name:  "equal sets",
			set:   Set[int]{1: {}, 2: {}},
			other: Set[int]{1: {}, 2: {}},
			want:  true,
		},
		{
			name:  "strict superset",
			set:   Set[int]{1: {}, 2: {}, 3: {}},
			other: Set[int]{1: {}, 2: {}},
			want:  true,
		},
		{
			name:  "missing element",
			set:   Set[int]{1: {}},
			other: Set[int]{1: {}, 2: {}},
			want:  false,
		},
		{
			name:  "empty other",
			set:   Set[int]{1: {}},
			other: Set[int]{},
			want:  true,
		},
		{
			name:  "empty set",
			set:   Set[int]{},
			other: Set[int]{1: {}},
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.set.IsSuperset(tt.other); got != tt.want {
				t.Errorf("IsSuperset(%v, %v) = %t, want %t", tt.set, tt.other, got, tt.want)
			}
		})
	}
}

func TestValues(t *testing.T) {
	tests := []struct {
		name string
		set  Set[int]
		want Set[int]
	}{
		{
			name: "multiple",
			set:  Set[int]{1: {}, 2: {}, 3: {}},
			want: Set[int]{1: {}, 2: {}, 3: {}},
		},
		{
			name: "empty",
			set:  Set[int]{},
			want: Set[int]{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := New[int]()
			for _, v := range tt.set.Values() {
				got.Add(v)
			}

			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("Values(%v) mismatch (-got +want):\n%s", tt.set, diff)
			}
		})
	}
}

func TestClone(t *testing.T) {
	tests := []struct {
		name string
		set  Set[int]
	}{
		{
			name: "multiple",
			set:  Set[int]{1: {}, 2: {}},
		},
		{
			name: "empty",
			set:  Set[int]{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.set.Clone()

			if diff := cmp.Diff(got, tt.set); diff != "" {
				t.Errorf("Clone(%v) mismatch (-got +want):\n%s", tt.set, diff)
			}

			got.Add(3)

			if tt.set.Contains(3) {
				t.Errorf("Clone(%v) mutated original set", tt.set)
			}
		})
	}
}

func TestUnion(t *testing.T) {
	tests := []struct {
		name  string
		left  Set[int]
		right Set[int]
		want  Set[int]
	}{
		{
			name:  "overlap",
			left:  Set[int]{1: {}, 2: {}},
			right: Set[int]{2: {}, 3: {}},
			want:  Set[int]{1: {}, 2: {}, 3: {}},
		},
		{
			name:  "empty",
			left:  Set[int]{1: {}},
			right: Set[int]{},
			want:  Set[int]{1: {}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left := tt.left.Clone()
			right := tt.right.Clone()

			got := left.Union(right)

			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("Union(%v, %v) mismatch (-got +want):\n%s", tt.left, tt.right, diff)
			}

			if diff := cmp.Diff(left, tt.left); diff != "" {
				t.Errorf("Union(%v, %v) mutated left operand (-got +want):\n%s", tt.left, tt.right, diff)
			}

			if diff := cmp.Diff(right, tt.right); diff != "" {
				t.Errorf("Union(%v, %v) mutated right operand (-got +want):\n%s", tt.left, tt.right, diff)
			}
		})
	}
}

func TestIntersection(t *testing.T) {
	tests := []struct {
		name  string
		left  Set[int]
		right Set[int]
		want  Set[int]
	}{
		{
			name:  "overlap",
			left:  Set[int]{1: {}, 2: {}, 3: {}},
			right: Set[int]{2: {}, 3: {}, 4: {}},
			want:  Set[int]{2: {}, 3: {}},
		},
		{
			name:  "none",
			left:  Set[int]{1: {}},
			right: Set[int]{2: {}},
			want:  Set[int]{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left := tt.left.Clone()
			right := tt.right.Clone()

			got := left.Intersection(right)

			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("Intersection(%v, %v) mismatch (-got +want):\n%s", tt.left, tt.right, diff)
			}

			if diff := cmp.Diff(left, tt.left); diff != "" {
				t.Errorf("Intersection(%v, %v) mutated left operand (-got +want):\n%s", tt.left, tt.right, diff)
			}

			if diff := cmp.Diff(right, tt.right); diff != "" {
				t.Errorf("Intersection(%v, %v) mutated right operand (-got +want):\n%s", tt.left, tt.right, diff)
			}
		})
	}
}

func TestDifference(t *testing.T) {
	tests := []struct {
		name  string
		left  Set[int]
		right Set[int]
		want  Set[int]
	}{
		{
			name:  "overlap",
			left:  Set[int]{1: {}, 2: {}, 3: {}},
			right: Set[int]{2: {}, 3: {}, 4: {}},
			want:  Set[int]{1: {}},
		},
		{
			name:  "no overlap",
			left:  Set[int]{1: {}, 2: {}},
			right: Set[int]{3: {}, 4: {}},
			want:  Set[int]{1: {}, 2: {}},
		},
		{
			name:  "equal sets",
			left:  Set[int]{1: {}, 2: {}},
			right: Set[int]{1: {}, 2: {}},
			want:  Set[int]{},
		},
		{
			name:  "empty left",
			left:  Set[int]{},
			right: Set[int]{1: {}},
			want:  Set[int]{},
		},
		{
			name:  "empty right",
			left:  Set[int]{1: {}, 2: {}},
			right: Set[int]{},
			want:  Set[int]{1: {}, 2: {}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left := tt.left.Clone()
			right := tt.right.Clone()

			got := left.Difference(right)

			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("Difference(%v, %v) mismatch (-got +want):\n%s", tt.left, tt.right, diff)
			}

			if diff := cmp.Diff(left, tt.left); diff != "" {
				t.Errorf("Difference(%v, %v) mutated left operand (-got +want):\n%s", tt.left, tt.right, diff)
			}

			if diff := cmp.Diff(right, tt.right); diff != "" {
				t.Errorf("Difference(%v, %v) mutated right operand (-got +want):\n%s", tt.left, tt.right, diff)
			}
		})
	}
}

func TestSymmetricDifference(t *testing.T) {
	tests := []struct {
		name  string
		left  Set[int]
		right Set[int]
		want  Set[int]
	}{
		{
			name:  "overlap",
			left:  Set[int]{1: {}, 2: {}, 3: {}},
			right: Set[int]{2: {}, 3: {}, 4: {}},
			want:  Set[int]{1: {}, 4: {}},
		},
		{
			name:  "no overlap",
			left:  Set[int]{1: {}, 2: {}},
			right: Set[int]{3: {}, 4: {}},
			want:  Set[int]{1: {}, 2: {}, 3: {}, 4: {}},
		},
		{
			name:  "equal sets",
			left:  Set[int]{1: {}, 2: {}},
			right: Set[int]{1: {}, 2: {}},
			want:  Set[int]{},
		},
		{
			name:  "empty left",
			left:  Set[int]{},
			right: Set[int]{1: {}, 2: {}},
			want:  Set[int]{1: {}, 2: {}},
		},
		{
			name:  "empty right",
			left:  Set[int]{1: {}, 2: {}},
			right: Set[int]{},
			want:  Set[int]{1: {}, 2: {}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			left := tt.left.Clone()
			right := tt.right.Clone()

			got := left.SymmetricDifference(right)

			if diff := cmp.Diff(got, tt.want); diff != "" {
				t.Errorf("SymmetricDifference(%v, %v) mismatch (-got +want):\n%s", tt.left, tt.right, diff)
			}

			if diff := cmp.Diff(left, tt.left); diff != "" {
				t.Errorf("SymmetricDifference(%v, %v) mutated left operand (-got +want):\n%s", tt.left, tt.right, diff)
			}

			if diff := cmp.Diff(right, tt.right); diff != "" {
				t.Errorf("SymmetricDifference(%v, %v) mutated right operand (-got +want):\n%s", tt.left, tt.right, diff)
			}
		})
	}
}
