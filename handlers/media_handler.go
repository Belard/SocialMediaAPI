package handlers

import (
	"SocialMediaAPI/config"
	"SocialMediaAPI/models"
	"SocialMediaAPI/utils"
	"log"
	"net/http"

	"github.com/gorilla/mux"
)

func (h *Handler) UploadMedia(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(string)

	if err := r.ParseMultipartForm(config.Load().MaxUploadSize); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "File too large")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Error retrieving file")
		return
	}
	defer file.Close()

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
	userID := r.Context().Value("userID").(string)

	mediaList, err := h.db.GetUserMedia(userID)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, "Error fetching media")
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, mediaList)
}

func (h *Handler) DeleteMedia(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(string)
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