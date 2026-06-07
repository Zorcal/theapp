package conv

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"

	"google.golang.org/protobuf/proto"
)

// PageToken is the decoded form of a paginated request's continuation cursor.
// OrderBy and Filter are pinned alongside Offset so the server can reject
// requests that change sort order or filter criteria mid-pagination, which
// would otherwise yield duplicates or gaps.
//
// Compare Filter with proto.Equal, not ==: proto message structs embed sync
// primitives that make == undefined, and proto.Equal correctly treats nil and
// the empty message as equal.
type PageToken[F proto.Message] struct {
	Offset  int
	OrderBy string
	Filter  F
}

// EncodePageToken serializes a PageToken to a base64-encoded JSON string
// suitable for use as an opaque API cursor.
func EncodePageToken[F proto.Message](offset int, orderBy string, filter F) (string, error) {
	enc := pageTokenEncoded{Offset: offset, OrderBy: orderBy}

	fb, err := proto.Marshal(filter)
	if err != nil {
		return "", fmt.Errorf("marshal filter: %w", err)
	}
	enc.Filter = fb

	b, err := json.Marshal(enc)
	if err != nil {
		return "", fmt.Errorf("marshal json: %w", err)
	}

	return base64.StdEncoding.EncodeToString(b), nil
}

// DecodePageToken is the inverse of EncodePageToken. An empty token decodes to
// the zero PageToken without error, representing the first page.
// Returns an error if the filter bytes cannot be unmarshalled into F, e.g.
// due to a wire-type mismatch.
func DecodePageToken[F proto.Message](token string) (PageToken[F], error) {
	if token == "" {
		return PageToken[F]{}, nil
	}

	b, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return PageToken[F]{}, fmt.Errorf("decode base64: %w", err)
	}

	var enc pageTokenEncoded
	if err := json.Unmarshal(b, &enc); err != nil {
		return PageToken[F]{}, fmt.Errorf("unmarshal json: %w", err)
	}

	pt := PageToken[F]{Offset: enc.Offset, OrderBy: enc.OrderBy}

	if len(enc.Filter) > 0 {
		var zero F
		f, ok := reflect.New(reflect.TypeOf(zero).Elem()).Interface().(F)
		if !ok {
			panic(fmt.Sprintf("reflect.New produced unexpected type for %T", zero))
		}
		if err := proto.Unmarshal(enc.Filter, f); err != nil {
			return PageToken[F]{}, fmt.Errorf("unmarshal filter: %w", err)
		}
		pt.Filter = f
	}

	return pt, nil
}

// pageTokenEncoded is the wire format of a page token. Filter is proto binary,
// which encoding/json serializes as base64.
type pageTokenEncoded struct {
	Offset  int    `json:"o"`
	OrderBy string `json:"ob,omitzero"`
	Filter  []byte `json:"f,omitempty"`
}
