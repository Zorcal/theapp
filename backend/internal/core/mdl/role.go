package mdl

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/zorcal/theapp/backend/pkg/set"
)

// SystemRole is a system role and the permissions currently granted to it. System roles are seed
// data and cannot be mutated directly.
type SystemRole struct {
	Name        string
	Permissions []Permission
}

// CustomRole is an organization-owned role and its currently granted permissions.
type CustomRole struct {
	ID          uuid.UUID
	Name        string
	Permissions []Permission
	CreatedAt   time.Time
	UpdatedAt   *time.Time
	ETag        string
}

// CreateCustomRole holds the fields needed to create a custom role.
type CreateCustomRole struct {
	Name        string
	Permissions []Permission
}

func (cr CreateCustomRole) Validate() error {
	trimmedName := strings.TrimSpace(cr.Name)
	if trimmedName == "" {
		return validationError("name required")
	}
	if trimmedName != cr.Name {
		return validationError("name must not have leading or trailing whitespace")
	}
	if err := validateCustomRolePerms(cr.Permissions); err != nil {
		return fmt.Errorf("validate permissions: %w", err)
	}
	return nil
}

// UpdateCustomRole holds the fields to update on a custom role.
// Fields controls which fields are applied; fields not listed are left unchanged.
type UpdateCustomRole struct {
	ID          uuid.UUID
	Fields      CustomRoleUpdateFields
	Name        string
	Permissions []Permission
}

func (ur UpdateCustomRole) Validate() error {
	if ur.Fields.Name {
		trimmedName := strings.TrimSpace(ur.Name)
		if trimmedName == "" {
			return validationError("name required")
		}
		if trimmedName != ur.Name {
			return validationError("name must not have leading or trailing whitespace")
		}
	}
	if ur.Fields.Permissions {
		if err := validateCustomRolePerms(ur.Permissions); err != nil {
			return fmt.Errorf("validate permissions: %w", err)
		}
	}
	return nil
}

// CustomRoleUpdateFields specifies which fields on an UpdateCustomRole should be applied.
type CustomRoleUpdateFields struct {
	Name        bool
	Permissions bool
}

// ModifyCustomRolePermissions holds permission-set changes for a custom role.
type ModifyCustomRolePermissions struct {
	ID                uuid.UUID
	AddPermissions    []Permission
	RemovePermissions []Permission
}

func (mrp ModifyCustomRolePermissions) Validate() error {
	if set.FromSlice(mrp.AddPermissions).Intersection(set.FromSlice(mrp.RemovePermissions)).Len() != 0 {
		return validationError("permission cannot be both added and removed")
	}
	if err := validateCustomRolePerms(slices.Concat(mrp.AddPermissions, mrp.RemovePermissions)); err != nil {
		return fmt.Errorf("validate permissions: %w", err)
	}
	return nil
}

func validateCustomRolePerms(perms []Permission) error {
	for _, perm := range perms {
		if slices.Contains(SystemOnlyPermissions(), perm) {
			return validationError(fmt.Sprintf("%q is system-only", perm))
		}
	}
	return nil
}
