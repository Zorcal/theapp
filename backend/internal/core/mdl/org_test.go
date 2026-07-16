package mdl

import (
	"errors"
	"testing"
)

func TestCreateOrganization_Validate(t *testing.T) {
	tests := []struct {
		name string
		in   CreateOrganization
	}{
		{"valid", CreateOrganization{Name: "acme", ProjectName: "acme"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); err != nil {
				t.Errorf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestCreateOrganization_Validate_error(t *testing.T) {
	tests := []struct {
		name string
		in   CreateOrganization
	}{
		{"empty name", CreateOrganization{Name: "", ProjectName: "acme"}},
		{"empty project name", CreateOrganization{Name: "acme", ProjectName: ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); !errors.Is(err, ErrValidation) {
				t.Errorf("Validate() error = %v, want ErrValidation", err)
			}
		})
	}
}

func TestCreateProject_Validate(t *testing.T) {
	tests := []struct {
		name string
		in   CreateProject
	}{
		{"valid", CreateProject{OrgID: 1, Name: "widgets"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); err != nil {
				t.Errorf("Validate() error = %v, want nil", err)
			}
		})
	}
}

func TestCreateProject_Validate_error(t *testing.T) {
	tests := []struct {
		name string
		in   CreateProject
	}{
		{"org id missing", CreateProject{OrgID: 0, Name: "widgets"}},
		{"empty name", CreateProject{OrgID: 1, Name: ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.in.Validate(); !errors.Is(err, ErrValidation) {
				t.Errorf("Validate() error = %v, want ErrValidation", err)
			}
		})
	}
}
