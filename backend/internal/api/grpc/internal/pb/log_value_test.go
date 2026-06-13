package pb

import (
	"log/slog"
	"slices"
	"testing"
)

func TestVerifyMagicLinkRequest_LogValue(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "non-empty token",
			token: "super-secret-token",
		},
		{
			name:  "empty token",
			token: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &VerifyMagicLinkRequest{Token: tt.token}
			attrs := req.LogValue().Group()

			i := slices.IndexFunc(attrs, func(a slog.Attr) bool { return a.Key == "token" })
			if i < 0 {
				t.Fatalf("VerifyMagicLinkRequest{Token:%q}.LogValue() has no 'token' attribute", tt.token)
			}
			if got := attrs[i].Value.String(); got != "[REDACTED]" {
				t.Errorf("VerifyMagicLinkRequest{Token:%q}.LogValue() token = %q, want [REDACTED]", tt.token, got)
			}
		})
	}
}

func TestRefreshAccessTokenRequest_LogValue(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "non-empty token",
			token: "super-secret-refresh",
		},
		{
			name:  "empty token",
			token: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &RefreshAccessTokenRequest{RefreshToken: tt.token}
			attrs := req.LogValue().Group()

			i := slices.IndexFunc(attrs, func(a slog.Attr) bool { return a.Key == "refresh_token" })
			if i < 0 {
				t.Fatalf("RefreshAccessTokenRequest{RefreshToken:%q}.LogValue() has no 'refresh_token' attribute", tt.token)
			}
			if got := attrs[i].Value.String(); got != "[REDACTED]" {
				t.Errorf("RefreshAccessTokenRequest{RefreshToken:%q}.LogValue() refresh_token = %q, want [REDACTED]", tt.token, got)
			}
		})
	}
}

func TestRevokeRefreshTokenRequest_LogValue(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{
			name:  "non-empty token",
			token: "super-secret-refresh",
		},
		{
			name:  "empty token",
			token: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &RevokeRefreshTokenRequest{RefreshToken: tt.token}
			attrs := req.LogValue().Group()

			i := slices.IndexFunc(attrs, func(a slog.Attr) bool { return a.Key == "refresh_token" })
			if i < 0 {
				t.Fatalf("RevokeRefreshTokenRequest{RefreshToken:%q}.LogValue() has no 'refresh_token' attribute", tt.token)
			}
			if got := attrs[i].Value.String(); got != "[REDACTED]" {
				t.Errorf("RevokeRefreshTokenRequest{RefreshToken:%q}.LogValue() refresh_token = %q, want [REDACTED]", tt.token, got)
			}
		})
	}
}

func TestTokenPair_LogValue(t *testing.T) {
	tests := []struct {
		name          string
		pair          *TokenPair
		wantExpiresIn int64
	}{
		{
			name: "redacts tokens and keeps expires_in",
			pair: &TokenPair{
				AccessToken:  "access",
				RefreshToken: "refresh",
				ExpiresIn:    900,
			},
			wantExpiresIn: 900,
		},
		{
			name: "zero expires_in",
			pair: &TokenPair{
				AccessToken:  "access",
				RefreshToken: "refresh",
				ExpiresIn:    0,
			},
			wantExpiresIn: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attrs := tt.pair.LogValue().Group()

			i := slices.IndexFunc(attrs, func(a slog.Attr) bool { return a.Key == "access_token" })
			if i < 0 {
				t.Fatalf("TokenPair.LogValue() has no 'access_token' attribute")
			}
			if got := attrs[i].Value.String(); got != "[REDACTED]" {
				t.Errorf("TokenPair.LogValue() access_token = %q, want [REDACTED]", got)
			}

			i = slices.IndexFunc(attrs, func(a slog.Attr) bool { return a.Key == "refresh_token" })
			if i < 0 {
				t.Fatalf("TokenPair.LogValue() has no 'refresh_token' attribute")
			}
			if got := attrs[i].Value.String(); got != "[REDACTED]" {
				t.Errorf("TokenPair.LogValue() refresh_token = %q, want [REDACTED]", got)
			}

			i = slices.IndexFunc(attrs, func(a slog.Attr) bool { return a.Key == "expires_in" })
			if i < 0 {
				t.Fatalf("TokenPair.LogValue() has no 'expires_in' attribute")
			}
			if got := attrs[i].Value.Int64(); got != tt.wantExpiresIn {
				t.Errorf("TokenPair.LogValue() expires_in = %d, want %d", got, tt.wantExpiresIn)
			}
		})
	}
}
