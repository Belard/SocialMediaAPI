package publishers

import (
	"SocialMediaAPI/models"
	"SocialMediaAPI/utils"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// TwitterPublisher implements PlatformPublisher for the Twitter/X API v2.
type TwitterPublisher struct {
	client *http.Client
}

// twitterErrorResponse represents a Twitter API v2 error payload.
type twitterErrorResponse struct {
	Detail string `json:"detail"`
	Title  string `json:"title"`
	Type   string `json:"type"`
	Status int    `json:"status"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// twitterTweetResponse represents the successful response from POST /2/tweets.
type twitterTweetResponse struct {
	Data struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	} `json:"data"`
}

// twitterMediaUploadResponse represents the response from the v1.1 media upload endpoint.
type twitterMediaUploadResponse struct {
	MediaID        int64  `json:"media_id"`
	MediaIDString  string `json:"media_id_string"`
	ExpiresAfter   int    `json:"expires_after_secs"`
	ProcessingInfo *struct {
		State          string `json:"state"`
		CheckAfterSecs int    `json:"check_after_secs"`
		Error          *struct {
			Code    int    `json:"code"`
			Name    string `json:"name"`
			Message string `json:"message"`
		} `json:"error"`
	} `json:"processing_info"`
}

// NewTwitterPublisher creates a TwitterPublisher with an injectable http.Client.
// If nil is passed a default client with a sensible timeout is used.
func NewTwitterPublisher(client *http.Client) *TwitterPublisher {
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &TwitterPublisher{client: client}
}

func (t *TwitterPublisher) httpClient() *http.Client {
	if t.client == nil {
		t.client = &http.Client{Timeout: 60 * time.Second}
	}
	return t.client
}

// Publish implements PlatformPublisher. It rejects short-form posts and
// publishes either text-only tweets or tweets with media attachments.
func (t *TwitterPublisher) Publish(post *models.Post, cred *models.PlatformCredentials) models.PublishResult {
	utils.Infof("twitter publish started post_id=%s user_id=%s media_count=%d post_type=%s", post.ID, post.UserID, len(post.Media), post.PostType)

	if cred == nil || cred.AccessToken == "" {
		utils.Warnf("twitter publish missing credentials post_id=%s user_id=%s", post.ID, post.UserID)
		return models.PublishResult{
			Platform: models.Twitter,
			Success:  false,
			Message:  "Missing Twitter credentials",
		}
	}

	// Check if token is expired
	tokenValidator := utils.NewTokenValidator()
	if tokenValidator.IsTokenExpired(cred) {
		utils.Warnf("twitter token expired post_id=%s user_id=%s", post.ID, post.UserID)
		return models.PublishResult{
			Platform: models.Twitter,
			Success:  false,
			Message:  "Twitter token has expired. Please reconnect your account via OAuth",
		}
	}

	// Twitter/X does NOT support short-form video posts (Reels/Shorts).
	if post.PostType == models.PostTypeShort {
		utils.Warnf("twitter publish rejected: shorts not supported post_id=%s", post.ID)
		return models.PublishResult{
			Platform: models.Twitter,
			Success:  false,
			Message:  "Twitter does not support short-form video posts. Use post_type 'normal' instead",
		}
	}

	// Publish with or without media
	var tweetID string
	var err error

	if len(post.Media) > 0 {
		utils.Infof("twitter publish mode=media post_id=%s media_count=%d", post.ID, len(post.Media))
		tweetID, err = t.publishWithMedia(post, cred.AccessToken)
	} else {
		utils.Infof("twitter publish mode=text post_id=%s", post.ID)
		tweetID, err = t.publishTextOnly(post.Content, cred.AccessToken)
	}

	if err != nil {
		utils.Errorf("twitter publish failed post_id=%s err=%v", post.ID, err)
		return models.PublishResult{
			Platform: models.Twitter,
			Success:  false,
			Message:  fmt.Sprintf("Error publishing to Twitter: %v", err),
		}
	}

	utils.Infof("twitter publish succeeded post_id=%s external_tweet_id=%s", post.ID, tweetID)

	return models.PublishResult{
		Platform: models.Twitter,
		Success:  true,
		Message:  "Published successfully on Twitter",
		PostID:   tweetID,
	}
}

// publishTextOnly creates a text-only tweet via Twitter API v2.
func (t *TwitterPublisher) publishTextOnly(text string, accessToken string) (string, error) {
	utils.Debugf("twitter posting text content")

	payload := map[string]interface{}{
		"text": text,
	}

	return t.createTweet(payload, accessToken)
}

// publishWithMedia uploads media attachments then creates a tweet referencing them.
func (t *TwitterPublisher) publishWithMedia(post *models.Post, accessToken string) (string, error) {
	mediaIDs := []string{}

	for _, media := range post.Media {
		mediaID, err := t.uploadMedia(media, accessToken)
		if err != nil {
			return "", fmt.Errorf("failed to upload media %s: %w", media.ID, err)
		}
		utils.Debugf("twitter media uploaded media_id=%s twitter_media_id=%s", media.ID, mediaID)
		mediaIDs = append(mediaIDs, mediaID)
	}

	// Twitter allows up to 4 images or 1 video per tweet
	payload := map[string]interface{}{
		"text": post.Content,
		"media": map[string]interface{}{
			"media_ids": mediaIDs,
		},
	}

	return t.createTweet(payload, accessToken)
}

// createTweet calls POST /2/tweets and returns the tweet ID.
func (t *TwitterPublisher) createTweet(payload map[string]interface{}, accessToken string) (string, error) {
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal tweet payload: %w", err)
	}

	req, err := http.NewRequest("POST", "https://api.x.com/2/tweets", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := t.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("twitter API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated {
		errMsg := t.parseTwitterError(body)
		utils.Errorf("twitter create tweet API error status=%d body=%s", resp.StatusCode, errMsg)
		return "", fmt.Errorf("Twitter API error (status %d): %s", resp.StatusCode, errMsg)
	}

	var tweetResp twitterTweetResponse
	if err := json.Unmarshal(body, &tweetResp); err != nil {
		return "", fmt.Errorf("failed to parse tweet response: %w", err)
	}

	if tweetResp.Data.ID == "" {
		return "", fmt.Errorf("twitter returned empty tweet ID")
	}

	return tweetResp.Data.ID, nil
}

// uploadMedia uploads a single media file to Twitter via the v1.1 media upload endpoint.
// For images it uses the simple upload; for videos it uses the chunked INIT/APPEND/FINALIZE flow.
func (t *TwitterPublisher) uploadMedia(media *models.Media, accessToken string) (string, error) {
	if media.Type == models.MediaVideo {
		return t.uploadMediaChunked(media, accessToken)
	}

	// Simple upload for images
	return t.uploadMediaSimple(media, accessToken)
}

// uploadMediaSimple performs a simple multipart media upload (suitable for images).
func (t *TwitterPublisher) uploadMediaSimple(media *models.Media, accessToken string) (string, error) {
	utils.Debugf("twitter simple media upload media_id=%s path=%s", media.ID, media.Path)

	file, err := os.Open(media.Path)
	if err != nil {
		return "", fmt.Errorf("failed to open media file: %w", err)
	}
	defer file.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("media", filepath.Base(media.Path))
	if err != nil {
		return "", fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return "", fmt.Errorf("failed to copy media data: %w", err)
	}
	writer.Close()

	req, err := http.NewRequest("POST", "https://upload.x.com/1.1/media/upload.json", &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := t.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("twitter media upload request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		errMsg := t.parseTwitterError(body)
		return "", fmt.Errorf("twitter media upload failed (status %d): %s", resp.StatusCode, errMsg)
	}

	var uploadResp twitterMediaUploadResponse
	if err := json.Unmarshal(body, &uploadResp); err != nil {
		return "", fmt.Errorf("failed to parse media upload response: %w", err)
	}

	if uploadResp.MediaIDString == "" {
		return "", fmt.Errorf("twitter returned empty media ID")
	}

	utils.Debugf("twitter simple media upload success twitter_media_id=%s", uploadResp.MediaIDString)
	return uploadResp.MediaIDString, nil
}

// uploadMediaChunked uses the INIT / APPEND / FINALIZE flow for video uploads.
func (t *TwitterPublisher) uploadMediaChunked(media *models.Media, accessToken string) (string, error) {
	utils.Debugf("twitter chunked media upload media_id=%s path=%s", media.ID, media.Path)

	fileInfo, err := os.Stat(media.Path)
	if err != nil {
		return "", fmt.Errorf("failed to stat media file: %w", err)
	}
	totalBytes := fileInfo.Size()

	// Determine media type from the MIME type
	mediaType := media.MimeType
	if mediaType == "" {
		mediaType = "video/mp4"
	}

	// --- INIT ---
	initPayload := fmt.Sprintf("command=INIT&media_type=%s&total_bytes=%d&media_category=tweet_video",
		mediaType, totalBytes)

	req, err := http.NewRequest("POST", "https://upload.x.com/1.1/media/upload.json",
		strings.NewReader(initPayload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := t.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("twitter INIT request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("twitter INIT failed (status %d): %s", resp.StatusCode, t.parseTwitterError(body))
	}

	var initResp twitterMediaUploadResponse
	if err := json.Unmarshal(body, &initResp); err != nil {
		return "", fmt.Errorf("failed to parse INIT response: %w", err)
	}
	mediaIDStr := initResp.MediaIDString
	if mediaIDStr == "" {
		mediaIDStr = strconv.FormatInt(initResp.MediaID, 10)
	}
	utils.Debugf("twitter INIT success media_id=%s", mediaIDStr)

	// --- APPEND (upload in 5 MB chunks) ---
	const chunkSize = 5 * 1024 * 1024 // 5 MB
	file, err := os.Open(media.Path)
	if err != nil {
		return "", fmt.Errorf("failed to open video file: %w", err)
	}
	defer file.Close()

	segmentIndex := 0
	chunkBuf := make([]byte, chunkSize)
	for {
		n, readErr := file.Read(chunkBuf)
		if n == 0 && readErr != nil {
			break
		}

		chunk := chunkBuf[:n]
		encoded := base64.StdEncoding.EncodeToString(chunk)

		appendPayload := fmt.Sprintf("command=APPEND&media_id=%s&segment_index=%d&media_data=%s",
			mediaIDStr, segmentIndex, encoded)

		appendReq, err := http.NewRequest("POST", "https://upload.x.com/1.1/media/upload.json",
			strings.NewReader(appendPayload))
		if err != nil {
			return "", err
		}
		appendReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		appendReq.Header.Set("Authorization", "Bearer "+accessToken)

		appendResp, err := t.httpClient().Do(appendReq)
		if err != nil {
			return "", fmt.Errorf("twitter APPEND request failed (segment %d): %w", segmentIndex, err)
		}
		appendBody, _ := io.ReadAll(appendResp.Body)
		appendResp.Body.Close()

		if appendResp.StatusCode != http.StatusNoContent && appendResp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("twitter APPEND failed (segment %d, status %d): %s",
				segmentIndex, appendResp.StatusCode, t.parseTwitterError(appendBody))
		}

		segmentIndex++
		if readErr != nil {
			break
		}
	}
	utils.Debugf("twitter APPEND complete media_id=%s segments=%d", mediaIDStr, segmentIndex)

	// --- FINALIZE ---
	finalizePayload := fmt.Sprintf("command=FINALIZE&media_id=%s", mediaIDStr)

	finalizeReq, err := http.NewRequest("POST", "https://upload.x.com/1.1/media/upload.json",
		strings.NewReader(finalizePayload))
	if err != nil {
		return "", err
	}
	finalizeReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	finalizeReq.Header.Set("Authorization", "Bearer "+accessToken)

	finalizeResp, err := t.httpClient().Do(finalizeReq)
	if err != nil {
		return "", fmt.Errorf("twitter FINALIZE request failed: %w", err)
	}
	defer finalizeResp.Body.Close()

	finalizeBody, _ := io.ReadAll(finalizeResp.Body)
	if finalizeResp.StatusCode != http.StatusOK && finalizeResp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("twitter FINALIZE failed (status %d): %s",
			finalizeResp.StatusCode, t.parseTwitterError(finalizeBody))
	}

	var finalResp twitterMediaUploadResponse
	if err := json.Unmarshal(finalizeBody, &finalResp); err != nil {
		return "", fmt.Errorf("failed to parse FINALIZE response: %w", err)
	}

	// If Twitter needs processing time, poll STATUS until ready
	if finalResp.ProcessingInfo != nil {
		if err := t.waitForMediaProcessing(mediaIDStr, accessToken); err != nil {
			return "", err
		}
	}

	utils.Debugf("twitter chunked media upload success twitter_media_id=%s", mediaIDStr)
	return mediaIDStr, nil
}

// waitForMediaProcessing polls the media STATUS endpoint until processing completes.
func (t *TwitterPublisher) waitForMediaProcessing(mediaID, accessToken string) error {
	for attempt := 0; attempt < 30; attempt++ {
		statusURL := fmt.Sprintf("https://upload.x.com/1.1/media/upload.json?command=STATUS&media_id=%s", mediaID)

		req, err := http.NewRequest("GET", statusURL, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+accessToken)

		resp, err := t.httpClient().Do(req)
		if err != nil {
			return fmt.Errorf("twitter STATUS request failed: %w", err)
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var statusResp twitterMediaUploadResponse
		if err := json.Unmarshal(body, &statusResp); err != nil {
			return fmt.Errorf("failed to parse STATUS response: %w", err)
		}

		if statusResp.ProcessingInfo == nil {
			return nil // processing complete
		}

		if statusResp.ProcessingInfo.Error != nil {
			return fmt.Errorf("twitter media processing error: %s", statusResp.ProcessingInfo.Error.Message)
		}

		if statusResp.ProcessingInfo.State == "succeeded" {
			return nil
		}

		if statusResp.ProcessingInfo.State == "failed" {
			return fmt.Errorf("twitter media processing failed")
		}

		waitSecs := statusResp.ProcessingInfo.CheckAfterSecs
		if waitSecs <= 0 {
			waitSecs = 2
		}
		utils.Debugf("twitter media processing state=%s check_after=%ds media_id=%s", statusResp.ProcessingInfo.State, waitSecs, mediaID)
		time.Sleep(time.Duration(waitSecs) * time.Second)
	}

	return fmt.Errorf("twitter media processing timeout")
}

// parseTwitterError extracts a human-readable error from a Twitter API error body.
func (t *TwitterPublisher) parseTwitterError(body []byte) string {
	var errResp twitterErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil {
		if errResp.Detail != "" {
			return errResp.Detail
		}
		if errResp.Title != "" {
			return errResp.Title
		}
		if len(errResp.Errors) > 0 {
			return errResp.Errors[0].Message
		}
	}
	return string(body)
}