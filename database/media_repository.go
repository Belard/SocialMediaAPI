package database

import "SocialMediaAPI/models"

func (d *Database) CreateMedia(media *models.Media) error {
	query := `INSERT INTO media (id, user_id, filename, path, url, type, size, mime_type, created_at)
			  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`
	_, err := d.DB.Exec(query, media.ID, media.UserID, media.Filename, media.Path,
		media.URL, media.Type, media.Size, media.MimeType, media.CreatedAt)
	return err
}

func (d *Database) GetMedia(id string) (*models.Media, error) {
	media := &models.Media{}
	query := `SELECT id, user_id, filename, path, url, type, size, mime_type, created_at
			  FROM media WHERE id = $1`
	err := d.DB.QueryRow(query, id).Scan(&media.ID, &media.UserID, &media.Filename,
		&media.Path, &media.URL, &media.Type, &media.Size, &media.MimeType, &media.CreatedAt)
	if err != nil {
		return nil, err
	}
	return media, nil
}

func (d *Database) GetMediaByIDs(ids []string) ([]*models.Media, error) {
	if len(ids) == 0 {
		return []*models.Media{}, nil
	}

	query := `SELECT id, user_id, filename, path, url, type, size, mime_type, created_at
			  FROM media WHERE id = ANY($1)`

	rows, err := d.DB.Query(query, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	mediaList := []*models.Media{}
	for rows.Next() {
		media := &models.Media{}
		err := rows.Scan(&media.ID, &media.UserID, &media.Filename, &media.Path,
			&media.URL, &media.Type, &media.Size, &media.MimeType, &media.CreatedAt)
		if err != nil {
			continue
		}
		mediaList = append(mediaList, media)
	}

	return mediaList, nil
}

func (d *Database) DeleteMedia(id string) error {
	query := `DELETE FROM media WHERE id = $1`
	_, err := d.DB.Exec(query, id)
	return err
}

func (d *Database) GetUserMedia(userID string) ([]*models.Media, error) {
	query := `SELECT id, user_id, filename, path, url, type, size, mime_type, created_at
			  FROM media WHERE user_id = $1 ORDER BY created_at DESC`

	rows, err := d.DB.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	mediaList := []*models.Media{}
	for rows.Next() {
		media := &models.Media{}
		err := rows.Scan(&media.ID, &media.UserID, &media.Filename, &media.Path,
			&media.URL, &media.Type, &media.Size, &media.MimeType, &media.CreatedAt)
		if err != nil {
			continue
		}
		mediaList = append(mediaList, media)
	}

	return mediaList, nil
}