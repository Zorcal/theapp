// Package conv provides conversion between core business models and protocol
// buffer representations used in gRPC communication.
package conv

import (
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
)

func maybeNewTimestamppb(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}
