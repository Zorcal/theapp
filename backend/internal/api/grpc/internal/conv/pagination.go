package conv

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
)

// PageToken is the decoded form of a paginated request's continuation cursor.
// OrderBys is pinned alongside Offset so the server can reject requests that
// change sort order mid-pagination, which would otherwise yield duplicates or
// gaps.
type PageToken[T ~string] struct {
	Offset   int              `json:"o"`
	OrderBys []mdl.OrderBy[T] `json:"ob,omitzero"`
}

// EncodePageToken serializes a PageToken to a base64-encoded JSON string
// suitable for use as an opaque API cursor.
func EncodePageToken[T ~string](offset int, orderBys []mdl.OrderBy[T]) (string, error) {
	tok := PageToken[T]{
		Offset:   offset,
		OrderBys: orderBys,
	}

	b, err := json.Marshal(tok)
	if err != nil {
		return "", fmt.Errorf("marshal json: %w", err)
	}

	return base64.StdEncoding.EncodeToString(b), nil
}

// DecodePageToken is the inverse of EncodePageToken. An empty token decodes to
// the zero PageToken without error, representing the first page.
func DecodePageToken[T ~string](token string) (PageToken[T], error) {
	if token == "" {
		return PageToken[T]{}, nil
	}

	b, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return PageToken[T]{}, fmt.Errorf("decode base64: %w", err)
	}

	var pt PageToken[T]
	if err := json.Unmarshal(b, &pt); err != nil {
		return PageToken[T]{}, fmt.Errorf("unmarshal json: %w", err)
	}

	return pt, nil
}
