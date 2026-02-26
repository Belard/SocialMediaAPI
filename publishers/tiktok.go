package publishers

import (
	"SocialMediaAPI/config"
	"SocialMediaAPI/models"
	"SocialMediaAPI/utils"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type TikTokPublisher struct {
	client *http.Client
}

type tiktokErrorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		LogID   string `json:"log_id"`
	} `json:"error"`
}

// NewTikTokPublisher creates a TikTokPublisher with an injectable http.Client.
func NewTikTokPublisher(client *http.Client) *TikTokPublisher {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &TikTokPublisher{client: client}
}

func (t *TikTokPublisher) httpClient() *http.Client {
	if t.client == nil {
		t.client = &http.Client{Timeout: 60 * time.Second}
	}
	return t.client
}

func (t *TikTokPublisher) Publish(post *models.Post, cred *models.PlatformCredentials) models.PublishResult {
	utils.Infof("tiktok publish started post_id=%s user_id=%s media_count=%d post_type=%s", post.ID, post.UserID, len(post.Media), post.PostType)

	if cred == nil || cred.AccessToken == "" {
		utils.Warnf("tiktok publish missing credentials post_id=%s user_id=%s", post.ID, post.UserID)
		return models.PublishResult{
			Platform: models.TikTok,
			Success:  false,
			Message:  "Missing TikTok credentials",
		}
	}

	// TikTok only supports short-form video posts
	if post.PostType != models.PostTypeShort {
		utils.Warnf("tiktok publish rejected: unsupported post_type post_id=%s post_type=%s", post.ID, post.PostType)
		return models.PublishResult{
			Platform: models.TikTok,
			Success:  false,
			Message:  "TikTok only supports short-form video posts (post_type must be 'short')",
		}
	}

	// Find the video media
	var videoMedia *models.Media
	for _, media := range post.Media {
		if media.Type == models.MediaVideo {
			videoMedia = media
			break
		}
	}

	if videoMedia == nil {
		utils.Warnf("tiktok publish no video found post_id=%s", post.ID)
		return models.PublishResult{
			Platform: models.TikTok,
			Success:  false,
			Message:  "TikTok requires a video attachment",
		}
	}

	// Step 1: Query creator info to validate privacy level options
	tiktokPrivacy := mapToTikTokPrivacy(post.PrivacyLevel)
	availableLevels, err := t.queryCreatorInfo(cred.AccessToken)
	if err != nil {
		utils.Warnf("tiktok creator info query failed post_id=%s err=%v (falling back to SELF_ONLY)", post.ID, err)
		tiktokPrivacy = "SELF_ONLY"
	} else if !containsPrivacyLevel(availableLevels, tiktokPrivacy) {
		utils.Warnf("tiktok privacy_level %s not available for user, falling back to SELF_ONLY post_id=%s available=%v", tiktokPrivacy, post.ID, availableLevels)
		tiktokPrivacy = "SELF_ONLY"
	}
	utils.Infof("tiktok resolved privacy_level=%s post_id=%s", tiktokPrivacy, post.ID)

	// Step 2: Initialize the video upload via TikTok Content Posting API
	uploadURL, publishID, err := t.initVideoUpload(cred.AccessToken, videoMedia, post.Content, post.IsSponsored, tiktokPrivacy)
	if err != nil {
		utils.Errorf("tiktok init upload failed post_id=%s err=%v", post.ID, err)
		return models.PublishResult{
			Platform: models.TikTok,
			Success:  false,
			Message:  fmt.Sprintf("Failed to initialize TikTok upload: %v", err),
		}
	}
	utils.Infof("tiktok init upload success post_id=%s publish_id=%s", post.ID, publishID)

	// Step 3: Upload the video file to the provided URL
	if err := t.uploadVideoFile(uploadURL, videoMedia); err != nil {
		utils.Errorf("tiktok video upload failed post_id=%s publish_id=%s err=%v", post.ID, publishID, err)
		return models.PublishResult{
			Platform: models.TikTok,
			Success:  false,
			Message:  fmt.Sprintf("Failed to upload video to TikTok: %v", err),
		}
	}
	utils.Infof("tiktok video upload success post_id=%s publish_id=%s", post.ID, publishID)

	// Step 4: Check publish status (TikTok processes asynchronously)
	finalStatus, err := t.waitForPublish(cred.AccessToken, publishID)
	if err != nil {
		utils.Errorf("tiktok publish status check failed post_id=%s publish_id=%s err=%v", post.ID, publishID, err)
		return models.PublishResult{
			Platform: models.TikTok,
			Success:  false,
			Message:  fmt.Sprintf("TikTok publish status check failed: %v", err),
		}
	}

	utils.Infof("tiktok publish completed post_id=%s publish_id=%s status=%s", post.ID, publishID, finalStatus)

	return models.PublishResult{
		Platform: models.TikTok,
		Success:  true,
		Message:  "Published successfully on TikTok",
		PostID:   publishID,
	}
}

