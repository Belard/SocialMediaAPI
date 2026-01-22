package publishers

import (
	"SocialMediaAPI/models"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type TwitterPublisher struct{}

func (t *TwitterPublisher) Publish(post *models.Post, cred *models.PlatformCredentials) models.PublishResult {
	time.Sleep(500 * time.Millisecond)

	if cred == nil || cred.AccessToken == "" {
		return models.PublishResult{
			Platform: models.Twitter,
			Success:  false,
			Message:  "Missing Twitter credentials",
		}
	}

	return models.PublishResult{
		Platform: models.Twitter,
		Success:  true,
		Message:  "Published successfully on Twitter",
		PostID:   fmt.Sprintf("tw_%s", uuid.New().String()[:8]),
	}
}