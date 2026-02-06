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

// InitiateFacebookOAuth starts the Facebook OAuth flow
func (h *Handler) InitiateFacebookOAuth(w http.ResponseWriter, r *http.Request) {
	// Get authenticated user ID from JWT
	userID := r.Context().Value("userID").(string)

	// Generate secure state token that includes userID
	state := h.oauthStateService.GenerateState(userID, "facebook")

	cfg := config.Load()
	
	if cfg.FacebookAppID == "" {
		utils.RespondWithError(w, http.StatusInternalServerError, 
			"Facebook App ID not configured. Set FACEBOOK_APP_ID environment variable")
		return
	}

	authURL := fmt.Sprintf(
		"https://www.facebook.com/%s/dialog/oauth?client_id=%s&redirect_uri=%s&state=%s&scope=pages_show_list,pages_manage_posts,pages_read_engagement,pages_read_user_content",
		cfg.FacebookVersion,
		cfg.FacebookAppID,
		url.QueryEscape(cfg.FacebookRedirectURI),
		state,
	)

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

	// Check if user denied access
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

	// Validate state and get userID (CSRF protection)
	oauthState, valid := h.oauthStateService.ValidateState(state)
	if !valid {
		utils.RespondWithError(w, http.StatusBadRequest, 
			"Invalid or expired state token. Please try connecting again.")
		return
	}

	// Now we have the userID from the validated state!
	userID := oauthState.UserID

	// Exchange code for access token
	accessToken, expiresIn, err := h.exchangeCodeForFacebookToken(code)
	if err != nil {
		http.Redirect(w, r, fmt.Sprintf("/oauth/error?error=token_exchange&description=%s", 
			url.QueryEscape(err.Error())), http.StatusFound)
		return
	}

	// Calculate expiration time
	var expiresAt *time.Time
	if expiresIn > 0 {
		expTime := time.Now().Add(time.Duration(expiresIn) * time.Second)
		expiresAt = &expTime
	}

	// Save credentials to database
	cred := &models.PlatformCredentials{
		ID:          uuid.New().String(),
		UserID:      userID,
		Platform:    models.Facebook,
		AccessToken: accessToken,
		TokenType:   "Bearer",
		ExpiresAt:   expiresAt,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	if err := h.db.SaveCredentials(cred); err != nil {
		http.Redirect(w, r, "/oauth/error?error=save_failed&description=Failed+to+save+credentials", 
			http.StatusFound)
		return
	}

	// Success! Redirect to success page
	http.Redirect(w, r, "/oauth/success?platform=facebook", http.StatusFound)
}

func (h *Handler) exchangeCodeForFacebookToken(code string) (string, int, error) {
	cfg := config.Load()

	tokenURL := fmt.Sprintf(
		"https://graph.facebook.com/%s/oauth/access_token?client_id=%s&client_secret=%s&redirect_uri=%s&code=%s",
		cfg.FacebookVersion,
		cfg.FacebookAppID,
		cfg.FacebookAppSecret,
		url.QueryEscape(cfg.FacebookRedirectURI),
		code,
	)

	resp, err := http.Get(tokenURL)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("Facebook API error: %s", string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", 0, err
	}

	return tokenResp.AccessToken, tokenResp.ExpiresIn, nil
}
