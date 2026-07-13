package conv

import (
	"github.com/zorcal/theapp/backend/internal/api/grpc/internal/pb"
	"github.com/zorcal/theapp/backend/internal/core/mdl"
)

func TokenPairToPB(p mdl.AuthTokenPair) *pb.TokenPair {
	return &pb.TokenPair{
		AccessToken:  p.AccessToken,
		RefreshToken: p.RefreshToken,
		ExpiresIn:    int64(p.ExpiresIn.Seconds()),
	}
}

func VerifyMagicLinkFromPB(req *pb.VerifyMagicLinkRequest) mdl.VerifyMagicLink {
	return mdl.VerifyMagicLink{Token: req.GetToken()}
}

func RefreshAccessTokenFromPB(req *pb.RefreshAccessTokenRequest) mdl.RefreshToken {
	return mdl.RefreshToken{Token: req.GetRefreshToken()}
}

func RevokeRefreshTokenFromPB(req *pb.RevokeRefreshTokenRequest) mdl.RefreshToken {
	return mdl.RefreshToken{Token: req.GetRefreshToken()}
}
