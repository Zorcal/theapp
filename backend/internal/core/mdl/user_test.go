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
		{
			name: "valid",
			in:   CreateUser{Email: "alice@test.com", Name: "Alice Smith"},
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

func TestCreateUser_Validate_error(t *testing.T) {
	tests := []struct {
		name string
		in   CreateUser
	}{
		{
			name: "empty email",
			in:   CreateUser{Email: "", Name: "Alice Smith"},
		},
		{
			name: "malformed email",
			in:   CreateUser{Email: "notanemail", Name: "Alice Smith"},
		},
		{
			name: "empty name",
			in:   CreateUser{Email: "alice@test.com", Name: ""},
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

func TestUpdateUser_Validate(t *testing.T) {
	tests := []struct {
		name string
		in   UpdateUser
	}{
		{
			name: "name set and non-empty",
			in:   UpdateUser{Name: "Alice Jones", Fields: UserUpdateFields{Name: true}},
		},
		{
			name: "name not set",
			in:   UpdateUser{Fields: UserUpdateFields{Name: false}},
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

func TestUpdateUser_Validate_error(t *testing.T) {
	tests := []struct {
		name string
		in   UpdateUser
	}{
		{
			name: "name set but empty",
			in:   UpdateUser{Name: "", Fields: UserUpdateFields{Name: true}},
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
