package handlers

import (
	"SocialMediaAPI/database"
	"SocialMediaAPI/services"
)

type Handler struct {
	db                *database.Database
	publisher         *services.PublisherService
	authService       *services.AuthService
	storage           *services.StorageService
	oauthStateService *services.OAuthStateService
}

func NewHandler(db *database.Database, publisher *services.PublisherService, authService *services.AuthService, storage *services.StorageService, oauthStateService *services.OAuthStateService) *Handler {
	return &Handler{
		db:                db,
		publisher:         publisher,
		authService:       authService,
		storage:           storage,
		oauthStateService: oauthStateService,
	}
}