package oauth

import (
	"SocialMediaAPI/config"
	"SocialMediaAPI/models"
	"SocialMediaAPI/utils"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

var youtubeHTTPClient = &http.Client{Timeout: 15 * time.Second}

// InitiateYouTubeOAuth starts the Google/YouTube OAuth 2.0 flow.
func (h *OAuthHandler) InitiateYouTubeOAuth(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		utils.Warnf("youtube oauth initiate unauthorized: missing user id in context")
		utils.RespondWithError(w, http.StatusUnauthorized, "User ID not found in request context")
		return
	}

	cfg := config.Load()

	if cfg.YouTubeClientID == "" {
		utils.Errorf("youtube oauth initiate config missing: YOUTUBE_CLIENT_ID")
		utils.RespondWithError(w, http.StatusInternalServerError,
			"YouTube Client ID not configured. Set YOUTUBE_CLIENT_ID environment variable")
		return
	}

	if cfg.YouTubeRedirectURI == "" {
		utils.Errorf("youtube oauth initiate config missing: YOUTUBE_REDIRECT_URI")
		utils.RespondWithError(w, http.StatusInternalServerError,
			"YouTube Redirect URI not configured. Set YOUTUBE_REDIRECT_URI environment variable")
		return
	}

	state := h.oauthStateService.GenerateState(userID, "youtube")

	// Google OAuth 2.0 Authorization URL
	params := url.Values{}
	params.Set("client_id", cfg.YouTubeClientID)
	params.Set("redirect_uri", cfg.YouTubeRedirectURI)
	params.Set("response_type", "code")
	params.Set("scope", strings.Join([]string{
		"https://www.googleapis.com/auth/youtube.upload",
		"https://www.googleapis.com/auth/youtube.readonly",
	}, " "))
	params.Set("state", state)
	params.Set("access_type", "offline")  // Request a refresh token
	params.Set("prompt", "consent")       // Force consent screen so we always get a refresh token

	authURL := "https://accounts.google.com/o/oauth2/v2/auth?" + params.Encode()
	utils.Infof("youtube oauth initiate success user_id=%s", userID)

	utils.RespondWithJSON(w, http.StatusOK, map[string]string{
		"auth_url": authURL,
		"state":    state,
	})
}

// HandleYouTubeCallback handles the OAuth callback from Google/YouTube.
func (h *OAuthHandler) HandleYouTubeCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errorParam := r.URL.Query().Get("error")

	utils.Infof("youtube callback received remote=%s has_code=%t has_state=%t has_error=%t",
		r.RemoteAddr, code != "", state != "", errorParam != "")

	if errorParam != "" {
		errorDesc := r.URL.Query().Get("error_description")
		utils.Warnf("youtube callback oauth error error=%s description=%s", errorParam, errorDesc)
		http.Redirect(w, r, fmt.Sprintf("/oauth/error?error=%s&description=%s",
			errorParam, url.QueryEscape(errorDesc)), http.StatusFound)
		return
	}

	if code == "" {
		utils.Warnf("youtube callback missing authorization code")
		utils.RespondWithError(w, http.StatusBadRequest, "Missing authorization code")
		return
	}

	if state == "" {
		utils.Warnf("youtube callback missing state parameter")
		utils.RespondWithError(w, http.StatusBadRequest, "Missing state parameter")
		return
	}

	oauthState, valid := h.oauthStateService.ValidateState(state)
	if !valid {
		utils.Warnf("youtube callback invalid or expired state")
		utils.RespondWithError(w, http.StatusBadRequest,
			"Invalid or expired state token. Please try connecting again.")
		return
	}

	if oauthState.Platform != "youtube" {
		utils.Warnf("youtube callback invalid platform in state platform=%s", oauthState.Platform)
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid state for YouTube OAuth")
		return
	}

	userID := oauthState.UserID

	// Exchange authorization code for access + refresh token
	accessToken, refreshToken, expiresIn, err := h.exchangeCodeForYouTubeToken(code)
	if err != nil {
		utils.Errorf("youtube token exchange failed user_id=%s err=%v", userID, err)
		http.Redirect(w, r, fmt.Sprintf("/oauth/error?error=token_exchange&description=%s",
			url.QueryEscape(err.Error())), http.StatusFound)
		return
	}
	utils.Infof("youtube token exchange success user_id=%s expires_in=%d", userID, expiresIn)

	// Fetch the YouTube channel identity
	youtubeChannelID, err := h.getYouTubeChannelIdentity(accessToken)
	if err != nil {
		utils.Warnf("youtube identity fetch failed (non-fatal) user_id=%s err=%v", userID, err)
		youtubeChannelID = ""
	} else {
		utils.Infof("youtube identity fetch success user_id=%s channel_id=%s", userID, youtubeChannelID)
	}

	var expiresAt *time.Time
	if expiresIn > 0 {
		expTime := time.Now().Add(time.Duration(expiresIn) * time.Second)
		expiresAt = &expTime
	}

	cred := &models.PlatformCredentials{
		ID:             uuid.New().String(),
		UserID:         userID,
		Platform:       models.YouTube,
		AccessToken:    accessToken,
		RefreshToken:   refreshToken,
		TokenType:      "Bearer",
		ExpiresAt:      expiresAt,
		PlatformUserID: youtubeChannelID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := h.db.SaveCredentials(cred); err != nil {
		utils.Errorf("youtube save credentials failed user_id=%s channel_id=%s err=%v", userID, youtubeChannelID, err)
		http.Redirect(w, r, "/oauth/error?error=save_failed&description=Failed+to+save+credentials",
			http.StatusFound)
		return
	}

	utils.Infof("youtube credentials saved user_id=%s platform=%s channel_id=%s", userID, models.YouTube, youtubeChannelID)
	utils.Infof("youtube callback completed successfully user_id=%s", userID)

	http.Redirect(w, r, "/oauth/success?platform=youtube", http.StatusFound)
}

