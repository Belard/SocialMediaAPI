package publishers

import (
	"SocialMediaAPI/config"
	"SocialMediaAPI/models"
	"SocialMediaAPI/utils"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type FacebookPublisher struct{
	client *http.Client
}

type FacebookPageResponse struct {
	Data []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		AccessToken string `json:"access_token"`
	} `json:"data"`
}

type FacebookPostResponse struct {
	ID string `json:"id"`
}

type FacebookPhotoResponse struct {
	ID string `json:"id"`
}

type FacebookErrorResponse struct {
	Error struct {
		Message   string `json:"message"`
		Type      string `json:"type"`
		Code      int    `json:"code"`
		FBTraceID string `json:"fbtrace_id"`
	} `json:"error"`
}

func (f *FacebookPublisher) Publish(post *models.Post, cred *models.PlatformCredentials) models.PublishResult {
	if cred == nil || cred.AccessToken == "" {
		return models.PublishResult{
			Platform: models.Facebook,
			Success:  false,
			Message:  "Missing Facebook credentials",
		}
	}

	// Check if token is expired
	tokenValidator := utils.NewTokenValidator()
	if tokenValidator.IsTokenExpired(cred) {
		// Attempt to refresh the token
		if err := tokenValidator.RefreshFacebookToken(cred); err != nil {
			return models.PublishResult{
				Platform: models.Facebook,
				Success:  false,
				Message:  fmt.Sprintf("Facebook token has expired and cannot be refreshed: %v", err),
			}
		}
	}

	// Get Page Access Token first
	pageAccessToken, pageID, err := f.getPageAccessToken(cred.AccessToken)
	if err != nil {
		return models.PublishResult{
			Platform: models.Facebook,
			Success:  false,
			Message:  fmt.Sprintf("Error getting page access token: %v", err),
		}
	}

	// Publish based on media presence
	var postID string
	if len(post.Media) > 0 {
		// Post with media
		postID, err = f.publishWithMedia(post, pageAccessToken, pageID)
	} else {
		// Text-only post
		postID, err = f.publishTextOnly(post, pageAccessToken, pageID)
	}

	if err != nil {
		return models.PublishResult{
			Platform: models.Facebook,
			Success:  false,
			Message:  fmt.Sprintf("Error publishing to Facebook: %v", err),
		}
	}

	return models.PublishResult{
		Platform: models.Facebook,
		Success:  true,
		Message:  "Published successfully on Facebook",
		PostID:   postID,
	}
}

// NewFacebookPublisher creates a FacebookPublisher with an injectable http.Client.
// If nil is passed, a default client with a sensible timeout is used.
func NewFacebookPublisher(client *http.Client) *FacebookPublisher {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &FacebookPublisher{client: client}
}

func (f *FacebookPublisher) httpClient() *http.Client {
	if f.client == nil {
		f.client = &http.Client{Timeout: 30 * time.Second}
	}
	return f.client
}

func (f *FacebookPublisher) getPageAccessToken(userAccessToken string) (string, string, error) {
	cfg := config.Load()
	url := fmt.Sprintf("https://graph.facebook.com/%s/me/accounts", cfg.FacebookVersion)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+userAccessToken)
	resp, err := f.httpClient().Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var fbError FacebookErrorResponse
		json.Unmarshal(body, &fbError)
		
		// Check for token expiration error
		tokenValidator := utils.NewTokenValidator()
		if tokenValidator.IsFacebookTokenExpiredError(body) {
			return "", "", fmt.Errorf("access token has expired (error code: %d)", fbError.Error.Code)
		}
		
		return "", "", fmt.Errorf("Facebook API error: %s (code: %d)", fbError.Error.Message, fbError.Error.Code)
	}

	var pageResp FacebookPageResponse
	if err := json.Unmarshal(body, &pageResp); err != nil {
		return "", "", err
	}

	if len(pageResp.Data) == 0 {
		return "", "", fmt.Errorf("no Facebook pages found for this account")
	}

	// Use the first page
	page := pageResp.Data[0]
	return page.AccessToken, page.ID, nil
}

