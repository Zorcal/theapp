// Package mustconv provides functions for safe type conversions that panic on
// invalid conversions to prevent silent errors.
package mustconv

import (
	"fmt"
	"math"
)

// Int32 converts an int or int64 to an int32, panicking if the value is out of
// the int32 range.
func Int32[T ~int | ~int64](i T) int32 {
	if i < math.MinInt32 || i > math.MaxInt32 {
		panic(fmt.Sprintf("cannot convert %d to int32: out of range", i))
	}
	return int32(i)
}
