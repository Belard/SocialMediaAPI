package publishers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"SocialMediaAPI/models"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type FacebookPublisher struct{}

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

func (f *FacebookPublisher) getPageAccessToken(userAccessToken string) (string, string, error) {
	url := fmt.Sprintf("https://graph.facebook.com/v18.0/me/accounts?access_token=%s", userAccessToken)

	resp, err := http.Get(url)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var fbError FacebookErrorResponse
		json.Unmarshal(body, &fbError)
		return "", "", fmt.Errorf("Facebook API error: %s", fbError.Error.Message)
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
	url := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/feed", pageID)

	payload := map[string]string{
		"message":      post.Content,
		"access_token": pageAccessToken,
	}

	jsonData, _ := json.Marshal(payload)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
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
	url := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/photos", pageID)

	// Create multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add image file
	media := post.Media[0]
	file, err := os.Open(media.Path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	part, err := writer.CreateFormFile("source", filepath.Base(media.Path))
	if err != nil {
		return "", err
	}
	io.Copy(part, file)

	// Add fields
	writer.WriteField("message", post.Content)
	writer.WriteField("access_token", pageAccessToken)

	writer.Close()

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
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

func (f *FacebookPublisher) publishMultiplePhotos(post *models.Post, pageAccessToken, pageID string) (string, error) {
	// Step 1: Upload all photos without publishing
	photoIDs := []string{}

	for _, media := range post.Media {
		if media.Type != models.MediaImage {
			continue
		}

		photoID, err := f.uploadPhotoUnpublished(media, pageAccessToken, pageID)
		if err != nil {
			return "", err
		}
		photoIDs = append(photoIDs, photoID)
	}

	// Step 2: Create a post with all photos
	url := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/feed", pageID)

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
		"access_token":   pageAccessToken,
	}

	jsonData, _ := json.Marshal(payload)

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
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
	url := fmt.Sprintf("https://graph.facebook.com/v18.0/%s/photos", pageID)

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
	io.Copy(part, file)

	writer.WriteField("published", "false")
	writer.WriteField("access_token", pageAccessToken)

	writer.Close()

	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
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