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