// exchangeCodeForYouTubeToken exchanges the authorization code for tokens via Google's token endpoint.
// Returns: accessToken, refreshToken, expiresIn, error
func (h *OAuthHandler) exchangeCodeForYouTubeToken(code string) (string, string, int, error) {
	cfg := config.Load()
	utils.Debugf("youtube token exchange request start")

	tokenURL := "https://oauth2.googleapis.com/token"

	form := url.Values{}
	form.Set("code", code)
	form.Set("client_id", cfg.YouTubeClientID)
	form.Set("client_secret", cfg.YouTubeClientSecret)
	form.Set("redirect_uri", cfg.YouTubeRedirectURI)
	form.Set("grant_type", "authorization_code")

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := youtubeHTTPClient.Do(req)
	if err != nil {
		return "", "", 0, fmt.Errorf("youtube token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to read token response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", 0, fmt.Errorf("youtube token exchange failed (status %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", "", 0, fmt.Errorf("failed to parse token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", "", 0, fmt.Errorf("google returned empty access token")
	}

	utils.Debugf("youtube token exchange success expires_in=%d has_refresh=%t", tokenResp.ExpiresIn, tokenResp.RefreshToken != "")
	return tokenResp.AccessToken, tokenResp.RefreshToken, tokenResp.ExpiresIn, nil
}

// getYouTubeChannelIdentity fetches the authenticated user's YouTube channel ID.
func (h *OAuthHandler) getYouTubeChannelIdentity(accessToken string) (string, error) {
	utils.Debugf("youtube identity fetch start")

	endpoint := "https://www.googleapis.com/youtube/v3/channels?part=id&mine=true"

	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create identity request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := youtubeHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("youtube identity request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read identity response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("youtube channels API error (status %d): %s", resp.StatusCode, string(body))
	}

	var channelResp struct {
		Items []struct {
			ID string `json:"id"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &channelResp); err != nil {
		return "", fmt.Errorf("failed to parse channels response: %w", err)
	}

	if len(channelResp.Items) == 0 {
		return "", fmt.Errorf("no YouTube channel found for this account")
	}

	channelID := channelResp.Items[0].ID
	utils.Debugf("youtube identity fetch success channel_id=%s", channelID)
	return channelID, nil
}
