package services

import (
	"SocialMediaAPI/database"
	"SocialMediaAPI/models"
	"SocialMediaAPI/publishers"
	"sync"
	"time"
)

type PublisherService struct {
	db         *database.Database
	publishers map[models.Platform]publishers.PlatformPublisher
}

func NewPublisherService(db *database.Database) *PublisherService {
	return &PublisherService{
		db: db,
		publishers: map[models.Platform]publishers.PlatformPublisher{
			models.Twitter:   &publishers.TwitterPublisher{},
			models.Facebook:  &publishers.FacebookPublisher{},
			models.LinkedIn:  &publishers.LinkedInPublisher{},
			models.Instagram: &publishers.InstagramPublisher{},
			models.TikTok:    &publishers.TikTokPublisher{},
		},
	}
}

func (ps *PublisherService) PublishPost(post *models.Post) []models.PublishResult {
	var wg sync.WaitGroup
	results := make([]models.PublishResult, len(post.Platforms))

	for i, platform := range post.Platforms {
		wg.Add(1)
		go func(idx int, plt models.Platform) {
			defer wg.Done()

			publisher, ok := ps.publishers[plt]
			if !ok {
				results[idx] = models.PublishResult{
					Platform: plt,
					Success:  false,
					Message:  "Platform not supported",
				}
				return
			}

			credentials, _ := ps.db.GetCredentials(post.UserID, plt)
			result := publisher.Publish(post, credentials)
			results[idx] = result

			ps.db.SavePublishResult(post.ID, result)
		}(i, platform)
	}

	wg.Wait()

	now := time.Now()
	post.PublishedAt = &now
	post.Status = models.StatusPublished
	post.UpdatedAt = time.Now()
	ps.db.UpdatePost(post)

	return results
}