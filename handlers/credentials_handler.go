package handlers

import (
	"SocialMediaAPI/models"
	"SocialMediaAPI/utils"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// SaveCredentials saves platform credentials for the authenticated user
func (h *Handler) SaveCredentials(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		utils.RespondWithError(w, http.StatusUnauthorized, "User ID not found in request context")
		return
	}

	var cred models.PlatformCredentials
	if err := json.NewDecoder(r.Body).Decode(&cred); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	if cred.Platform == "" || cred.AccessToken == "" {
		utils.RespondWithError(w, http.StatusBadRequest, "Platform and access_token are required")
		return
	}

	cred.ID = uuid.New().String()
	cred.UserID = userID
	cred.CreatedAt = time.Now()

	if err := h.db.SaveCredentials(&cred); err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, "Error saving credentials")
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, map[string]string{
		"message": "Credentials saved successfully",
	})
}

// GetConnectedPlatforms returns which platforms the user has connected
func (h *Handler) GetConnectedPlatforms(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		utils.RespondWithError(w, http.StatusUnauthorized, "User ID not found in request context")
		return
	}

	query := `SELECT platform, created_at FROM credentials WHERE user_id = $1`

	rows, err := h.db.DB.Query(query, userID)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, "Error fetching credentials")
		return
	}
	defer rows.Close()

	type ConnectedPlatform struct {
		Platform  string    `json:"platform"`
		Connected bool      `json:"connected"`
		CreatedAt time.Time `json:"created_at,omitempty"`
	}

	connectedMap := make(map[string]time.Time)
	for rows.Next() {
		var platform string
		var createdAt time.Time
		if err := rows.Scan(&platform, &createdAt); err != nil {
			utils.RespondWithError(w, http.StatusInternalServerError, "Error reading credentials")
			return
		}
		connectedMap[platform] = createdAt
	}
	if err := rows.Err(); err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, "Error reading credentials")
		return
	}

	// All platforms
	allPlatforms := []models.Platform{
		models.Twitter,
		models.Facebook,
		models.LinkedIn,
		models.Instagram,
		models.TikTok,
	}

	platforms := []ConnectedPlatform{}
	for _, platform := range allPlatforms {
		if createdAt, connected := connectedMap[string(platform)]; connected {
			platforms = append(platforms, ConnectedPlatform{
				Platform:  string(platform),
				Connected: true,
				CreatedAt: createdAt,
			})
		} else {
			platforms = append(platforms, ConnectedPlatform{
				Platform:  string(platform),
				Connected: false,
			})
		}
	}

	utils.RespondWithJSON(w, http.StatusOK, map[string]interface{}{
		"user_id":   userID,
		"platforms": platforms,
	})
}

// DisconnectPlatform removes credentials for a specific platform
func (h *Handler) DisconnectPlatform(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		utils.RespondWithError(w, http.StatusUnauthorized, "User ID not found in request context")
		return
	}

	var req struct {
		Platform string `json:"platform"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request")
		return
	}

	query := `DELETE FROM credentials WHERE user_id = $1 AND platform = $2`
	result, err := h.db.DB.Exec(query, userID, req.Platform)

	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, "Error disconnecting platform")
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		utils.RespondWithError(w, http.StatusNotFound, "Platform was not connected")
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, map[string]string{
		"message": fmt.Sprintf("%s disconnected successfully", req.Platform),
	})
}
