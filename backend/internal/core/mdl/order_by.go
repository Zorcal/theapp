package mdl

// Direction represents the direction to order data in.
type Direction string

// All directions.
const (
	DirectionAsc  Direction = "ASC"
	DirectionDesc Direction = "DESC"
)

// OrderBy contains information needed to order data.
type OrderBy[T ~string] struct {
	Field     T
	Direction Direction
}

// NewOrderBy constructs a new OrderBy value.
func NewOrderBy[T ~string](field T, dir Direction) OrderBy[T] {
	return OrderBy[T]{
		Field:     field,
		Direction: dir,
	}
}
