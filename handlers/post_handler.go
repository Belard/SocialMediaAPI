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

	// Default post_type to "normal" if not specified
	if post.PostType == "" {
		post.PostType = models.PostTypeNormal
	}

	// Validate post_type value
	if post.PostType != models.PostTypeNormal && post.PostType != models.PostTypeShort && post.PostType != models.PostTypeStory {
		utils.RespondWithError(w, http.StatusBadRequest,
			"Invalid post_type. Must be 'normal', 'short', or 'story'")
		return
	}

	// Default privacy_level to "public" if not specified
	if post.PrivacyLevel == "" {
		post.PrivacyLevel = models.PrivacyPublic
	}

	// Validate privacy_level value
	validPrivacy := map[models.PrivacyLevel]bool{
		models.PrivacyPublic:    true,
		models.PrivacyFollowers: true,
		models.PrivacyFriends:   true,
		models.PrivacyPrivate:   true,
	}
	if !validPrivacy[post.PrivacyLevel] {
		utils.RespondWithError(w, http.StatusBadRequest,
			"Invalid privacy_level. Must be 'public', 'followers', 'friends', or 'private'")
		return
	}

	// Enforce platform restrictions based on post_type
	if post.PostType == models.PostTypeNormal {
		// Normal posts cannot be published to TikTok
		for _, p := range post.Platforms {
			if p == models.TikTok {
				utils.RespondWithError(w, http.StatusBadRequest,
					"TikTok only supports short-form video posts. Set post_type to 'short' to publish to TikTok")
				return
			}
		}
	}

	if post.PostType == models.PostTypeShort {
		// Short posts only support platforms that accept short-form video: Instagram (Reels), Facebook (Reels), TikTok
		allowedShortPlatforms := map[models.Platform]bool{
			models.Instagram: true,
			models.Facebook:  true,
			models.TikTok:    true,
		}
		for _, p := range post.Platforms {
			if !allowedShortPlatforms[p] {
				utils.RespondWithError(w, http.StatusBadRequest,
					"Short posts only support instagram, facebook, and tiktok platforms")
				return
			}
		}

		// Short posts require at least one video
		hasVideo := false
		if len(post.MediaIDs) > 0 {
			mediaList, err := h.db.GetMediaByIDs(post.MediaIDs)
			if err == nil {
				for _, m := range mediaList {
					if m.Type == models.MediaVideo {
						hasVideo = true
						break
					}
				}
			}
		}
		if !hasVideo && len(post.MediaIDs) > 0 {
			utils.RespondWithError(w, http.StatusBadRequest,
				"Short posts require at least one video media attachment")
			return
		}
	}

	if post.PostType == models.PostTypeStory {
		// Story posts only support Facebook and Instagram
		allowedStoryPlatforms := map[models.Platform]bool{
			models.Facebook:  true,
			models.Instagram: true,
		}
		for _, p := range post.Platforms {
			if !allowedStoryPlatforms[p] {
				utils.RespondWithError(w, http.StatusBadRequest,
					"Story posts only support facebook and instagram platforms")
				return
			}
		}

		// Story posts require at least one media attachment (image or video)
		if len(post.MediaIDs) == 0 {
			utils.RespondWithError(w, http.StatusBadRequest,
				"Story posts require at least one image or video media attachment")
			return
		}
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
