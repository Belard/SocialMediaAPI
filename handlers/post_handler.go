package handlers

import (
	"SocialMediaAPI/models"
	"SocialMediaAPI/utils"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

func (h *Handler) CreatePost(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		utils.RespondWithError(w, http.StatusUnauthorized, "User ID not found in request context")
		return
	}

	var post models.Post
	if err := json.NewDecoder(r.Body).Decode(&post); err != nil {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	if post.Content == "" {
		utils.RespondWithError(w, http.StatusBadRequest, "Content is required")
		return
	}

	if len(post.Platforms) == 0 {
		utils.RespondWithError(w, http.StatusBadRequest, "At least one platform is required")
		return
	}

	if len(post.MediaIDs) > 0 {
		mediaList, err := h.db.GetMediaByIDs(post.MediaIDs)
		if err != nil {
			utils.RespondWithError(w, http.StatusBadRequest, "Invalid media IDs")
			return
		}

		requestedMedia := make(map[string]struct{}, len(post.MediaIDs))
		for _, mediaID := range post.MediaIDs {
			requestedMedia[mediaID] = struct{}{}
		}

		for _, media := range mediaList {
			delete(requestedMedia, media.ID)
		}

		if len(requestedMedia) > 0 {
			utils.RespondWithError(w, http.StatusBadRequest, "One or more media IDs were not found")
			return
		}

		for _, media := range mediaList {
			if media.UserID != userID {
				utils.RespondWithError(w, http.StatusForbidden, "Access denied to media")
				return
			}
		}

		post.Media = mediaList
	}

	post.ID = uuid.New().String()
	post.UserID = userID
	post.CreatedAt = time.Now()
	post.UpdatedAt = time.Now()

	if post.ScheduledFor != nil && post.ScheduledFor.After(time.Now()) {
		post.Status = models.StatusScheduled
		if err := h.db.CreatePost(&post); err != nil {
			utils.RespondWithError(w, http.StatusInternalServerError, "Error creating post scheduled for future")
			return
		}
		utils.RespondWithJSON(w, http.StatusCreated, post)
	} else {
		post.Status = models.StatusDraft
		if err := h.db.CreatePost(&post); err != nil {
			utils.RespondWithError(w, http.StatusInternalServerError, "Error creating post now")
			return
		}

		results := h.publisher.PublishPost(&post)
		failedPlatforms := make([]string, 0)
		for _, result := range results {
			if !result.Success {
				failedPlatforms = append(failedPlatforms, string(result.Platform))
			}
		}

		response := models.PublishResponse{
			PostID:  post.ID,
			Results: results,
		}

		if len(failedPlatforms) > 0 {
			utils.RespondWithJSON(w, http.StatusBadGateway, map[string]interface{}{
				"error":             "Failed to publish to one or more platforms",
				"failed_platforms":  failedPlatforms,
				"publish_response": response,
				"message":           "Check publish_response.results for platform-specific details",
				"failed_summary":    "Failed platforms: " + strings.Join(failedPlatforms, ", "),
			})
			return
		}

		utils.RespondWithJSON(w, http.StatusCreated, response)
	}
}

func (h *Handler) GetPosts(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		utils.RespondWithError(w, http.StatusUnauthorized, "User ID not found in request context")
		return
	}

	posts, err := h.db.GetUserPosts(userID)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, "Error fetching posts")
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, posts)
}

func (h *Handler) GetPost(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		utils.RespondWithError(w, http.StatusUnauthorized, "User ID not found in request context")
		return
	}
	vars := mux.Vars(r)
	postID := vars["id"]

	post, err := h.db.GetPost(postID)
	if err != nil {
		utils.RespondWithError(w, http.StatusNotFound, "Post not found")
		return
	}

	if post.UserID != userID {
		utils.RespondWithError(w, http.StatusForbidden, "Access denied")
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, post)
}
