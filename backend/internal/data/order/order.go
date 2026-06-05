// Package order provides a common structure for passing around order by information.
package order

// Direction represents the direction to order data in.
type Direction string

// All directions. Values follow standard SQL.
const (
	DirectionAsc  Direction = "ASC"
	DirectionDesc Direction = "DESC"
)

// By contains information needed to order data.
type By[T ~string] struct {
	Field     T
	Direction Direction
}

// NewBy constructs a new By value.
func NewBy[T ~string](field T, dir Direction) By[T] {
	return By[T]{
		Field:     field,
		Direction: dir,
	}
}
