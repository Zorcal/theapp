package pb

import "log/slog"

func (x *VerifyMagicLinkRequest) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("token", "[REDACTED]"),
	)
}

func (x *RefreshAccessTokenRequest) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("refresh_token", "[REDACTED]"),
	)
}

func (x *RevokeRefreshTokenRequest) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("refresh_token", "[REDACTED]"),
	)
}

func (x *TokenPair) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("access_token", "[REDACTED]"),
		slog.String("refresh_token", "[REDACTED]"),
		slog.Int64("expires_in", x.GetExpiresIn()),
	)
}
