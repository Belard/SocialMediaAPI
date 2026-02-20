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

var twitterHTTPClient = &http.Client{Timeout: 15 * time.Second}

// InitiateTwitterOAuth starts the Twitter/X OAuth 2.0 flow with PKCE.
func (h *OAuthHandler) InitiateTwitterOAuth(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		utils.Warnf("twitter oauth initiate unauthorized: missing user id in context")
		utils.RespondWithError(w, http.StatusUnauthorized, "User ID not found in request context")
		return
	}

	cfg := config.Load()

	if cfg.TwitterClientID == "" {
		utils.Errorf("twitter oauth initiate config missing: TWITTER_CLIENT_ID")
		utils.RespondWithError(w, http.StatusInternalServerError,
			"Twitter Client ID not configured. Set TWITTER_CLIENT_ID environment variable")
		return
	}

	if cfg.TwitterRedirectURI == "" {
		utils.Errorf("twitter oauth initiate config missing: TWITTER_REDIRECT_URI")
		utils.RespondWithError(w, http.StatusInternalServerError,
			"Twitter Redirect URI not configured. Set TWITTER_REDIRECT_URI environment variable")
		return
	}

	state := h.oauthStateService.GenerateState(userID, "twitter")

	// Twitter OAuth 2.0 uses PKCE (same pattern as TikTok)
	codeVerifier := generateCodeVerifier()
	h.oauthStateService.StoreCodeVerifier(state, codeVerifier)
	codeChallenge := generateCodeChallenge(codeVerifier)

	// Twitter OAuth 2.0 Authorization URL
	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", cfg.TwitterClientID)
	params.Set("redirect_uri", cfg.TwitterRedirectURI)
	params.Set("scope", strings.Join([]string{
		"tweet.read",
		"tweet.write",
		"users.read",
		"offline.access",
	}, " "))
	params.Set("state", state)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")

	authURL := "https://twitter.com/i/oauth2/authorize?" + params.Encode()
	utils.Infof("twitter oauth initiate success user_id=%s", userID)

	utils.RespondWithJSON(w, http.StatusOK, map[string]string{
		"auth_url": authURL,
		"state":    state,
	})
}

// HandleTwitterCallback handles the OAuth callback from Twitter/X.
func (h *OAuthHandler) HandleTwitterCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errorParam := r.URL.Query().Get("error")

	utils.Infof("twitter callback received remote=%s has_code=%t has_state=%t has_error=%t",
		r.RemoteAddr, code != "", state != "", errorParam != "")

	if errorParam != "" {
		errorDesc := r.URL.Query().Get("error_description")
		utils.Warnf("twitter callback oauth error error=%s description=%s", errorParam, errorDesc)
		http.Redirect(w, r, fmt.Sprintf("/oauth/error?error=%s&description=%s",
			errorParam, url.QueryEscape(errorDesc)), http.StatusFound)
		return
	}

	if code == "" {
		utils.Warnf("twitter callback missing authorization code")
		utils.RespondWithError(w, http.StatusBadRequest, "Missing authorization code")
		return
	}

	if state == "" {
		utils.Warnf("twitter callback missing state parameter")
		utils.RespondWithError(w, http.StatusBadRequest, "Missing state parameter")
		return
	}

	oauthState, valid := h.oauthStateService.ValidateState(state)
	if !valid {
		utils.Warnf("twitter callback invalid or expired state")
		utils.RespondWithError(w, http.StatusBadRequest,
			"Invalid or expired state token. Please try connecting again.")
		return
	}

	if oauthState.Platform != "twitter" {
		utils.Warnf("twitter callback invalid platform in state platform=%s", oauthState.Platform)
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid state for Twitter OAuth")
		return
	}

	userID := oauthState.UserID

	// Retrieve code_verifier stored during initiation
	codeVerifier := h.oauthStateService.GetCodeVerifier(state)

	// Exchange authorization code for access token
	accessToken, refreshToken, expiresIn, twitterUserID, err := h.exchangeCodeForTwitterToken(code, codeVerifier)
	if err != nil {
		utils.Errorf("twitter token exchange failed user_id=%s err=%v", userID, err)
		http.Redirect(w, r, fmt.Sprintf("/oauth/error?error=token_exchange&description=%s",
			url.QueryEscape(err.Error())), http.StatusFound)
		return
	}
	utils.Infof("twitter token exchange success user_id=%s twitter_user_id=%s expires_in=%d", userID, twitterUserID, expiresIn)

	var expiresAt *time.Time
	if expiresIn > 0 {
		expTime := time.Now().Add(time.Duration(expiresIn) * time.Second)
		expiresAt = &expTime
	}

	cred := &models.PlatformCredentials{
		ID:             uuid.New().String(),
		UserID:         userID,
		Platform:       models.Twitter,
		AccessToken:    accessToken,
		RefreshToken:   refreshToken,
		TokenType:      "Bearer",
		ExpiresAt:      expiresAt,
		PlatformUserID: twitterUserID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := h.db.SaveCredentials(cred); err != nil {
		utils.Errorf("twitter save credentials failed user_id=%s twitter_user_id=%s err=%v", userID, twitterUserID, err)
		http.Redirect(w, r, "/oauth/error?error=save_failed&description=Failed+to+save+credentials",
			http.StatusFound)
		return
	}

	utils.Infof("twitter credentials saved user_id=%s platform=%s twitter_user_id=%s", userID, models.Twitter, twitterUserID)
	utils.Infof("twitter callback completed successfully user_id=%s", userID)

	http.Redirect(w, r, "/oauth/success?platform=twitter", http.StatusFound)
}

