// Package slicesx extends the slices package from the standard library.
package slicesx

// Map applies a function to each element of a slice and returns a new slice
// with the results.
func Map[T, U any](s []T, f func(T) U) []U {
	out := make([]U, len(s))
	for i, e := range s {
		out[i] = f(e)
	}
	return out
}

// CountFunc returns the number of elements for which f returns true.
func CountFunc[S ~[]E, E any](s S, f func(E) bool) int {
	var count int
	for _, e := range s {
		if f(e) {
			count++
		}
	}
	return count
}
