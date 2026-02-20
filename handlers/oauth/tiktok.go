package oauth

import (
	"SocialMediaAPI/config"
	"SocialMediaAPI/models"
	"SocialMediaAPI/utils"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

var tiktokHTTPClient = &http.Client{Timeout: 15 * time.Second}

// InitiateTikTokOAuth starts the TikTok OAuth flow.
// TikTok uses Login Kit v2 with PKCE (code_verifier / code_challenge).
func (h *OAuthHandler) InitiateTikTokOAuth(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value("userID").(string)
	if !ok || userID == "" {
		utils.Warnf("tiktok oauth initiate unauthorized: missing user id in context")
		utils.RespondWithError(w, http.StatusUnauthorized, "User ID not found in request context")
		return
	}

	cfg := config.Load()

	if cfg.TikTokClientKey == "" {
		utils.Errorf("tiktok oauth initiate config missing: TIKTOK_CLIENT_KEY")
		utils.RespondWithError(w, http.StatusInternalServerError,
			"TikTok Client Key not configured. Set TIKTOK_CLIENT_KEY environment variable")
		return
	}

	if cfg.TikTokRedirectURI == "" {
		utils.Errorf("tiktok oauth initiate config missing: TIKTOK_REDIRECT_URI")
		utils.RespondWithError(w, http.StatusInternalServerError,
			"TikTok Redirect URI not configured. Set TIKTOK_REDIRECT_URI environment variable")
		return
	}

	state := h.oauthStateService.GenerateState(userID, "tiktok")

	// Generate PKCE code_verifier (43-128 characters, URL-safe)
	codeVerifier := generateCodeVerifier()
	// Store code_verifier so we can send it during token exchange.
	h.oauthStateService.StoreCodeVerifier(state, codeVerifier)

	// Derive S256 code_challenge = BASE64URL(SHA256(ASCII(code_verifier)))
	codeChallenge := generateCodeChallenge(codeVerifier)

	// TikTok OAuth 2.0 Authorization URL
	params := url.Values{}
	params.Set("client_key", cfg.TikTokClientKey)
	params.Set("redirect_uri", cfg.TikTokRedirectURI)
	params.Set("response_type", "code")
	params.Set("scope", strings.Join([]string{
		"user.info.basic",
		"video.publish",
		"video.upload",
	}, ","))
	params.Set("state", state)
	params.Set("code_challenge", codeChallenge)
	params.Set("code_challenge_method", "S256")

	authURL := "https://www.tiktok.com/v2/auth/authorize/?" + params.Encode()
	utils.Infof("tiktok oauth initiate success user_id=%s", userID)

	utils.RespondWithJSON(w, http.StatusOK, map[string]string{
		"auth_url": authURL,
		"state":    state,
	})
}

// HandleTikTokCallback handles the OAuth callback from TikTok
func (h *OAuthHandler) HandleTikTokCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")
	errorParam := r.URL.Query().Get("error")

	utils.Infof("tiktok callback received remote=%s has_code=%t has_state=%t has_error=%t", r.RemoteAddr, code != "", state != "", errorParam != "")

	if errorParam != "" {
		errorDesc := r.URL.Query().Get("error_description")
		utils.Warnf("tiktok callback oauth error error=%s description=%s", errorParam, errorDesc)
		http.Redirect(w, r, fmt.Sprintf("/oauth/error?error=%s&description=%s",
			errorParam, url.QueryEscape(errorDesc)), http.StatusFound)
		return
	}

	if code == "" {
		utils.Warnf("tiktok callback missing authorization code")
		utils.RespondWithError(w, http.StatusBadRequest, "Missing authorization code")
		return
	}

	if state == "" {
		utils.Warnf("tiktok callback missing state parameter")
		utils.RespondWithError(w, http.StatusBadRequest, "Missing state parameter")
		return
	}

	oauthState, valid := h.oauthStateService.ValidateState(state)
	if !valid {
		utils.Warnf("tiktok callback invalid or expired state")
		utils.RespondWithError(w, http.StatusBadRequest,
			"Invalid or expired state token. Please try connecting again.")
		return
	}

	if oauthState.Platform != "tiktok" {
		utils.Warnf("tiktok callback invalid platform in state platform=%s", oauthState.Platform)
		utils.RespondWithError(w, http.StatusBadRequest, "Invalid state for TikTok OAuth")
		return
	}

	userID := oauthState.UserID

	// Retrieve code_verifier stored during initiation
	codeVerifier := h.oauthStateService.GetCodeVerifier(state)

	// Exchange authorization code for access token
	accessToken, refreshToken, expiresIn, openID, err := h.exchangeCodeForTikTokToken(code, codeVerifier)
	if err != nil {
		utils.Errorf("tiktok token exchange failed user_id=%s err=%v", userID, err)
		http.Redirect(w, r, fmt.Sprintf("/oauth/error?error=token_exchange&description=%s",
			url.QueryEscape(err.Error())), http.StatusFound)
		return
	}
	utils.Infof("tiktok token exchange success user_id=%s open_id=%s expires_in=%d", userID, openID, expiresIn)

	var expiresAt *time.Time
	if expiresIn > 0 {
		expTime := time.Now().Add(time.Duration(expiresIn) * time.Second)
		expiresAt = &expTime
	}

	cred := &models.PlatformCredentials{
		ID:             uuid.New().String(),
		UserID:         userID,
		Platform:       models.TikTok,
		AccessToken:    accessToken,
		RefreshToken:   refreshToken,
		TokenType:      "Bearer",
		ExpiresAt:      expiresAt,
		PlatformUserID: openID,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := h.db.SaveCredentials(cred); err != nil {
		utils.Errorf("tiktok save credentials failed user_id=%s open_id=%s err=%v", userID, openID, err)
		http.Redirect(w, r, "/oauth/error?error=save_failed&description=Failed+to+save+credentials",
			http.StatusFound)
		return
	}

	utils.Infof("tiktok credentials saved user_id=%s platform=%s open_id=%s", userID, models.TikTok, openID)
	utils.Infof("tiktok callback completed successfully user_id=%s", userID)

	http.Redirect(w, r, "/oauth/success?platform=tiktok", http.StatusFound)
}

// exchangeCodeForTikTokToken exchanges the auth code for an access token via TikTok's token endpoint.
// Returns: accessToken, refreshToken, expiresIn, openID, error
func (h *OAuthHandler) exchangeCodeForTikTokToken(code, codeVerifier string) (string, string, int, string, error) {
	cfg := config.Load()
	utils.Debugf("tiktok token exchange request start")

	tokenURL := "https://open.tiktokapis.com/v2/oauth/token/"

	form := url.Values{}
	form.Set("client_key", cfg.TikTokClientKey)
	form.Set("client_secret", cfg.TikTokClientSecret)
	form.Set("code", code)
	form.Set("grant_type", "authorization_code")
	form.Set("redirect_uri", cfg.TikTokRedirectURI)
	if codeVerifier != "" {
		form.Set("code_verifier", codeVerifier)
	}

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", 0, "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := tiktokHTTPClient.Do(req)
	if err != nil {
		utils.Errorf("tiktok token exchange http request failed err=%v", err)
		return "", "", 0, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		utils.Errorf("tiktok token exchange read body failed err=%v", err)
		return "", "", 0, "", err
	}

	if resp.StatusCode != http.StatusOK {
		utils.Errorf("tiktok token exchange api status=%d body=%s", resp.StatusCode, string(body))
		return "", "", 0, "", fmt.Errorf("TikTok token API error (status %d): %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		ExpiresIn    int    `json:"expires_in"`
		OpenID       string `json:"open_id"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
		TokenType    string `json:"token_type"`
	}

	if err := json.Unmarshal(body, &tokenResp); err != nil {
		utils.Errorf("tiktok token exchange parse response failed err=%v", err)
		return "", "", 0, "", err
	}

	if tokenResp.AccessToken == "" {
		utils.Errorf("tiktok token exchange returned empty access token")
		return "", "", 0, "", fmt.Errorf("TikTok token API returned empty access token")
	}

	utils.Debugf("tiktok token exchange request success expires_in=%d open_id=%s", tokenResp.ExpiresIn, tokenResp.OpenID)
	return tokenResp.AccessToken, tokenResp.RefreshToken, tokenResp.ExpiresIn, tokenResp.OpenID, nil
}

// generateCodeVerifier generates a random code verifier for PKCE (43-128 chars).
func generateCodeVerifier() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// generateCodeChallenge derives the S256 code challenge from a code verifier:
// code_challenge = BASE64URL(SHA256(ASCII(code_verifier)))
func generateCodeChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}
