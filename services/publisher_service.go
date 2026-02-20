package services

import (
	"SocialMediaAPI/database"
	"SocialMediaAPI/models"
	"SocialMediaAPI/publishers"
	"SocialMediaAPI/utils"
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
			models.Twitter:   publishers.NewTwitterPublisher(nil),
			models.Facebook:  publishers.NewFacebookPublisher(nil),
			models.LinkedIn:  &publishers.LinkedInPublisher{},
			models.Instagram: publishers.NewInstagramPublisher(nil),
			models.TikTok:    publishers.NewTikTokPublisher(nil),
			models.YouTube:   publishers.NewYouTubePublisher(nil),
		},
	}
}

func (ps *PublisherService) PublishPost(post *models.Post) []models.PublishResult {
	utils.Infof("starting publish post_id=%s user_id=%s platforms=%d media=%d", post.ID, post.UserID, len(post.Platforms), len(post.Media))

	var wg sync.WaitGroup
	results := make([]models.PublishResult, len(post.Platforms))

	for i, platform := range post.Platforms {
		wg.Add(1)
		go func(idx int, plt models.Platform) {
			defer wg.Done()
			utils.Debugf("processing platform post_id=%s platform=%s", post.ID, plt)

			publisher, ok := ps.publishers[plt]
			if !ok {
				utils.Warnf("platform not supported post_id=%s platform=%s", post.ID, plt)
				results[idx] = models.PublishResult{
					Platform: plt,
					Success:  false,
					Message:  "Platform not supported",
				}
				return
			}

			credentials, err := ps.db.GetCredentials(post.UserID, plt)
			if err != nil {
				utils.Warnf("credentials lookup failed post_id=%s user_id=%s platform=%s err=%v", post.ID, post.UserID, plt, err)
			} else if credentials == nil || credentials.AccessToken == "" {
				utils.Warnf("credentials missing or empty post_id=%s user_id=%s platform=%s", post.ID, post.UserID, plt)
			} else {
				utils.Debugf("credentials loaded post_id=%s user_id=%s platform=%s", post.ID, post.UserID, plt)
			}

			result := publisher.Publish(post, credentials)
			results[idx] = result
			if result.Success {
				utils.Infof("platform publish success post_id=%s platform=%s external_post_id=%s", post.ID, plt, result.PostID)
			} else {
				utils.Errorf("platform publish failed post_id=%s platform=%s message=%s", post.ID, plt, result.Message)
			}

			if err := ps.db.SavePublishResult(post.ID, result); err != nil {
				utils.Errorf("failed to save publish result post_id=%s platform=%s err=%v", post.ID, plt, err)
			}
		}(i, platform)
	}

	wg.Wait()

	allSucceeded := len(results) > 0
	for _, result := range results {
		if !result.Success {
			allSucceeded = false
			break
		}
	}

	if allSucceeded {
		now := time.Now()
		post.PublishedAt = &now
		post.Status = models.StatusPublished
		utils.Infof("post publish completed status=published post_id=%s", post.ID)
	} else {
		post.PublishedAt = nil
		post.Status = models.StatusFailed
		utils.Warnf("post publish completed status=failed post_id=%s", post.ID)
	}

	post.UpdatedAt = time.Now()
	if err := ps.db.UpdatePost(post); err != nil {
		utils.Errorf("failed to update post status post_id=%s status=%s err=%v", post.ID, post.Status, err)
	} else {
		utils.Debugf("post status persisted post_id=%s status=%s", post.ID, post.Status)
	}

	utils.Infof("finished publish post_id=%s success=%t", post.ID, allSucceeded)

	return results
}
