// Package set provides a generic set implementation.
package set

// Set is a generic set of comparable values.
type Set[T comparable] map[T]struct{}

// New returns a new empty set.
func New[T comparable]() Set[T] {
	return Set[T]{}
}

// Add inserts v into the set and returns the set.
func (s Set[T]) Add(v T) Set[T] {
	s[v] = struct{}{}
	return s
}

// Remove deletes v from the set and returns the set.
func (s Set[T]) Remove(v T) Set[T] {
	delete(s, v)
	return s
}

// Len returns the number of elements in the set.
func (s Set[T]) Len() int {
	return len(s)
}

// Contains reports whether v is an element of the set.
func (s Set[T]) Contains(v T) bool {
	_, ok := s[v]
	return ok
}

// Values returns the elements of the set in unspecified order.
func (s Set[T]) Values() []T {
	result := make([]T, 0, len(s))
	for v := range s {
		result = append(result, v)
	}
	return result
}

// Clone returns a copy of the set.
func (s Set[T]) Clone() Set[T] {
	result := make(Set[T], len(s))
	for v := range s {
		result[v] = struct{}{}
	}
	return result
}

// Union returns a new set containing all elements from s and other.
func (s Set[T]) Union(other Set[T]) Set[T] {
	result := s.Clone()
	for v := range other {
		result.Add(v)
	}
	return result
}

// Intersection returns a new set containing only the elements present in both
// s and other.
func (s Set[T]) Intersection(other Set[T]) Set[T] {
	result := New[T]()

	// Iterate over the smaller set to reduce lookups.
	if len(other) < len(s) {
		s, other = other, s
	}

	for v := range s {
		if other.Contains(v) {
			result.Add(v)
		}
	}

	return result
}
