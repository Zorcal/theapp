package mdl

import (
	"errors"
	"testing"

	"github.com/zorcal/theapp/backend/internal/testingx"
)

func TestAuthProject_IsSystemControlProject(t *testing.T) {
	tests := []struct {
		name string
		in   AuthProject
		want bool
	}{
		{
			name: "system control project",
			in:   AuthProject{OrgName: SystemOrgName, IsControl: true},
			want: true,
		},
		{
			name: "system ordinary project",
			in:   AuthProject{OrgName: SystemOrgName},
			want: false,
		},
		{
			name: "other organization control project",
			in:   AuthProject{OrgName: "acme", IsControl: true},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.in.IsSystemControlProject(); got != tt.want {
				t.Errorf("IsSystemControlProject() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestAuthSessionContext(t *testing.T) {
	want := AuthSession{
		User:    AuthUser{Email: "alice@test.com"},
		Project: &AuthProject{ID: 1, OrgID: 2},
	}

	got, ok := AuthSessionFromContext(ContextWithAuthSession(t.Context(), want))
	if !ok {
		t.Fatal("AuthSessionFromContext() ok = false, want true")
	}

	testingx.AssertDiff(t, got, want)
}

func TestAuthSessionFromContext_missing(t *testing.T) {
	got, ok := AuthSessionFromContext(t.Context())
	if ok {
		t.Error("AuthSessionFromContext() ok = true, want false")
	}

	want := AuthSession{}

	testingx.AssertDiff(t, got, want)
}

func TestRequestMagicLink_Validate(t *testing.T) {
	tests := []struct {
		name string
		in   RequestMagicLink
	}{
		{
			name: "valid",
			in:   RequestMagicLink{Email: "alice@test.com"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); err != nil {
				t.Errorf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestRequestMagicLink_Validate_error(t *testing.T) {
	tests := []struct {
		name string
		in   RequestMagicLink
	}{
		{
			name: "empty email",
			in:   RequestMagicLink{Email: ""},
		},
		{
			name: "malformed email",
			in:   RequestMagicLink{Email: "notanemail"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); !errors.Is(err, ErrValidation) {
				t.Errorf("Validate() error = %v, want ErrValidation", err)
			}
		})
	}
}

func TestVerifyMagicLink_Validate(t *testing.T) {
	tests := []struct {
		name string
		in   VerifyMagicLink
	}{
		{
			name: "valid",
			in:   VerifyMagicLink{Token: "sometoken"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); err != nil {
				t.Errorf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestVerifyMagicLink_Validate_error(t *testing.T) {
	tests := []struct {
		name string
		in   VerifyMagicLink
	}{
		{
			name: "empty token",
			in:   VerifyMagicLink{Token: ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); !errors.Is(err, ErrValidation) {
				t.Errorf("Validate() error = %v, want ErrValidation", err)
			}
		})
	}
}

func TestRefreshToken_Validate(t *testing.T) {
	tests := []struct {
		name string
		in   RefreshToken
	}{
		{
			name: "valid",
			in:   RefreshToken{Token: "sometoken"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); err != nil {
				t.Errorf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestRefreshToken_Validate_error(t *testing.T) {
	tests := []struct {
		name string
		in   RefreshToken
	}{
		{
			name: "empty token",
			in:   RefreshToken{Token: ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); !errors.Is(err, ErrValidation) {
				t.Errorf("Validate() error = %v, want ErrValidation", err)
			}
		})
	}
}
