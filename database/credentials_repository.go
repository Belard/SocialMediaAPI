package database

import (
	"SocialMediaAPI/models"
	"SocialMediaAPI/utils"
	"database/sql"
)

func (d *Database) SaveCredentials(cred *models.PlatformCredentials) error {
	// Encrypt sensitive tokens before storing
	encryptedAccessToken, err := utils.EncryptToken(cred.AccessToken)
	if err != nil {
		return err
	}

	encryptedRefreshToken := ""
	if cred.RefreshToken != "" {
		encryptedRefreshToken, err = utils.EncryptToken(cred.RefreshToken)
		if err != nil {
			return err
		}
	}

	encryptedSecret := ""
	if cred.Secret != "" {
		encryptedSecret, err = utils.EncryptToken(cred.Secret)
		if err != nil {
			return err
		}
	}

	query := `INSERT INTO credentials (id, user_id, platform, access_token, refresh_token, secret, token_type, expires_at, created_at, updated_at)
			  VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
			  ON CONFLICT (user_id, platform) 
			  DO UPDATE SET access_token = $4, refresh_token = $5, secret = $6, token_type = $7, expires_at = $8, updated_at = $10`

	_, err = d.DB.Exec(query, cred.ID, cred.UserID, cred.Platform,
		encryptedAccessToken, encryptedRefreshToken, encryptedSecret, cred.TokenType, cred.ExpiresAt, cred.CreatedAt, cred.UpdatedAt)
	return err
}

func (d *Database) GetCredentials(userID string, platform models.Platform) (*models.PlatformCredentials, error) {
	cred := &models.PlatformCredentials{}
	query := `SELECT id, user_id, platform, access_token, refresh_token, secret, token_type, expires_at, created_at, updated_at
			  FROM credentials WHERE user_id = $1 AND platform = $2`

	err := d.DB.QueryRow(query, userID, platform).Scan(&cred.ID, &cred.UserID,
		&cred.Platform, &cred.AccessToken, &cred.RefreshToken, &cred.Secret, &cred.TokenType, &cred.ExpiresAt, &cred.CreatedAt, &cred.UpdatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	// Decrypt tokens after retrieving from database
	decryptedAccessToken, err := utils.DecryptToken(cred.AccessToken)
	if err != nil {
		return nil, err
	}
	cred.AccessToken = decryptedAccessToken

	if cred.RefreshToken != "" {
		decryptedRefreshToken, err := utils.DecryptToken(cred.RefreshToken)
		if err != nil {
			return nil, err
		}
		cred.RefreshToken = decryptedRefreshToken
	}

	if cred.Secret != "" {
		decryptedSecret, err := utils.DecryptToken(cred.Secret)
		if err != nil {
			return nil, err
		}
		cred.Secret = decryptedSecret
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