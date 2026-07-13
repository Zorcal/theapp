package mdl

import (
	"errors"
	"testing"
)

func TestCreateUser_Validate(t *testing.T) {
	tests := []struct {
		name string
		in   CreateUser
	}{
		{"valid", CreateUser{Email: "alice@test.com", Name: "Alice Smith"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); err != nil {
				t.Errorf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestCreateUser_Validate_error(t *testing.T) {
	tests := []struct {
		name string
		in   CreateUser
	}{
		{"empty email", CreateUser{Email: "", Name: "Alice Smith"}},
		{"malformed email", CreateUser{Email: "notanemail", Name: "Alice Smith"}},
		{"empty name", CreateUser{Email: "alice@test.com", Name: ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); !errors.Is(err, ErrValidation) {
				t.Errorf("Validate() error = %v, want ErrValidation", err)
			}
		})
	}
}

func TestUpdateUser_Validate(t *testing.T) {
	tests := []struct {
		name string
		in   UpdateUser
	}{
		{"name set and non-empty", UpdateUser{Name: "Alice Jones", Fields: UserUpdateFields{Name: true}}},
		{"name not set", UpdateUser{Fields: UserUpdateFields{Name: false}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); err != nil {
				t.Errorf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestUpdateUser_Validate_error(t *testing.T) {
	tests := []struct {
		name string
		in   UpdateUser
	}{
		{"name set but empty", UpdateUser{Name: "", Fields: UserUpdateFields{Name: true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); !errors.Is(err, ErrValidation) {
				t.Errorf("Validate() error = %v, want ErrValidation", err)
			}
		})
	}
}