// initVideoUpload initializes a direct video upload with TikTok Content Posting API.
// Returns the upload URL and publish ID.
func (t *TikTokPublisher) initVideoUpload(accessToken string, media *models.Media, title string, isSponsored bool, privacyLevel string) (string, string, error) {
	cfg := config.Load()
	_ = cfg // reserved for future version config

	endpoint := "https://open.tiktokapis.com/v2/post/publish/video/init/"

	// Get file size
	fileInfo, err := os.Stat(media.Path)
	if err != nil {
		return "", "", fmt.Errorf("failed to stat video file: %w", err)
	}
	fileSize := fileInfo.Size()

	// TikTok enforces a 150-character title limit.
	if len(title) > 150 {
		title = title[:150]
	}

	// Prepare the request body.
	// brand_content_toggle and brand_organic_toggle are REQUIRED by TikTok's
	// content sharing guidelines (https://developers.tiktok.com/doc/content-sharing-guidelines/).
	// Omitting them causes a 403. When isSponsored is true TikTok will display
	// a paid-partnership / branded-content label on the video.
	payload := map[string]interface{}{
		"post_info": map[string]interface{}{
			"title":                    title,
			"privacy_level":            privacyLevel,
			"disable_duet":             false,
			"disable_comment":          false,
			"disable_stitch":           false,
			"video_cover_timestamp_ms": 0,
			"brand_content_toggle":     isSponsored,
			"brand_organic_toggle":     isSponsored,
			"is_aigc":                  false,
		},
		"source_info": map[string]interface{}{
			"source":            "FILE_UPLOAD",
			"video_size":        fileSize,
			"chunk_size":        fileSize, // Single chunk upload for files
			"total_chunk_count": 1,
		},
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", "", err
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	resp, err := t.httpClient().Do(req)
	if err != nil {
		return "", "", fmt.Errorf("TikTok init upload request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		utils.Errorf("tiktok init upload API error status=%d body=%s", resp.StatusCode, string(body))
		return "", "", fmt.Errorf("TikTok API error (status %d): %s", resp.StatusCode, t.parseTikTokError(body))
	}

	var initResp struct {
		Data struct {
			PublishID string `json:"publish_id"`
			UploadURL string `json:"upload_url"`
		} `json:"data"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &initResp); err != nil {
		return "", "", fmt.Errorf("failed to parse init response: %w", err)
	}

	if initResp.Error.Code != "" && initResp.Error.Code != "ok" {
		return "", "", fmt.Errorf("TikTok init error: %s - %s", initResp.Error.Code, initResp.Error.Message)
	}

	if initResp.Data.UploadURL == "" {
		return "", "", fmt.Errorf("TikTok returned empty upload URL")
	}

	return initResp.Data.UploadURL, initResp.Data.PublishID, nil
}

// uploadVideoFile uploads the video binary to TikTok's upload URL.
func (t *TikTokPublisher) uploadVideoFile(uploadURL string, media *models.Media) error {
	videoFile, err := os.Open(media.Path)
	if err != nil {
		return fmt.Errorf("failed to open video file: %w", err)
	}
	defer videoFile.Close()

	stat, err := videoFile.Stat()
	if err != nil {
		return fmt.Errorf("failed to stat video file: %w", err)
	}

	req, err := http.NewRequest("PUT", uploadURL, videoFile)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "video/mp4")
	req.Header.Set("Content-Range", fmt.Sprintf("bytes 0-%d/%d", stat.Size()-1, stat.Size()))
	req.ContentLength = stat.Size()

	resp, err := t.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("video upload request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("TikTok upload error (status %d): %s", resp.StatusCode, string(body))
	}

	return nil
}

// waitForPublish polls TikTok's publish status endpoint until the video is published or fails.
func (t *TikTokPublisher) waitForPublish(accessToken, publishID string) (string, error) {
	endpoint := "https://open.tiktokapis.com/v2/post/publish/status/fetch/"

	for attempt := 0; attempt < 15; attempt++ {
		payload := map[string]string{
			"publish_id": publishID,
		}
		jsonData, _ := json.Marshal(payload)

		req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonData))
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)
		req.Header.Set("Content-Type", "application/json; charset=UTF-8")

		resp, err := t.httpClient().Do(req)
		if err != nil {
			return "", fmt.Errorf("status check request failed: %w", err)
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("TikTok status API error (status %d): %s", resp.StatusCode, string(body))
		}

		var statusResp struct {
			Data struct {
				Status string `json:"status"`
			} `json:"data"`
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}

		if err := json.Unmarshal(body, &statusResp); err != nil {
			return "", fmt.Errorf("failed to parse status response: %w", err)
		}

		status := strings.ToUpper(statusResp.Data.Status)
		utils.Debugf("tiktok publish status check attempt=%d publish_id=%s status=%s", attempt+1, publishID, status)

		switch status {
		case "PUBLISH_COMPLETE":
			return status, nil
		case "FAILED":
			errMsg := statusResp.Error.Message
			if errMsg == "" {
				errMsg = "TikTok video processing failed"
			}
			return status, fmt.Errorf("tiktok publish failed: %s", errMsg)
		}

		// PROCESSING_UPLOAD, PROCESSING_DOWNLOAD, or SENDING_TO_USER_INBOX
		time.Sleep(3 * time.Second)
	}

	return "TIMEOUT", fmt.Errorf("TikTok video processing timeout after 45 seconds")
}

func (t *TikTokPublisher) parseTikTokError(body []byte) string {
	var ttErr tiktokErrorResponse
	if err := json.Unmarshal(body, &ttErr); err == nil && ttErr.Error.Message != "" {
		return ttErr.Error.Message
	}
	return string(body)
}

// mapToTikTokPrivacy maps the generic PrivacyLevel to TikTok's privacy_level enum.
func mapToTikTokPrivacy(level models.PrivacyLevel) string {
	switch level {
	case models.PrivacyPublic:
		return "PUBLIC_TO_EVERYONE"
	case models.PrivacyFollowers:
		return "FOLLOWER_OF_CREATOR"
	case models.PrivacyFriends:
		return "MUTUAL_FOLLOW_FRIENDS"
	case models.PrivacyPrivate:
		return "SELF_ONLY"
	default:
		return "SELF_ONLY"
	}
}

// containsPrivacyLevel checks if a TikTok privacy level is in the available list.
func containsPrivacyLevel(levels []string, target string) bool {
	for _, l := range levels {
		if strings.EqualFold(l, target) {
			return true
		}
	}
	return false
}

// queryCreatorInfo calls TikTok's /v2/post/publish/creator_info/query/ to fetch
// the privacy_level_options the authenticated user has enabled.
// Returns the list of available privacy levels (e.g. ["PUBLIC_TO_EVERYONE","SELF_ONLY"]).
func (t *TikTokPublisher) queryCreatorInfo(accessToken string) ([]string, error) {
	endpoint := "https://open.tiktokapis.com/v2/post/publish/creator_info/query/"

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer([]byte("{}")))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")

	resp, err := t.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("creator info request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("creator info API error (status %d): %s", resp.StatusCode, t.parseTikTokError(body))
	}

	var infoResp struct {
		Data struct {
			PrivacyLevelOptions []string `json:"privacy_level_options"`
		} `json:"data"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &infoResp); err != nil {
		return nil, fmt.Errorf("failed to parse creator info response: %w", err)
	}

	if infoResp.Error.Code != "" && infoResp.Error.Code != "ok" {
		return nil, fmt.Errorf("creator info error: %s - %s", infoResp.Error.Code, infoResp.Error.Message)
	}

	utils.Debugf("tiktok creator info privacy_level_options=%v", infoResp.Data.PrivacyLevelOptions)
	return infoResp.Data.PrivacyLevelOptions, nil
}