func (f *FacebookPublisher) publishTextOnly(post *models.Post, pageAccessToken, pageID string) (string, error) {
	cfg := config.Load()
	url := fmt.Sprintf("https://graph.facebook.com/%s/%s/feed", cfg.FacebookVersion, pageID)

	payload := map[string]string{
		"message":      post.Content,
	}

	jsonData, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+pageAccessToken)
	resp, err := f.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var fbError FacebookErrorResponse
		json.Unmarshal(body, &fbError)
		return "", fmt.Errorf("Facebook API error: %s", fbError.Error.Message)
	}

	var postResp FacebookPostResponse
	if err := json.Unmarshal(body, &postResp); err != nil {
		return "", err
	}

	return postResp.ID, nil
}

func (f *FacebookPublisher) publishWithMedia(post *models.Post, pageAccessToken, pageID string) (string, error) {
	// For multiple images, we need to upload them first and then create a post
	if len(post.Media) == 1 && post.Media[0].Type == models.MediaImage {
		// Single image - can post directly
		return f.publishSinglePhoto(post, pageAccessToken, pageID)
	} else if len(post.Media) > 1 {
		// Multiple images - need to upload first then create album post
		return f.publishMultiplePhotos(post, pageAccessToken, pageID)
	}

	return "", fmt.Errorf("unsupported media configuration")
}

func (f *FacebookPublisher) publishSinglePhoto(post *models.Post, pageAccessToken, pageID string) (string, error) {
	media := post.Media[0]
	return f.uploadPhoto(media, pageAccessToken, pageID, true, post.Content)
}

func (f *FacebookPublisher) publishMultiplePhotos(post *models.Post, pageAccessToken, pageID string) (string, error) {
	// Step 1: Upload all photos without publishing (bounded concurrency)
	photoIDs := []string{}
	var mu sync.Mutex
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	errCh := make(chan error, 1)

	for _, media := range post.Media {
		if media.Type != models.MediaImage {
			continue
		}
		m := media
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			photoID, err := f.uploadPhoto(m, pageAccessToken, pageID, false, "")
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
				return
			}
			mu.Lock()
			photoIDs = append(photoIDs, photoID)
			mu.Unlock()
		}()
	}
	wg.Wait()
	select {
	case e := <-errCh:
		return "", e
	default:
	}

	// Step 2: Create a post with all photos
	cfg := config.Load()
	url := fmt.Sprintf("https://graph.facebook.com/%s/%s/feed", cfg.FacebookVersion, pageID)

	// Build attached_media parameter
	attachedMedia := []map[string]string{}
	for _, photoID := range photoIDs {
		attachedMedia = append(attachedMedia, map[string]string{
			"media_fbid": photoID,
		})
	}

	payload := map[string]interface{}{
		"message":        post.Content,
		"attached_media": attachedMedia,
	}

	jsonData, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+pageAccessToken)
	resp, err := f.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var fbError FacebookErrorResponse
		json.Unmarshal(body, &fbError)
		return "", fmt.Errorf("Facebook API error: %s", fbError.Error.Message)
	}

	var postResp FacebookPostResponse
	if err := json.Unmarshal(body, &postResp); err != nil {
		return "", err
	}

	return postResp.ID, nil
}

func (f *FacebookPublisher) uploadPhotoUnpublished(media *models.Media, pageAccessToken, pageID string) (string, error) {
	return f.uploadPhoto(media, pageAccessToken, pageID, false, "")
}

// uploadPhoto uploads a photo to the page. If published is false the photo will be uploaded unpublished.
func (f *FacebookPublisher) uploadPhoto(media *models.Media, pageAccessToken, pageID string, published bool, message string) (string, error) {
	cfg := config.Load()
	url := fmt.Sprintf("https://graph.facebook.com/%s/%s/photos", cfg.FacebookVersion, pageID)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	file, err := os.Open(media.Path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	part, err := writer.CreateFormFile("source", filepath.Base(media.Path))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, file); err != nil {
		return "", err
	}

	if message != "" {
		writer.WriteField("message", message)
	}
	if !published {
		writer.WriteField("published", "false")
	}

	writer.Close()

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+pageAccessToken)

	resp, err := f.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var fbError FacebookErrorResponse
		json.Unmarshal(respBody, &fbError)
		return "", fmt.Errorf("Facebook API error: %s", fbError.Error.Message)
	}

	var photoResp FacebookPhotoResponse
	if err := json.Unmarshal(respBody, &photoResp); err != nil {
		return "", err
	}

	return photoResp.ID, nil
}
