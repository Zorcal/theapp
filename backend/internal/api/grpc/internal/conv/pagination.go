package conv

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// PageToken is the decoded form of a paginated request's continuation cursor.
// OrderBy holds the request's raw order_by string, pinned alongside Offset so
// the server can reject requests that change sort order mid-pagination, which
// would otherwise yield duplicates or gaps.
type PageToken struct {
	Offset  int    `json:"o"`
	OrderBy string `json:"ob,omitzero"`
}

// EncodePageToken serializes a PageToken to a base64-encoded JSON string
// suitable for use as an opaque API cursor.
func EncodePageToken(offset int, orderBy string) (string, error) {
	tok := PageToken{
		Offset:  offset,
		OrderBy: orderBy,
	}

	b, err := json.Marshal(tok)
	if err != nil {
		return "", fmt.Errorf("marshal json: %w", err)
	}

	return base64.StdEncoding.EncodeToString(b), nil
}

// DecodePageToken is the inverse of EncodePageToken. An empty token decodes to
// the zero PageToken without error, representing the first page.
func DecodePageToken(token string) (PageToken, error) {
	if token == "" {
		return PageToken{}, nil
	}

	b, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return PageToken{}, fmt.Errorf("decode base64: %w", err)
	}

	var pt PageToken
	if err := json.Unmarshal(b, &pt); err != nil {
		return PageToken{}, fmt.Errorf("unmarshal json: %w", err)
	}

	return pt, nil
}
