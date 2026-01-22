package publishers

import (
	"SocialMediaAPI/models"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type FacebookPublisher struct{}

func (f *FacebookPublisher) Publish(post *models.Post, cred *models.PlatformCredentials) models.PublishResult {
	time.Sleep(600 * time.Millisecond)

	if cred == nil || cred.AccessToken == "" {
		return models.PublishResult{
			Platform: models.Facebook,
			Success:  false,
			Message:  "Missing Facebook credentials",
		}
	}

	return models.PublishResult{
		Platform: models.Facebook,
		Success:  true,
		Message:  "Published successfully on Facebook",
		PostID:   fmt.Sprintf("fb_%s", uuid.New().String()[:8]),
	}
}