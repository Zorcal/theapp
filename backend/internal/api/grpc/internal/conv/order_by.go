package conv

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
)

// parseOrderBy parses the raw `orderBy` string into a structured order by representation. Assumes `orderBy` has the
// shape explained in https://google.aip.dev/132#ordering.
//
// `fieldMapping` maps fields in the raw `orderBy` value to custom field types. An empty `fieldMapping` results in an
// empty result.
func parseOrderBy[T ~string](orderBy string, fieldMapping map[string]T) ([]mdl.OrderBy[T], error) {
	if orderBy == "" {
		return nil, nil
	}

	if len(fieldMapping) == 0 {
		return nil, nil
	}

	out := make([]mdl.OrderBy[T], 0, 3)
	for s := range strings.SplitSeq(orderBy, ",") {
		parts := strings.Fields(s)
		if len(parts) == 0 || len(parts) > 2 {
			return nil, errors.New("invalid format")
		}

		field, ok := fieldMapping[parts[0]]
		if !ok {
			return nil, fmt.Errorf("invalid field %s", parts[0])
		}

		dir := mdl.DirectionAsc
		if len(parts) == 2 {
			var err error
			dir, err = parseDirection(parts[1])
			if err != nil {
				return nil, fmt.Errorf("invalid format: parse direction %s: %w", parts[1], err)
			}
		}

		out = append(out, mdl.OrderBy[T]{
			Direction: dir,
			Field:     field,
		})
	}
	out = slices.Clip(out)

	return out, nil
}

func parseDirection(s string) (mdl.Direction, error) {
	switch strings.ToLower(s) {
	case "asc":
		return mdl.DirectionAsc, nil
	case "desc":
		return mdl.DirectionDesc, nil
	default:
		return mdl.Direction(""), errors.New("invalid direction")
	}
}
