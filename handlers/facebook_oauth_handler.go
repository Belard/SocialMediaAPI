package handlers

import (
	"SocialMediaAPI/config"
	"SocialMediaAPI/models"
	"SocialMediaAPI/utils"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
)

var facebookHTTPClient = &http.Client{Timeout: 10 * time.Second}

// InitiateFacebookOAuth starts the Facebook OAuth flow
func (h *Handler) InitiateFacebookOAuth(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user ID from JWT (safe type assertion)
	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		utils.Warnf("facebook oauth initiate unauthorized: missing user id in context")
		utils.RespondWithError(w, http.StatusUnauthorized, "User ID not found in request context")
		return
	}

	// Generate secure state token that includes userID
	state := h.oauthStateService.GenerateState(userID, "facebook")

	cfg := config.Load()
	
	if cfg.FacebookAppID == "" {
		utils.Errorf("facebook oauth initiate config missing: FACEBOOK_APP_ID")
		utils.RespondWithError(w, http.StatusInternalServerError, 
			"Facebook App ID not configured. Set FACEBOOK_APP_ID environment variable")
		return
	}

	authURL := fmt.Sprintf(
		"https://www.facebook.com/%s/dialog/oauth?client_id=%s&redirect_uri=%s&state=%s&scope=pages_show_list,pages_manage_posts",
		cfg.FacebookVersion,
		cfg.FacebookAppID,
		url.QueryEscape(cfg.FacebookRedirectURI),
		state,
	)

	utils.Infof("facebook oauth initiate success user_id=%s", userID)

	utils.RespondWithJSON(w, http.StatusOK, map[string]string{
		"auth_url": authURL,
		"state":    state,
	})
}

// HandleFacebookCallback handles the OAuth callback from Facebook
func (h *Handler) HandleFacebookCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errorParam := r.URL.Query().Get("error")

	utils.Infof("received callback remote=%s has_code=%t has_state=%t has_error=%t", r.RemoteAddr, code != "", state != "", errorParam != "")

	// Check if user denied access
	if errorParam != "" {
		errorDesc := r.URL.Query().Get("error_description")
		utils.Warnf("user denied or OAuth error error=%s description=%s", errorParam, errorDesc)
		http.Redirect(w, r, fmt.Sprintf("/oauth/error?error=%s&description=%s", 
			errorParam, url.QueryEscape(errorDesc)), http.StatusFound)
		return
	}

	if code == "" {
		utils.Warnf("missing authorization code")
		utils.RespondWithError(w, http.StatusBadRequest, "Missing authorization code")
		return
	}

	if state == "" {
		utils.Warnf("missing state parameter")
		utils.RespondWithError(w, http.StatusBadRequest, "Missing state parameter")
		return
	}

	// Validate state and get userID (CSRF protection)
	oauthState, valid := h.oauthStateService.ValidateState(state)
	if !valid {
		utils.Warnf("invalid or expired state")
		utils.RespondWithError(w, http.StatusBadRequest, 
			"Invalid or expired state token. Please try connecting again.")
		return
	}

	// Now we have the userID from the validated state!
	userID := oauthState.UserID

	// Exchange code for access token
	accessToken, expiresIn, err := h.exchangeCodeForFacebookToken(code)
	if err != nil {
		utils.Errorf("token exchange failed user_id=%s err=%v", userID, err)
		http.Redirect(w, r, fmt.Sprintf("/oauth/error?error=token_exchange&description=%s", 
			url.QueryEscape(err.Error())), http.StatusFound)
		return
	}
	utils.Infof("token exchange success user_id=%s expires_in=%d", userID, expiresIn)

	// Fetch Facebook user ID and page info (bind token to identity)
	facebookUserID, pageID, err := h.getFacebookUserIdentity(accessToken)
	if err != nil {
		utils.Errorf("identity fetch failed user_id=%s err=%v", userID, err)
		http.Redirect(w, r, fmt.Sprintf("/oauth/error?error=identity_fetch&description=%s", 
			url.QueryEscape(err.Error())), http.StatusFound)
		return
	}
	utils.Infof("identity fetch success user_id=%s facebook_user_id=%s page_id=%s", userID, facebookUserID, pageID)

	// Calculate expiration time
	var expiresAt *time.Time
	if expiresIn > 0 {
		expTime := time.Now().Add(time.Duration(expiresIn) * time.Second)
		expiresAt = &expTime
	}

	// Save credentials to database with identity binding
	cred := &models.PlatformCredentials{
		ID:             uuid.New().String(),
		UserID:         userID,
		Platform:       models.Facebook,
		AccessToken:    accessToken,
		TokenType:      "Bearer",
		ExpiresAt:      expiresAt,
		PlatformUserID: facebookUserID,
		PlatformPageID: pageID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := h.db.SaveCredentials(cred); err != nil {
		utils.Errorf("failed to save credentials user_id=%s facebook_user_id=%s page_id=%s err=%v", userID, facebookUserID, pageID, err)
		http.Redirect(w, r, "/oauth/error?error=save_failed&description=Failed+to+save+credentials", 
			http.StatusFound)
		return
	}
	utils.Infof("credentials saved user_id=%s platform=%s facebook_user_id=%s page_id=%s", userID, models.Facebook, facebookUserID, pageID)

	// Success! Redirect to success page
	utils.Infof("completed successfully user_id=%s", userID)
	http.Redirect(w, r, "/oauth/success?platform=facebook", http.StatusFound)
}

