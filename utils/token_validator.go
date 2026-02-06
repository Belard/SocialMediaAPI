package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"SocialMediaAPI/config"
	"SocialMediaAPI/models"
)

type TokenValidator struct{}

func NewTokenValidator() *TokenValidator {
	return &TokenValidator{}
}

// IsTokenExpired checks if a token is expired or will expire within a buffer time
func (t *TokenValidator) IsTokenExpired(cred *models.PlatformCredentials) bool {
	if cred.ExpiresAt == nil {
		// If no expiration set, assume it's valid (shouldn't happen with current implementation)
		return false
	}
	// Consider expired if less than 5 minutes remaining
	buffer := 5 * time.Minute
	return time.Now().Add(buffer).After(*cred.ExpiresAt)
}

// ValidateFacebookToken checks if a Facebook token is still valid
func (t *TokenValidator) ValidateFacebookToken(accessToken string) bool {
	cfg := config.Load()
	url := fmt.Sprintf("https://graph.facebook.com/%s/me?access_token=%s", cfg.FacebookVersion, accessToken)

	resp, err := http.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// RefreshFacebookToken attempts to refresh a Facebook token
// Note: Facebook User Tokens need to be obtained with long_lived_token during OAuth
// This function validates the token and returns it if still valid
func (t *TokenValidator) RefreshFacebookToken(cred *models.PlatformCredentials) error {
	// Check if token is still valid via API call
	if t.ValidateFacebookToken(cred.AccessToken) {
		// Token is still valid, extend expiration by typical duration
		newExpiry := time.Now().Add(60 * 24 * time.Hour) // Assume 60 days for long-lived tokens
		cred.ExpiresAt = &newExpiry
		return nil
	}

	return fmt.Errorf("token is no longer valid and cannot be refreshed")
}

// GetFacebookErrorCode extracts error code from Facebook API response
func (t *TokenValidator) GetFacebookErrorCode(body []byte) int {
	var fbError struct {
		Error struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	json.Unmarshal(body, &fbError)
	return fbError.Error.Code
}

// IsFacebookTokenExpiredError checks if the error is due to token expiration
func (t *TokenValidator) IsFacebookTokenExpiredError(body []byte) bool {
	var fbError struct {
		Error struct {
			Code    int    `json:"code"`
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	json.Unmarshal(body, &fbError)
	
	// Facebook error codes for expired/invalid tokens
	// 190: Invalid OAuth 2.0 Token
	// 192: Invalid Oauth token signature
	// 467: Throttling
	return fbError.Error.Code == 190 || fbError.Error.Code == 192 || 
		   (fbError.Error.Code == 467 && contains(fbError.Error.Message, "token"))
}

func contains(s, substr string) bool {
	if len(s) == 0 || len(substr) == 0 {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
