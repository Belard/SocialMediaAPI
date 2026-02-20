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

var instagramHTTPClient = &http.Client{Timeout: 10 * time.Second}

// InitiateInstagramOAuth starts the Instagram OAuth flow
func (h *OAuthHandler) InitiateInstagramOAuth(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		utils.Warnf("instagram oauth initiate unauthorized: missing user id in context")
		utils.RespondWithError(w, http.StatusUnauthorized, "User ID not found in request context")
		return
	}

	state := h.oauthStateService.GenerateState(userID, "instagram")
	cfg := config.Load()

	if cfg.InstagramAppID == "" {
		utils.Errorf("instagram oauth initiate config missing: INSTAGRAM_APP_ID")
		utils.RespondWithError(w, http.StatusInternalServerError,
			"Instagram App ID not configured. Set INSTAGRAM_APP_ID environment variable")
		return
	}

	if cfg.InstagramRedirectURI == "" {
		utils.Errorf("instagram oauth initiate config missing: INSTAGRAM_REDIRECT_URI")
		utils.RespondWithError(w, http.StatusInternalServerError,
			"Instagram Redirect URI not configured. Set INSTAGRAM_REDIRECT_URI environment variable")
		return
	}

	params := url.Values{}
	params.Set("client_id", cfg.InstagramAppID)
	params.Set("redirect_uri", cfg.InstagramRedirectURI)
	params.Set("response_type", "code")
	params.Set("scope", strings.Join([]string{
		"instagram_business_basic",
		"instagram_business_manage_messages",
		"instagram_business_manage_comments",
		"instagram_business_content_publish",
	}, ","))
	params.Set("state", state)
	params.Set("enable_fb_login", "true")

	if forceReauth := r.URL.Query().Get("force_reauth"); forceReauth == "true" || forceReauth == "false" {
		params.Set("force_reauth", forceReauth)
	}

	authURL := "https://www.instagram.com/oauth/authorize?" + params.Encode()
	utils.Infof("instagram oauth initiate success user_id=%s has_force_reauth=%t", userID, r.URL.Query().Has("force_reauth"))

	utils.RespondWithJSON(w, http.StatusOK, map[string]string{
		"auth_url": authURL,
		"state":    state,
	})
}

