package database

import "SocialMediaAPI/models"

func (d *Database) SaveCredentials(cred *models.PlatformCredentials) error {
	query := `INSERT INTO credentials (id, user_id, platform, access_token, secret, created_at)
			  VALUES ($1, $2, $3, $4, $5, $6)
			  ON CONFLICT (user_id, platform) 
			  DO UPDATE SET access_token = $4, secret = $5`

	_, err := d.DB.Exec(query, cred.ID, cred.UserID, cred.Platform,
		cred.AccessToken, cred.Secret, cred.CreatedAt)
	return err
}

func (d *Database) GetCredentials(userID string, platform models.Platform) (*models.PlatformCredentials, error) {
	cred := &models.PlatformCredentials{}
	query := `SELECT id, user_id, platform, access_token, secret, created_at 
			  FROM credentials WHERE user_id = $1 AND platform = $2`

	err := d.DB.QueryRow(query, userID, platform).Scan(&cred.ID, &cred.UserID,
		&cred.Platform, &cred.AccessToken, &cred.Secret, &cred.CreatedAt)

	if err != nil {
		return nil, err
	}
	return cred, nil
}

func (d *Database) SavePublishResult(postID string, result models.PublishResult) error {
	query := `INSERT INTO publish_results (post_id, platform, success, message, external_post_id)
			  VALUES ($1, $2, $3, $4, $5)`

	_, err := d.DB.Exec(query, postID, result.Platform, result.Success,
		result.Message, result.PostID)
	return err
}