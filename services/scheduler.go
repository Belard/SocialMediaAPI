package services

import (
	"SocialMediaAPI/database"
	"log"

	"github.com/robfig/cron/v3"
)

type Scheduler struct {
	cron      *cron.Cron
	db        *database.Database
	publisher *PublisherService
}

func NewScheduler(db *database.Database, publisher *PublisherService) *Scheduler {
	return &Scheduler{
		cron:      cron.New(),
		db:        db,
		publisher: publisher,
	}
}

func (s *Scheduler) Start() {
	s.cron.AddFunc("@every 1m", func() {
		posts, err := s.db.GetScheduledPosts()
		if err != nil {
			log.Printf("Error fetching scheduled posts: %v", err)
			return
		}

		for _, post := range posts {
			log.Printf("Publishing scheduled post: %s", post.ID)
			s.publisher.PublishPost(post)
		}
	})

	s.cron.Start()
	log.Println("Scheduler started")
}

func (s *Scheduler) Stop() {
	s.cron.Stop()
}