func (h *Handler) exchangeCodeForFacebookToken(code string) (string, int, error) {
	cfg := config.Load()
	utils.Debugf("facebook token exchange request start")

	tokenURL := fmt.Sprintf(
		"https://graph.facebook.com/%s/oauth/access_token?client_id=%s&client_secret=%s&redirect_uri=%s&code=%s",
		cfg.FacebookVersion,
		cfg.FacebookAppID,
		cfg.FacebookAppSecret,
		url.QueryEscape(cfg.FacebookRedirectURI),
		code,
	)

	resp, err := facebookHTTPClient.Get(tokenURL)
	if err != nil {
		utils.Errorf("facebook token exchange http request failed err=%v", err)
		return "", 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		utils.Errorf("facebook token exchange read body failed err=%v", err)
		return "", 0, err
	}

	if resp.StatusCode != http.StatusOK {
		utils.Errorf("facebook token exchange api status=%d", resp.StatusCode)
		return "", 0, fmt.Errorf("Facebook API error: %s", string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		utils.Errorf("facebook token exchange parse response failed err=%v", err)
		return "", 0, err
	}

	if tokenResp.AccessToken == "" {
		utils.Errorf("facebook token exchange returned empty access token")
		return "", 0, fmt.Errorf("Facebook token API returned empty access token")
	}

	utils.Debugf("facebook token exchange request success expires_in=%d", tokenResp.ExpiresIn)

	return tokenResp.AccessToken, tokenResp.ExpiresIn, nil
}

// getFacebookUserIdentity fetches the Facebook user ID and primary page ID
// This binds the token to a specific Facebook identity
func (h *Handler) getFacebookUserIdentity(accessToken string) (string, string, error) {
	cfg := config.Load()
	utils.Debugf("facebook identity fetch start")

	// Get the authenticated user's ID
	userURL := fmt.Sprintf("https://graph.facebook.com/%s/me?access_token=%s", cfg.FacebookVersion, accessToken)
	
	resp, err := facebookHTTPClient.Get(userURL)
	if err != nil {
		utils.Errorf("facebook identity fetch user info request failed err=%v", err)
		return "", "", fmt.Errorf("failed to fetch Facebook user info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		utils.Errorf("facebook identity fetch user info api status=%d", resp.StatusCode)
		return "", "", fmt.Errorf("Facebook API error: %s", string(body))
	}

	bodyData, err := io.ReadAll(resp.Body)
	if err != nil {
		utils.Errorf("facebook identity fetch user info read body failed err=%v", err)
		return "", "", fmt.Errorf("failed to read Facebook user response: %w", err)
	}
	var userResp struct {
		ID string `json:"id"`
	}

	if err := json.Unmarshal(bodyData, &userResp); err != nil {
		utils.Errorf("facebook identity fetch user info parse response failed err=%v", err)
		return "", "", fmt.Errorf("failed to parse Facebook user response: %w", err)
	}

	facebookUserID := userResp.ID

	// Get the user's pages (fetch first page as primary)
	pagesURL := fmt.Sprintf("https://graph.facebook.com/%s/me/accounts?access_token=%s", cfg.FacebookVersion, accessToken)
	
	resp, err = facebookHTTPClient.Get(pagesURL)
	if err != nil {
		utils.Errorf("facebook identity fetch pages request failed user_id=%s err=%v", facebookUserID, err)
		return facebookUserID, "", fmt.Errorf("failed to fetch Facebook pages: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		utils.Errorf("facebook identity fetch pages api status=%d user_id=%s", resp.StatusCode, facebookUserID)
		return facebookUserID, "", fmt.Errorf("Facebook pages API error: %s", string(body))
	}

	var pagesResp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	bodyData, err = io.ReadAll(resp.Body)
	if err != nil {
		utils.Errorf("facebook identity fetch pages read body failed user_id=%s err=%v", facebookUserID, err)
		return facebookUserID, "", fmt.Errorf("failed to read Facebook pages response: %w", err)
	}
	if err := json.Unmarshal(bodyData, &pagesResp); err != nil {
		utils.Errorf("facebook identity fetch pages parse response failed user_id=%s err=%v", facebookUserID, err)
		return facebookUserID, "", fmt.Errorf("failed to parse Facebook pages response: %w", err)
	}

	pageID := ""
	if len(pagesResp.Data) > 0 {
		pageID = pagesResp.Data[0].ID
	}

	utils.Debugf("facebook identity fetch success user_id=%s page_id=%s", facebookUserID, pageID)

	return facebookUserID, pageID, nil
}