// exchangeCodeForTwitterToken exchanges the authorization code for an access token.
// Returns: accessToken, refreshToken, expiresIn, twitterUserID, error
func (h *OAuthHandler) exchangeCodeForTwitterToken(code, codeVerifier string) (string, string, int, string, error) {
	cfg := config.Load()
	utils.Debugf("twitter token exchange request start")

	tokenURL := "https://api.x.com/2/oauth2/token"

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", cfg.TwitterRedirectURI)
	form.Set("client_id", cfg.TwitterClientID)
	if codeVerifier != "" {
		form.Set("code_verifier", codeVerifier)
	}

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", 0, "", fmt.Errorf("failed to create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Twitter requires Basic auth with client_id:client_secret for confidential clients
	if cfg.TwitterClientSecret != "" {
		req.SetBasicAuth(cfg.TwitterClientID, cfg.TwitterClientSecret)
	}

	resp, err := twitterHTTPClient.Do(req)
	if err != nil {
		return "", "", 0, "", fmt.Errorf("twitter token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", 0, "", fmt.Errorf("failed to read token response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", 0, "", fmt.Errorf("twitter token exchange failed (status %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", "", 0, "", fmt.Errorf("failed to parse token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", "", 0, "", fmt.Errorf("twitter returned empty access token")
	}

	utils.Debugf("twitter token exchange success expires_in=%d", tokenResp.ExpiresIn)

	// Fetch the authenticated user's identity
	twitterUserID, err := h.getTwitterUserIdentity(tokenResp.AccessToken)
	if err != nil {
		utils.Warnf("twitter identity fetch failed (non-fatal): %v", err)
		// Don't fail the whole flow; we still have a valid token
		twitterUserID = ""
	}

	return tokenResp.AccessToken, tokenResp.RefreshToken, tokenResp.ExpiresIn, twitterUserID, nil
}

// getTwitterUserIdentity fetches the authenticated user's Twitter/X ID via GET /2/users/me.
func (h *OAuthHandler) getTwitterUserIdentity(accessToken string) (string, error) {
	utils.Debugf("twitter identity fetch start")

	req, err := http.NewRequest("GET", "https://api.x.com/2/users/me", nil)
	if err != nil {
		return "", fmt.Errorf("failed to create identity request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := twitterHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("twitter identity request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read identity response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("twitter identity API error (status %d): %s", resp.StatusCode, string(body))
	}

	var userResp struct {
		Data struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Username string `json:"username"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &userResp); err != nil {
		return "", fmt.Errorf("failed to parse identity response: %w", err)
	}

	if userResp.Data.ID == "" {
		return "", fmt.Errorf("twitter returned empty user ID")
	}

	utils.Debugf("twitter identity fetch success twitter_user_id=%s username=%s", userResp.Data.ID, userResp.Data.Username)
	return userResp.Data.ID, nil
}