// HandleInstagramCallback handles the OAuth callback from Instagram (Meta)
func (h *OAuthHandler) HandleInstagramCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errorParam := r.URL.Query().Get("error")

	utils.Infof("instagram callback received remote=%s has_code=%t has_state=%t has_error=%t", r.RemoteAddr, code != "", state != "", errorParam != "")

	if errorParam != "" {
		errorDesc := r.URL.Query().Get("error_description")
		utils.Warnf("instagram callback oauth error error=%s description=%s", errorParam, sanitizeMetaError(errorDesc))
		http.Redirect(w, r, fmt.Sprintf("/oauth/error?error=%s&description=%s",
			errorParam, url.QueryEscape(errorDesc)), http.StatusFound)
		return
	}

	if code == "" {
		utils.Warnf("instagram callback missing authorization code")
		utils.RespondWithError(w, http.StatusBadRequest, "Missing authorization code")
		return
	}

	if state == "" {
		utils.Warnf("instagram callback missing state parameter")
		utils.RespondWithError(w, http.StatusBadRequest, "Missing state parameter")
		return
	}

	oauthState, valid := h.oauthStateService.ValidateState(state)
	if !valid {
		utils.Warnf("instagram callback invalid or expired state")
		utils.RespondWithError(w, http.StatusBadRequest,
			"Invalid or expired state token. Please try connecting again.")
		return
	}

	if oauthState.Platform != "instagram" {
		utils.Warnf("instagram callback invalid platform in state platform=%s", oauthState.Platform)
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid state for Instagram OAuth")
		return
	}

	userID := oauthState.UserID

	shortToken, _, err := h.exchangeCodeForInstagramToken(strings.TrimSuffix(code, "#_"))
	if err != nil {
		utils.Errorf("instagram token exchange failed user_id=%s err=%v", userID, err)
		http.Redirect(w, r, fmt.Sprintf("/oauth/error?error=token_exchange&description=%s",
			url.QueryEscape(err.Error())), http.StatusFound)
		return
	}
	utils.Infof("instagram token exchange success user_id=%s", userID)

	longLivedToken, expiresIn, err := h.exchangeInstagramLongLivedToken(shortToken)
	if err != nil {
		utils.Errorf("instagram long-lived token exchange failed user_id=%s err=%v", userID, err)
		http.Redirect(w, r, fmt.Sprintf("/oauth/error?error=long_lived_exchange&description=%s",
			url.QueryEscape(err.Error())), http.StatusFound)
		return
	}
	utils.Infof("instagram long-lived token exchange success user_id=%s expires_in=%d", userID, expiresIn)

	instagramUserID, pageID, err := h.getInstagramBusinessIdentity(longLivedToken)
	if err != nil {
		utils.Errorf("instagram identity fetch failed user_id=%s err=%v", userID, err)
		http.Redirect(w, r, fmt.Sprintf("/oauth/error?error=identity_fetch&description=%s",
			url.QueryEscape(err.Error())), http.StatusFound)
		return
	}
	utils.Infof("instagram identity fetch success user_id=%s instagram_user_id=%s page_id=%s", userID, instagramUserID, pageID)

	var expiresAt *time.Time
	if expiresIn > 0 {
		expTime := time.Now().Add(time.Duration(expiresIn) * time.Second)
		expiresAt = &expTime
	}

	cred := &models.PlatformCredentials{
		ID:             uuid.New().String(),
		UserID:         userID,
		Platform:       models.Instagram,
		AccessToken:    longLivedToken,
		TokenType:      "Bearer",
		ExpiresAt:      expiresAt,
		PlatformUserID: instagramUserID,
		PlatformPageID: pageID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := h.db.SaveCredentials(cred); err != nil {
		utils.Errorf("instagram save credentials failed user_id=%s instagram_user_id=%s page_id=%s err=%v", userID, instagramUserID, pageID, err)
		http.Redirect(w, r, "/oauth/error?error=save_failed&description=Failed+to+save+credentials",
			http.StatusFound)
		return
	}

	utils.Infof("instagram credentials saved user_id=%s platform=%s instagram_user_id=%s page_id=%s", userID, models.Instagram, instagramUserID, pageID)
	utils.Infof("instagram callback completed successfully user_id=%s", userID)

	http.Redirect(w, r, "/oauth/success?platform=instagram", http.StatusFound)
}

