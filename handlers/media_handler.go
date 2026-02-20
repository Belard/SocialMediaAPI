package handlers

import (
	"SocialMediaAPI/config"
	"SocialMediaAPI/models"
	"SocialMediaAPI/services"
	"SocialMediaAPI/utils"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gorilla/mux"
)

// allowedUploadExtensions for quick handler-level rejection before reading the body.
var allowedUploadExtensions = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true,
	".gif": true, ".webp": true, ".mp4": true,
}

func (h *Handler) UploadMedia(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		utils.RespondWithError(w, http.StatusUnauthorized, "User ID not found in request context")
		return
	}

	// Reject requests with a Content-Length exceeding the absolute maximum early.
	cfg := config.Load()
	if r.ContentLength > cfg.MaxUploadSize {
		utils.RespondWithError(w, http.StatusRequestEntityTooLarge,
			fmt.Sprintf("Request body too large; maximum allowed is %d MB", cfg.MaxUploadSize/(1<<20)))
		return
	}

	if err := r.ParseMultipartForm(cfg.MaxUploadSize); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "File too large or malformed multipart request")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Error retrieving file: ensure the field name is 'file'")
		return
	}
	defer file.Close()

	// Quick extension check for fast rejection.
	ext := strings.ToLower(filepath.Ext(header.Filename))
	if !allowedUploadExtensions[ext] {
		utils.RespondWithError(w, http.StatusBadRequest,
			"File type not allowed; accepted extensions: .jpg, .jpeg, .png, .gif, .webp, .mp4")
		return
	}

	// Magic-number content verification — reject disguised/spoofed files early.
	kind, err := services.DetectFileType(file)
	if err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Unable to verify file type: "+err.Error())
		return
	}
	if !services.IsAllowedMIME(kind.MIME.Value) {
		utils.RespondWithError(w, http.StatusUnsupportedMediaType,
			fmt.Sprintf("File content type %s is not allowed; accepted: JPEG, PNG, GIF, WebP images and MP4 video", kind.MIME.Value))
		return
	}

	media, err := h.storage.SaveFile(file, header, userID)
	if err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := h.db.CreateMedia(media); err != nil {
		h.storage.DeleteFile(media)
		utils.RespondWithError(w, http.StatusInternalServerError, "Error saving media")
		return
	}

	utils.RespondWithJSON(w, http.StatusCreated, models.UploadResponse{Media: media})
}

func (h *Handler) GetMedia(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		utils.RespondWithError(w, http.StatusUnauthorized, "User ID not found in request context")
		return
	}

	mediaList, err := h.db.GetUserMedia(userID)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, "Error fetching media")
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, mediaList)
}

func (h *Handler) DeleteMedia(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		utils.RespondWithError(w, http.StatusUnauthorized, "User ID not found in request context")
		return
	}
	vars := mux.Vars(r)
	mediaID := vars["id"]

	media, err := h.db.GetMedia(mediaID)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, "Media not found")
		return
	}

	if media.UserID != userID {
		utils.RespondWithError(w, http.StatusForbidden, "Access denied")
		return
	}

	if err := h.storage.DeleteFile(media); err != nil {
		log.Printf("Error deleting file: %v", err)
	}

	if err := h.db.DeleteMedia(mediaID); err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, "Error deleting media")
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, map[string]string{"message": "Media deleted successfully"})
}