package auth

import (
	"time"

	"github.com/zorcal/theapp/backend/internal/core/mdl"
	"github.com/zorcal/theapp/backend/internal/core/pgstores/pgauth"
	"github.com/zorcal/theapp/backend/pkg/x/slicesx"
)

func createMagicLinkTokenToPg(userID int, hash string, expiresAt time.Time) pgauth.CreateMagicLinkToken {
	return pgauth.CreateMagicLinkToken{
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: expiresAt,
	}
}

func createRefreshTokenToPg(userID int, hash string, expiresAt time.Time) pgauth.CreateRefreshToken {
	return pgauth.CreateRefreshToken{
		UserID:    userID,
		TokenHash: hash,
		ExpiresAt: expiresAt,
	}
}

func permissionsFromPg(names []string) []mdl.Permission {
	return slicesx.Map(names, func(n string) mdl.Permission { return mdl.Permission(n) })
}

func authSessionFromPg(data pgauth.AuthSessionData, permissionNames []string) mdl.AuthSession {
	return mdl.AuthSession{
		User: mdl.AuthUser{
			UserID:      data.UserExternalID,
			Email:       data.Email,
			Permissions: permissionsFromPg(permissionNames),
		},
		Project: authProjectFromPg(data),
	}
}

func authProjectFromPg(data pgauth.AuthSessionData) *mdl.AuthProject {
	if data.ProjectID == nil {
		return nil
	}
	if data.OrgID == nil || data.OrgName == nil || data.IsControlProject == nil || data.IsOrgMember == nil {
		// A project row always belongs to an organization and carries its control-project marker.
		panic("incomplete project metadata in auth session data")
	}

	return &mdl.AuthProject{
		ID:          *data.ProjectID,
		OrgID:       *data.OrgID,
		OrgName:     *data.OrgName,
		IsControl:   *data.IsControlProject,
		IsOrgMember: *data.IsOrgMember,
	}
}
