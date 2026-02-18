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
	"strings"
	"time"

	"github.com/google/uuid"
)

// InitiateInstagramOAuth starts the Instagram OAuth flow
func (h *Handler) InitiateInstagramOAuth(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		utils.RespondWithError(w, http.StatusUnauthorized, "User ID not found in request context")
		return
	}

	state := h.oauthStateService.GenerateState(userID, "instagram")
	cfg := config.Load()

	if cfg.InstagramAppID == "" {
		utils.RespondWithError(w, http.StatusInternalServerError,
			"Instagram App ID not configured. Set INSTAGRAM_APP_ID environment variable")
		return
	}

	if cfg.InstagramRedirectURI == "" {
		utils.RespondWithError(w, http.StatusInternalServerError,
			"Instagram Redirect URI not configured. Set INSTAGRAM_REDIRECT_URI environment variable")
		return
	}

	authURL := fmt.Sprintf(
		"https://www.facebook.com/%s/dialog/oauth?client_id=%s&redirect_uri=%s&state=%s&scope=%s",
		cfg.FacebookVersion,
		cfg.InstagramAppID,
		url.QueryEscape(cfg.InstagramRedirectURI),
		state,
		url.QueryEscape("instagram_basic,instagram_content_publish,pages_show_list,business_management"),
	)

	utils.RespondWithJSON(w, http.StatusOK, map[string]string{
		"auth_url": authURL,
		"state":    state,
	})
}

// HandleInstagramCallback handles the OAuth callback from Instagram (Meta)
func (h *Handler) HandleInstagramCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errorParam := r.URL.Query().Get("error")

	if errorParam != "" {
		errorDesc := r.URL.Query().Get("error_description")
		http.Redirect(w, r, fmt.Sprintf("/oauth/error?error=%s&description=%s",
			errorParam, url.QueryEscape(errorDesc)), http.StatusFound)
		return
	}

	if code == "" {
		utils.RespondWithError(w, http.StatusBadRequest, "Missing authorization code")
		return
	}

	if state == "" {
		utils.RespondWithError(w, http.StatusBadRequest, "Missing state parameter")
		return
	}

	oauthState, valid := h.oauthStateService.ValidateState(state)
	if !valid {
		utils.RespondWithError(w, http.StatusBadRequest,
			"Invalid or expired state token. Please try connecting again.")
		return
	}

	if oauthState.Platform != "instagram" {
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid state for Instagram OAuth")
		return
	}

	userID := oauthState.UserID

	shortToken, _, err := h.exchangeCodeForInstagramToken(code)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/oauth/error?error=token_exchange&description=%s",
			url.QueryEscape(err.Error())), http.StatusFound)
		return
	}

	longLivedToken, expiresIn, err := h.exchangeInstagramLongLivedToken(shortToken)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/oauth/error?error=long_lived_exchange&description=%s",
			url.QueryEscape(err.Error())), http.StatusFound)
		return
	}

	instagramUserID, pageID, err := h.getInstagramBusinessIdentity(longLivedToken)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/oauth/error?error=identity_fetch&description=%s",
			url.QueryEscape(err.Error())), http.StatusFound)
		return
	}

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
		http.Redirect(w, r, "/oauth/error?error=save_failed&description=Failed+to+save+credentials",
			http.StatusFound)
		return
	}

	http.Redirect(w, r, "/oauth/success?platform=instagram", http.StatusFound)
}

func (h *Handler) exchangeCodeForInstagramToken(code string) (string, int, error) {
	cfg := config.Load()

	tokenURL := fmt.Sprintf(
		"https://graph.facebook.com/%s/oauth/access_token?client_id=%s&client_secret=%s&redirect_uri=%s&code=%s",
		cfg.FacebookVersion,
		cfg.InstagramAppID,
		cfg.InstagramAppSecret,
		url.QueryEscape(cfg.InstagramRedirectURI),
		code,
	)

	resp, err := facebookHTTPClient.Get(tokenURL)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, err
	}

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("Instagram token API error: %s", string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", 0, err
	}

	if tokenResp.AccessToken == "" {
		return "", 0, fmt.Errorf("Instagram token API returned empty access token")
	}

	return tokenResp.AccessToken, tokenResp.ExpiresIn, nil
}

func (h *Handler) exchangeInstagramLongLivedToken(shortToken string) (string, int, error) {
	cfg := config.Load()

	exchangeURL := fmt.Sprintf(
		"https://graph.facebook.com/%s/oauth/access_token?grant_type=fb_exchange_token&client_id=%s&client_secret=%s&fb_exchange_token=%s",
		cfg.FacebookVersion,
		cfg.InstagramAppID,
		cfg.InstagramAppSecret,
		url.QueryEscape(shortToken),
	)

	resp, err := facebookHTTPClient.Get(exchangeURL)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, err
	}

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("long-lived token exchange failed: %s", string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", 0, err
	}

	if tokenResp.AccessToken == "" {
		return "", 0, fmt.Errorf("long-lived token exchange returned empty token")
	}

	return tokenResp.AccessToken, tokenResp.ExpiresIn, nil
}

// getInstagramBusinessIdentity fetches the first Instagram Business Account linked to user's pages.
func (h *Handler) getInstagramBusinessIdentity(accessToken string) (string, string, error) {
	cfg := config.Load()
	pagesURL := fmt.Sprintf(
		"https://graph.facebook.com/%s/me/accounts?fields=id,name,instagram_business_account{id,username}&access_token=%s",
		cfg.FacebookVersion,
		url.QueryEscape(accessToken),
	)

	resp, err := facebookHTTPClient.Get(pagesURL)
	if err != nil {
		return "", "", fmt.Errorf("failed to fetch Facebook pages for Instagram binding: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to read pages response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("Meta pages API error: %s", string(body))
	}

	var pagesResp struct {
		Data []struct {
			ID                       string `json:"id"`
			InstagramBusinessAccount *struct {
				ID       string `json:"id"`
				Username string `json:"username"`
			} `json:"instagram_business_account"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &pagesResp); err != nil {
		return "", "", fmt.Errorf("failed to parse pages response: %w", err)
	}

	for _, page := range pagesResp.Data {
		if page.InstagramBusinessAccount != nil && page.InstagramBusinessAccount.ID != "" {
			return page.InstagramBusinessAccount.ID, page.ID, nil
		}
	}

	return "", "", fmt.Errorf("no Instagram Business account found. Ensure your Instagram account is Professional (Business/Creator), linked to a Facebook Page, and app permissions were approved")
}

func sanitizeMetaError(errMsg string) string {
	msg := strings.TrimSpace(errMsg)
	if msg == "" {
		return "Unknown error"
	}
	return msg
}
