package publishers

import (
	"SocialMediaAPI/models"
)

type PlatformPublisher interface {
	Publish(post *models.Post, credentials *models.PlatformCredentials) models.PublishResult
}