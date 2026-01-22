package publishers

import (
	"SocialMediaAPI/models"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type InstagramPublisher struct{}

func (i *InstagramPublisher) Publish(post *models.Post, cred *models.PlatformCredentials) models.PublishResult {
	time.Sleep(800 * time.Millisecond)

	if cred == nil || cred.AccessToken == "" {
		return models.PublishResult{
			Platform: models.Instagram,
			Success:  false,
			Message:  "Missing Instagram credentials",
		}
	}

	hasImage := false
	for _, media := range post.Media {
		if media.Type == models.MediaImage {
			hasImage = true
			break
		}
	}

	if !hasImage {
		return models.PublishResult{
			Platform: models.Instagram,
			Success:  false,
			Message:  "Instagram requires at least one image",
		}
	}

	return models.PublishResult{
		Platform: models.Instagram,
		Success:  true,
		Message:  "Published successfully on Instagram",
		PostID:   fmt.Sprintf("ig_%s", uuid.New().String()[:8]),
	}
}