func (h *OAuthHandler) exchangeCodeForInstagramToken(code string) (string, int, error) {
	cfg := config.Load()
	utils.Debugf("instagram token exchange request start")

	// Instagram Business Login uses api.instagram.com (not graph.instagram.com)
	tokenURL := "https://api.instagram.com/oauth/access_token"

	form := url.Values{}
	form.Set("client_id", cfg.InstagramAppID)
	form.Set("client_secret", cfg.InstagramAppSecret)
	form.Set("grant_type", "authorization_code")
	form.Set("redirect_uri", cfg.InstagramRedirectURI)
	form.Set("code", code)

	resp, err := instagramHTTPClient.PostForm(tokenURL, form)
	if err != nil {
		utils.Errorf("instagram token exchange http request failed err=%v", err)
		return "", 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		utils.Errorf("instagram token exchange read body failed err=%v", err)
		return "", 0, err
	}

	if resp.StatusCode != http.StatusOK {
		utils.Errorf("instagram token exchange api status=%d", resp.StatusCode)
		return "", 0, fmt.Errorf("Instagram token API error: %s", string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		utils.Errorf("instagram token exchange parse response failed err=%v", err)
		return "", 0, err
	}

	if tokenResp.AccessToken == "" {
		utils.Errorf("instagram token exchange returned empty access token")
		return "", 0, fmt.Errorf("Instagram token API returned empty access token")
	}

	utils.Debugf("instagram token exchange request success expires_in=%d", tokenResp.ExpiresIn)
	return tokenResp.AccessToken, tokenResp.ExpiresIn, nil
}

func (h *OAuthHandler) exchangeInstagramLongLivedToken(shortToken string) (string, int, error) {
	cfg := config.Load()
	utils.Debugf("instagram long-lived token exchange request start")

	// Instagram Business Login uses ig_exchange_token (not fb_exchange_token), no version prefix
	exchangeURL := fmt.Sprintf(
		"https://graph.instagram.com/access_token?grant_type=ig_exchange_token&client_secret=%s&access_token=%s",
		cfg.InstagramAppSecret,
		url.QueryEscape(shortToken),
	)

	resp, err := instagramHTTPClient.Get(exchangeURL)
	if err != nil {
		utils.Errorf("instagram long-lived exchange http request failed err=%v", err)
		return "", 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		utils.Errorf("instagram long-lived exchange read body failed err=%v", err)
		return "", 0, err
	}

	if resp.StatusCode != http.StatusOK {
		utils.Errorf("instagram long-lived exchange api status=%d", resp.StatusCode)
		return "", 0, fmt.Errorf("long-lived token exchange failed: %s", string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		utils.Errorf("instagram long-lived exchange parse response failed err=%v", err)
		return "", 0, err
	}

	if tokenResp.AccessToken == "" {
		utils.Errorf("instagram long-lived exchange returned empty token")
		return "", 0, fmt.Errorf("long-lived token exchange returned empty token")
	}

	utils.Debugf("instagram long-lived token exchange request success expires_in=%d", tokenResp.ExpiresIn)
	return tokenResp.AccessToken, tokenResp.ExpiresIn, nil
}

// getInstagramBusinessIdentity fetches the Instagram user ID via the Instagram Business Login /me endpoint.
func (h *OAuthHandler) getInstagramBusinessIdentity(accessToken string) (string, string, error) {
	cfg := config.Load()
	utils.Debugf("instagram business identity fetch start")

	// Instagram Business Login: /me returns user_id and username directly
	meURL := fmt.Sprintf(
		"https://graph.instagram.com/%s/me?fields=user_id,username&access_token=%s",
		cfg.InstagramVersion,
		url.QueryEscape(accessToken),
	)

	resp, err := instagramHTTPClient.Get(meURL)
	if err != nil {
		utils.Errorf("instagram business identity http request failed err=%v", err)
		return "", "", fmt.Errorf("failed to fetch Instagram identity: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		utils.Errorf("instagram business identity read body failed err=%v", err)
		return "", "", fmt.Errorf("failed to read identity response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		utils.Errorf("instagram business identity api status=%d", resp.StatusCode)
		return "", "", fmt.Errorf("Instagram identity API error: %s", string(body))
	}

	var meResp struct {
		UserID   string `json:"user_id"`
		Username string `json:"username"`
		ID       string `json:"id"`
	}

	if err := json.Unmarshal(body, &meResp); err != nil {
		utils.Errorf("instagram business identity parse response failed err=%v", err)
		return "", "", fmt.Errorf("failed to parse identity response: %w", err)
	}

	// user_id is the Instagram-scoped user ID needed for the Content Publishing API
	instagramUserID := meResp.UserID
	if instagramUserID == "" {
		instagramUserID = meResp.ID
	}

	if instagramUserID == "" {
		utils.Warnf("instagram business identity returned empty user_id")
		return "", "", fmt.Errorf("Instagram identity API returned empty user ID")
	}

	utils.Debugf("instagram business identity found user_id=%s username=%s", instagramUserID, meResp.Username)
	return instagramUserID, "", nil
}

func sanitizeMetaError(errMsg string) string {
	msg := strings.TrimSpace(errMsg)
	if msg == "" {
		return "Unknown error"
	}
	return msg
}
