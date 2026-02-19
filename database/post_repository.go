package database

import (
	"SocialMediaAPI/models"
	"time"

	"github.com/lib/pq"
)

func (d *Database) CreatePost(post *models.Post) error {
	query := `INSERT INTO posts (id, user_id, content, post_type, media_ids, platforms, status, scheduled_for, created_at, updated_at)
			  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`

	platforms := make([]string, len(post.Platforms))
	for i, p := range post.Platforms {
		platforms[i] = string(p)
	}

	_, err := d.DB.Exec(query, post.ID, post.UserID, post.Content, post.PostType, pq.Array(post.MediaIDs),
		pq.Array(platforms), post.Status, post.ScheduledFor, post.CreatedAt, post.UpdatedAt)
	return err
}

func (d *Database) UpdatePost(post *models.Post) error {
	query := `UPDATE posts SET content = $1, post_type = $2, media_ids = $3, platforms = $4, 
			  status = $5, scheduled_for = $6, published_at = $7, updated_at = $8
			  WHERE id = $9`

	platforms := make([]string, len(post.Platforms))
	for i, p := range post.Platforms {
		platforms[i] = string(p)
	}

	_, err := d.DB.Exec(query, post.Content, post.PostType, pq.Array(post.MediaIDs), pq.Array(platforms),
		post.Status, post.ScheduledFor, post.PublishedAt, post.UpdatedAt, post.ID)
	return err
}

func (d *Database) GetPost(id string) (*models.Post, error) {
	post := &models.Post{}
	var platforms []string
	var mediaIDs []string

	query := `SELECT id, user_id, content, post_type, media_ids, platforms, status, 
			  scheduled_for, published_at, created_at, updated_at 
			  FROM posts WHERE id = $1`

	err := d.DB.QueryRow(query, id).Scan(&post.ID, &post.UserID, &post.Content,
		&post.PostType, pq.Array(&mediaIDs), pq.Array(&platforms), &post.Status, &post.ScheduledFor,
		&post.PublishedAt, &post.CreatedAt, &post.UpdatedAt)

	if err != nil {
		return nil, err
	}

	post.Platforms = make([]models.Platform, len(platforms))
	for i, p := range platforms {
		post.Platforms[i] = models.Platform(p)
	}

	if mediaIDs != nil {
		post.MediaIDs = mediaIDs
		post.Media, _ = d.GetMediaByIDs(mediaIDs)
	}

	return post, nil
}

func (d *Database) GetUserPosts(userID string) ([]*models.Post, error) {
	query := `SELECT id, user_id, content, post_type, media_ids, platforms, status, 
			  scheduled_for, published_at, created_at, updated_at 
			  FROM posts WHERE user_id = $1 ORDER BY created_at DESC`

	rows, err := d.DB.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	posts := []*models.Post{}
	for rows.Next() {
		post := &models.Post{}
		var platforms []string
		var mediaIDs []string

		err := rows.Scan(&post.ID, &post.UserID, &post.Content, &post.PostType, pq.Array(&mediaIDs),
			pq.Array(&platforms), &post.Status, &post.ScheduledFor, &post.PublishedAt,
			&post.CreatedAt, &post.UpdatedAt)

		if err != nil {
			continue
		}

		post.Platforms = make([]models.Platform, len(platforms))
		for i, p := range platforms {
			post.Platforms[i] = models.Platform(p)
		}

		if mediaIDs != nil {
			post.MediaIDs = mediaIDs
			post.Media, _ = d.GetMediaByIDs(mediaIDs)
		}

		posts = append(posts, post)
	}

	return posts, nil
}

func (d *Database) GetScheduledPosts() ([]*models.Post, error) {
	query := `SELECT id, user_id, content, post_type, media_ids, platforms, status, 
			  scheduled_for, published_at, created_at, updated_at 
			  FROM posts WHERE status = $1 AND scheduled_for <= $2`

	rows, err := d.DB.Query(query, models.StatusScheduled, time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	posts := []*models.Post{}
	for rows.Next() {
		post := &models.Post{}
		var platforms []string
		var mediaIDs []string

		err := rows.Scan(&post.ID, &post.UserID, &post.Content, &post.PostType, pq.Array(&mediaIDs),
			pq.Array(&platforms), &post.Status, &post.ScheduledFor, &post.PublishedAt,
			&post.CreatedAt, &post.UpdatedAt)

		if err != nil {
			continue
		}

		post.Platforms = make([]models.Platform, len(platforms))
		for i, p := range platforms {
			post.Platforms[i] = models.Platform(p)
		}

		if mediaIDs != nil {
			post.MediaIDs = mediaIDs
			post.Media, _ = d.GetMediaByIDs(mediaIDs)
		}

		posts = append(posts, post)
	}

	return posts, nil
}