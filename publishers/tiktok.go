package publishers

import (
	"SocialMediaAPI/models"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type TikTokPublisher struct{}

func (t *TikTokPublisher) Publish(post *models.Post, cred *models.PlatformCredentials) models.PublishResult {
	time.Sleep(900 * time.Millisecond)

	if cred == nil || cred.AccessToken == "" {
		return models.PublishResult{
			Platform: models.TikTok,
			Success:  false,
			Message:  "Missing TikTok credentials",
		}
	}

	hasVideo := false
	for _, media := range post.Media {
		if media.Type == models.MediaVideo {
			hasVideo = true
			break
		}
	}

	if !hasVideo {
		return models.PublishResult{
			Platform: models.TikTok,
			Success:  false,
			Message:  "TikTok requires a video",
		}
	}

	return models.PublishResult{
		Platform: models.TikTok,
		Success:  true,
		Message:  "Published successfully on TikTok",
		PostID:   fmt.Sprintf("tt_%s", uuid.New().String()[:8]),
	}
}