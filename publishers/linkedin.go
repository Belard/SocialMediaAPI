package publishers

import (
	"SocialMediaAPI/models"
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

	return models.PublishResult{
		Platform: models.LinkedIn,
		Success:  true,
		Message:  "Published successfully on LinkedIn",
		PostID:   fmt.Sprintf("li_%s", uuid.New().String()[:8]),
	}
}
