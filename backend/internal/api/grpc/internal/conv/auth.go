package conv

import (
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
)

func TokenPairToPB(p mdl.TokenPair) *pb.TokenPair {
	return &pb.TokenPair{
		AccessToken:  p.AccessToken,
		RefreshToken: p.RefreshToken,
		ExpiresIn:    int64(p.ExpiresIn.Seconds()),
	}
}
