package handlers

import (
	"SocialMediaAPI/database"
	"SocialMediaAPI/services"
)

type Handler struct {
	db          *database.Database
	publisher   *services.PublisherService
	authService *services.AuthService
	storage     *services.StorageService
}

func NewHandler(db *database.Database, publisher *services.PublisherService, authService *services.AuthService, storage *services.StorageService) *Handler {
	return &Handler{
		db:          db,
		publisher:   publisher,
		authService: authService,
		storage:     storage,
	}
}