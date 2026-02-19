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
	utils.Infof("facebook publish started post_id=%s user_id=%s media_count=%d post_type=%s", post.ID, post.UserID, len(post.Media), post.PostType)

	if cred == nil || cred.AccessToken == "" {
		utils.Warnf("facebook publish missing credentials post_id=%s user_id=%s", post.ID, post.UserID)
		return models.PublishResult{
			Platform: models.Facebook,
			Success:  false,
			Message:  "Missing Facebook credentials",
		}
	}

	// Check if token is expired
	tokenValidator := utils.NewTokenValidator()
	if tokenValidator.IsTokenExpired(cred) {
		utils.Warnf("facebook token expired attempting refresh post_id=%s user_id=%s", post.ID, post.UserID)
		// Attempt to refresh the token
		if err := tokenValidator.RefreshFacebookToken(cred); err != nil {
			utils.Errorf("facebook token refresh failed post_id=%s user_id=%s err=%v", post.ID, post.UserID, err)
			return models.PublishResult{
				Platform: models.Facebook,
				Success:  false,
				Message:  fmt.Sprintf("Facebook token has expired and cannot be refreshed: %v", err),
			}
		}
		utils.Infof("facebook token refresh succeeded post_id=%s user_id=%s", post.ID, post.UserID)
	}

	// Get Page Access Token first
	pageAccessToken, pageID, err := f.getPageAccessToken(cred.AccessToken)
	if err != nil {
		utils.Errorf("facebook page token lookup failed post_id=%s user_id=%s err=%v", post.ID, post.UserID, err)
		return models.PublishResult{
			Platform: models.Facebook,
			Success:  false,
			Message:  fmt.Sprintf("Error getting page access token: %v", err),
		}
	}
	utils.Debugf("facebook page token lookup succeeded post_id=%s page_id=%s", post.ID, pageID)

	// Short posts → publish as Facebook Reel
	if post.PostType == models.PostTypeShort {
		utils.Infof("facebook publish mode=reel post_id=%s page_id=%s", post.ID, pageID)
		postID, err := f.publishReel(post, pageAccessToken, pageID)
		if err != nil {
			utils.Errorf("facebook reel publish failed post_id=%s page_id=%s err=%v", post.ID, pageID, err)
			return models.PublishResult{
				Platform: models.Facebook,
				Success:  false,
				Message:  fmt.Sprintf("Error publishing Facebook Reel: %v", err),
			}
		}
		utils.Infof("facebook reel publish succeeded post_id=%s page_id=%s external_post_id=%s", post.ID, pageID, postID)
		return models.PublishResult{
			Platform: models.Facebook,
			Success:  true,
			Message:  "Published successfully as Facebook Reel",
			PostID:   postID,
		}
	}

	// Normal posts — existing publishing logic
	var postID string
	if len(post.Media) > 0 {
		utils.Infof("facebook publish mode=media post_id=%s page_id=%s media_count=%d", post.ID, pageID, len(post.Media))
		postID, err = f.publishWithMedia(post, pageAccessToken, pageID)
	} else {
		utils.Infof("facebook publish mode=text post_id=%s page_id=%s", post.ID, pageID)
		postID, err = f.publishTextOnly(post, pageAccessToken, pageID)
	}

	if err != nil {
		utils.Errorf("facebook publish failed post_id=%s page_id=%s err=%v", post.ID, pageID, err)
		return models.PublishResult{
			Platform: models.Facebook,
			Success:  false,
			Message:  fmt.Sprintf("Error publishing to Facebook: %v", err),
		}
	}

	utils.Infof("facebook publish succeeded post_id=%s page_id=%s external_post_id=%s", post.ID, pageID, postID)

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
	utils.Debugf("facebook requesting page access token")

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
		utils.Errorf("facebook page access token API error status=%d code=%d message=%s", resp.StatusCode, fbError.Error.Code, fbError.Error.Message)
		
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
		utils.Warnf("facebook no pages returned for account")
		return "", "", fmt.Errorf("no Facebook pages found for this account")
	}

	// Use the first page
	page := pageResp.Data[0]
	utils.Debugf("facebook selected page page_id=%s page_name=%s", page.ID, page.Name)
	return page.AccessToken, page.ID, nil
}

