package utils

import (
	"encoding/json"
	"fmt"
	"io"
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

// RefreshFacebookToken attempts to exchange a short-lived token for a long-lived one
// For Facebook, user tokens can be exchanged once for a 60-day long-lived token
// This should be called immediately after getting the initial token from OAuth
func (t *TokenValidator) RefreshFacebookToken(cred *models.PlatformCredentials) error {
	cfg := config.Load()
	
	// Check if token is still valid via API call
	if !t.ValidateFacebookToken(cred.AccessToken) {
		return fmt.Errorf("token is no longer valid and cannot be refreshed")
	}

	// Attempt to exchange for long-lived token
	// Facebook allows exchanging user tokens for long-lived versions (60 days)
	exchangeURL := fmt.Sprintf(
		"https://graph.facebook.com/%s/oauth/access_token?grant_type=fb_exchange_token&client_id=%s&client_secret=%s&fb_exchange_token=%s",
		cfg.FacebookVersion,
		cfg.FacebookAppID,
		cfg.FacebookAppSecret,
		cred.AccessToken,
	)

	resp, err := http.Get(exchangeURL)
	if err != nil {
		// Exchange failed, but token is still valid - just extend current expiry
		newExpiry := time.Now().Add(24 * time.Hour)
		cred.ExpiresAt = &newExpiry
		return nil
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}

	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		// Exchange failed, but token is still valid
		newExpiry := time.Now().Add(24 * time.Hour)
		cred.ExpiresAt = &newExpiry
		return nil
	}

	// Successfully exchanged for long-lived token
	cred.AccessToken = tokenResp.AccessToken
	if tokenResp.ExpiresIn > 0 {
		newExpiry := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
		cred.ExpiresAt = &newExpiry
	}
	return nil
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
