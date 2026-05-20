package publishers

import (
	"SocialMediaAPI/models"
	"SocialMediaAPI/utils"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type LinkedInPublisher struct{}

func (l *LinkedInPublisher) Publish(post *models.Post, cred *models.PlatformCredentials) models.PublishResult {
	time.Sleep(700 * time.Millisecond)

	if cred == nil || cred.AccessToken == "" {
		return models.PublishResult{
			Platform: models.LinkedIn,
			Success:  false,
			Message:  "Missing LinkedIn credentials",
		}
	}

	// Check if token is expired
	tokenValidator := utils.NewTokenValidator()
	if tokenValidator.IsTokenExpired(cred) {
		return models.PublishResult{
			Platform: models.LinkedIn,
			Success:  false,
			Message:  "LinkedIn token has expired. Please reconnect your account via OAuth",
		}
	}

	// LinkedIn does NOT support stories or short-form video posts.
	if post.PostType == models.PostTypeStory {
		return models.PublishResult{
			Platform: models.LinkedIn,
			Success:  false,
			Message:  "LinkedIn does not support stories. Use post_type 'normal' instead",
		}
	}

	return models.PublishResult{
		Platform: models.LinkedIn,
		Success:  true,
		Message:  "Published successfully on LinkedIn",
		PostID:   fmt.Sprintf("li_%s", uuid.New().String()[:8]),
	}
}