func (f *FacebookPublisher) publishTextOnly(post *models.Post, pageAccessToken, pageID string) (string, error) {
	cfg := config.Load()
	url := fmt.Sprintf("https://graph.facebook.com/%s/%s/feed", cfg.FacebookVersion, pageID)
	utils.Debugf("facebook posting text content post_id=%s page_id=%s", post.ID, pageID)

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
		utils.Errorf("facebook text post API error post_id=%s page_id=%s status=%d message=%s", post.ID, pageID, resp.StatusCode, fbError.Error.Message)
		return "", fmt.Errorf("Facebook API error: %s", fbError.Error.Message)
	}

	var postResp FacebookPostResponse
	if err := json.Unmarshal(body, &postResp); err != nil {
		return "", err
	}

	return postResp.ID, nil
}

func (f *FacebookPublisher) publishWithMedia(post *models.Post, pageAccessToken, pageID string) (string, error) {
	utils.Debugf("facebook publishWithMedia post_id=%s page_id=%s media_count=%d", post.ID, pageID, len(post.Media))
	// For multiple images, we need to upload them first and then create a post
	if len(post.Media) == 1 && post.Media[0].Type == models.MediaImage {
		// Single image - can post directly
		utils.Debugf("facebook media flow single image post_id=%s page_id=%s", post.ID, pageID)
		return f.publishSinglePhoto(post, pageAccessToken, pageID)
	} else if len(post.Media) > 1 {
		// Multiple images - need to upload first then create album post
		utils.Debugf("facebook media flow multiple images post_id=%s page_id=%s count=%d", post.ID, pageID, len(post.Media))
		return f.publishMultiplePhotos(post, pageAccessToken, pageID)
	}

	return "", fmt.Errorf("unsupported media configuration")
}

func (f *FacebookPublisher) publishSinglePhoto(post *models.Post, pageAccessToken, pageID string) (string, error) {
	media := post.Media[0]
	return f.uploadPhoto(media, pageAccessToken, pageID, true, post.Content)
}

func (f *FacebookPublisher) publishMultiplePhotos(post *models.Post, pageAccessToken, pageID string) (string, error) {
	utils.Infof("facebook uploading multiple photos post_id=%s page_id=%s", post.ID, pageID)
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
				utils.Errorf("facebook photo upload failed post_id=%s page_id=%s media_id=%s err=%v", post.ID, pageID, m.ID, err)
				select {
				case errCh <- err:
				default:
				}
				return
			}
			utils.Debugf("facebook photo uploaded unpublished post_id=%s page_id=%s media_id=%s photo_id=%s", post.ID, pageID, m.ID, photoID)
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
	utils.Debugf("facebook all unpublished photos uploaded post_id=%s page_id=%s count=%d", post.ID, pageID, len(photoIDs))

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
		utils.Errorf("facebook multi-photo feed post API error post_id=%s page_id=%s status=%d message=%s", post.ID, pageID, resp.StatusCode, fbError.Error.Message)
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
	utils.Debugf("facebook upload photo start page_id=%s media_id=%s published=%t", pageID, media.ID, published)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	file, err := os.Open(media.Path)
	if err != nil {
		utils.Errorf("facebook upload photo open file failed media_id=%s path=%s err=%v", media.ID, media.Path, err)
		return "", err
	}
	defer file.Close()

	part, err := writer.CreateFormFile("source", filepath.Base(media.Path))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, file); err != nil {
		utils.Errorf("facebook upload photo copy failed media_id=%s err=%v", media.ID, err)
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
		utils.Errorf("facebook upload photo API error page_id=%s media_id=%s status=%d message=%s", pageID, media.ID, resp.StatusCode, fbError.Error.Message)
		return "", fmt.Errorf("Facebook API error: %s", fbError.Error.Message)
	}

	var photoResp FacebookPhotoResponse
	if err := json.Unmarshal(respBody, &photoResp); err != nil {
		return "", err
	}
	utils.Debugf("facebook upload photo success page_id=%s media_id=%s photo_id=%s", pageID, media.ID, photoResp.ID)

	return photoResp.ID, nil
}

