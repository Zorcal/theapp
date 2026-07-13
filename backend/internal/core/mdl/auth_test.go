package mdl

import (
	"errors"
	"testing"
)

func TestRequestMagicLink_Validate(t *testing.T) {
	tests := []struct {
		name string
		in   RequestMagicLink
	}{
		{"valid", RequestMagicLink{Email: "alice@test.com"}},
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
		{"empty email", RequestMagicLink{Email: ""}},
		{"malformed email", RequestMagicLink{Email: "notanemail"}},
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
		{"valid", VerifyMagicLink{Token: "sometoken"}},
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
		{"empty token", VerifyMagicLink{Token: ""}},
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
		{"valid", RefreshToken{Token: "sometoken"}},
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
		{"empty token", RefreshToken{Token: ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); !errors.Is(err, ErrValidation) {
				t.Errorf("Validate() error = %v, want ErrValidation", err)
			}
		})
	}
}
