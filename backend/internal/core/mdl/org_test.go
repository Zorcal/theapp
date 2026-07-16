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
		{
			name: "valid",
			in:   CreateOrganization{Name: "acme", ProjectName: "acme"},
		},
		{
			name: "project name control is no longer reserved",
			in:   CreateOrganization{Name: "acme", ProjectName: "control"},
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

func TestCreateOrganization_Validate_error(t *testing.T) {
	tests := []struct {
		name string
		in   CreateOrganization
	}{
		{
			name: "empty name",
			in:   CreateOrganization{Name: "", ProjectName: "acme"},
		},
		{
			name: "empty project name",
			in:   CreateOrganization{Name: "acme", ProjectName: ""},
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

func TestCreateProject_Validate(t *testing.T) {
	tests := []struct {
		name string
		in   CreateProject
	}{
		{
			name: "valid",
			in:   CreateProject{OrgID: 1, Name: "widgets"},
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

func TestCreateProject_Validate_error(t *testing.T) {
	tests := []struct {
		name string
		in   CreateProject
	}{
		{
			name: "org id missing",
			in:   CreateProject{OrgID: 0, Name: "widgets"},
		},
		{
			name: "empty name",
			in:   CreateProject{OrgID: 1, Name: ""},
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