// publishReel publishes a short-form video as a Facebook Reel.
// Uses the two-step flow: initialize upload → upload video → finish.
func (f *FacebookPublisher) publishReel(post *models.Post, pageAccessToken, pageID string) (string, error) {
	cfg := config.Load()

	// Find the first video in the post's media
	var videoMedia *models.Media
	for _, media := range post.Media {
		if media.Type == models.MediaVideo {
			videoMedia = media
			break
		}
	}
	if videoMedia == nil {
		return "", fmt.Errorf("Facebook Reels require a video attachment")
	}

	utils.Infof("facebook reel upload start post_id=%s page_id=%s media_id=%s", post.ID, pageID, videoMedia.ID)

	// Step 1: Initialize the video upload
	initURL := fmt.Sprintf("https://graph.facebook.com/%s/%s/video_reels", cfg.FacebookVersion, pageID)

	initPayload := map[string]string{
		"upload_phase": "start",
	}
	jsonData, _ := json.Marshal(initPayload)

	req, err := http.NewRequest("POST", initURL, bytes.NewBuffer(jsonData))
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
		utils.Errorf("facebook reel init API error post_id=%s page_id=%s status=%d message=%s", post.ID, pageID, resp.StatusCode, fbError.Error.Message)
		return "", fmt.Errorf("Facebook Reel init error: %s", fbError.Error.Message)
	}

	var initResp struct {
		VideoID string `json:"video_id"`
		UploadURL string `json:"upload_url"`
	}
	if err := json.Unmarshal(body, &initResp); err != nil {
		return "", fmt.Errorf("failed to parse reel init response: %w", err)
	}

	utils.Debugf("facebook reel init success post_id=%s video_id=%s", post.ID, initResp.VideoID)

	// Step 2: Upload the video binary
	videoFile, err := os.Open(videoMedia.Path)
	if err != nil {
		return "", fmt.Errorf("failed to open video file: %w", err)
	}
	defer videoFile.Close()

	stat, err := videoFile.Stat()
	if err != nil {
		return "", fmt.Errorf("failed to stat video file: %w", err)
	}

	uploadReq, err := http.NewRequest("POST", initResp.UploadURL, videoFile)
	if err != nil {
		return "", err
	}
	uploadReq.Header.Set("Authorization", "OAuth "+pageAccessToken)
	uploadReq.Header.Set("offset", "0")
	uploadReq.Header.Set("file_size", fmt.Sprintf("%d", stat.Size()))
	uploadReq.ContentLength = stat.Size()

	uploadResp, err := f.httpClient().Do(uploadReq)
	if err != nil {
		return "", fmt.Errorf("video upload request failed: %w", err)
	}
	defer uploadResp.Body.Close()

	uploadBody, _ := io.ReadAll(uploadResp.Body)
	if uploadResp.StatusCode != http.StatusOK {
		utils.Errorf("facebook reel upload API error post_id=%s status=%d body=%s", post.ID, uploadResp.StatusCode, string(uploadBody))
		return "", fmt.Errorf("Facebook Reel upload error: %s", string(uploadBody))
	}
	utils.Debugf("facebook reel upload success post_id=%s video_id=%s", post.ID, initResp.VideoID)

	// Step 3: Publish (finish) the reel
	finishURL := fmt.Sprintf("https://graph.facebook.com/%s/%s/video_reels", cfg.FacebookVersion, pageID)

	finishPayload := map[string]string{
		"upload_phase": "finish",
		"video_id":     initResp.VideoID,
		"title":        post.Content,
		"description":  post.Content,
	}
	jsonData, _ = json.Marshal(finishPayload)

	finishReq, err := http.NewRequest("POST", finishURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	finishReq.Header.Set("Content-Type", "application/json")
	finishReq.Header.Set("Authorization", "Bearer "+pageAccessToken)

	finishResp, err := f.httpClient().Do(finishReq)
	if err != nil {
		return "", fmt.Errorf("reel finish request failed: %w", err)
	}
	defer finishResp.Body.Close()

	finishBody, _ := io.ReadAll(finishResp.Body)
	if finishResp.StatusCode != http.StatusOK {
		var fbError FacebookErrorResponse
		json.Unmarshal(finishBody, &fbError)
		utils.Errorf("facebook reel finish API error post_id=%s status=%d message=%s", post.ID, finishResp.StatusCode, fbError.Error.Message)
		return "", fmt.Errorf("Facebook Reel publish error: %s", fbError.Error.Message)
	}

	var finishResult struct {
		Success bool `json:"success"`
	}
	json.Unmarshal(finishBody, &finishResult)

	utils.Infof("facebook reel published post_id=%s video_id=%s", post.ID, initResp.VideoID)
	return initResp.VideoID, nil
}
