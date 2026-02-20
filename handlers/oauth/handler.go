package oauth

import (
	"SocialMediaAPI/database"
	"SocialMediaAPI/services"
)

// OAuthHandler holds dependencies for all OAuth-related HTTP handlers.
type OAuthHandler struct {
	db                *database.Database
	oauthStateService *services.OAuthStateService
}

// NewOAuthHandler creates a new OAuthHandler with the required dependencies.
func NewOAuthHandler(db *database.Database, oauthStateService *services.OAuthStateService) *OAuthHandler {
	return &OAuthHandler{
		db:                db,
		oauthStateService: oauthStateService,
	}
}
