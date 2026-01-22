package handlers

import (
	"SocialMediaAPI/models"
	"SocialMediaAPI/utils"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
)

func (h *Handler) CreatePost(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(string)

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
			utils.RespondWithError(w, http.StatusInternalServerError, "Error creating post")
			return
		}
		utils.RespondWithJSON(w, http.StatusCreated, post)
	} else {
		post.Status = models.StatusDraft
		if err := h.db.CreatePost(&post); err != nil {
			utils.RespondWithError(w, http.StatusInternalServerError, "Error creating post")
			return
		}

		results := h.publisher.PublishPost(&post)
		response := models.PublishResponse{
			PostID:  post.ID,
			Results: results,
		}
		utils.RespondWithJSON(w, http.StatusCreated, response)
	}
}

func (h *Handler) GetPosts(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(string)

	posts, err := h.db.GetUserPosts(userID)
	if err != nil {
		utils.RespondWithError(w, http.StatusInternalServerError, "Error fetching posts")
		return
	}

	utils.RespondWithJSON(w, http.StatusOK, posts)
}

func (h *Handler) GetPost(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(string)
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

func (h *Handler) SaveCredentials(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(string)

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

	utils.RespondWithJSON(w, http.StatusOK, map[string]string{"message": "Credentials saved successfully"})
}