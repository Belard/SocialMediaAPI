package publishers

import (
	"SocialMediaAPI/models"
	"SocialMediaAPI/utils"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// YouTubePublisher implements PlatformPublisher for the YouTube Data API v3.
type YouTubePublisher struct {
	client *http.Client
}

// youtubeErrorResponse represents a YouTube Data API error.
type youtubeErrorResponse struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Errors  []struct {
			Message string `json:"message"`
			Domain  string `json:"domain"`
			Reason  string `json:"reason"`
		} `json:"errors"`
	} `json:"error"`
}

// youtubeVideoSnippet holds the snippet part of a YouTube video resource.
type youtubeVideoSnippet struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags,omitempty"`
	CategoryID  string   `json:"categoryId"`
}

// youtubeVideoStatus holds the status part of a YouTube video resource.
type youtubeVideoStatus struct {
	PrivacyStatus           string `json:"privacyStatus"`
	SelfDeclaredMadeForKids bool   `json:"selfDeclaredMadeForKids"`
}

// youtubeVideoResource is the metadata sent when inserting a video.
type youtubeVideoResource struct {
	Snippet *youtubeVideoSnippet `json:"snippet"`
	Status  *youtubeVideoStatus  `json:"status"`
}

// youtubeInsertResponse is the response from a successful video insert.
type youtubeInsertResponse struct {
	ID      string `json:"id"`
	Snippet struct {
		Title string `json:"title"`
	} `json:"snippet"`
}

// NewYouTubePublisher creates a YouTubePublisher with an injectable http.Client.
// If nil is passed a default client with a generous timeout is used.
func NewYouTubePublisher(client *http.Client) *YouTubePublisher {
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	return &YouTubePublisher{client: client}
}

func (y *YouTubePublisher) httpClient() *http.Client {
	if y.client == nil {
		y.client = &http.Client{Timeout: 120 * time.Second}
	}
	return y.client
}

// Publish implements PlatformPublisher.
// YouTube requires a video attachment for every post.
// Short-form posts are published as YouTube Shorts.
func (y *YouTubePublisher) Publish(post *models.Post, cred *models.PlatformCredentials) models.PublishResult {
	utils.Infof("youtube publish started post_id=%s user_id=%s media_count=%d post_type=%s", post.ID, post.UserID, len(post.Media), post.PostType)

	if cred == nil || cred.AccessToken == "" {
		utils.Warnf("youtube publish missing credentials post_id=%s user_id=%s", post.ID, post.UserID)
		return models.PublishResult{
			Platform: models.YouTube,
			Success:  false,
			Message:  "Missing YouTube credentials",
		}
	}

	// Check if token is expired
	tokenValidator := utils.NewTokenValidator()
	if tokenValidator.IsTokenExpired(cred) {
		utils.Warnf("youtube token expired post_id=%s user_id=%s", post.ID, post.UserID)
		return models.PublishResult{
			Platform: models.YouTube,
			Success:  false,
			Message:  "YouTube token has expired. Please reconnect your account via OAuth",
		}
	}

	// YouTube always requires a video
	var videoMedia *models.Media
	for _, media := range post.Media {
		if media.Type == models.MediaVideo {
			videoMedia = media
			break
		}
	}

	if videoMedia == nil {
		utils.Warnf("youtube publish no video found post_id=%s", post.ID)
		return models.PublishResult{
			Platform: models.YouTube,
			Success:  false,
			Message:  "YouTube requires a video attachment",
		}
	}

	isShort := post.PostType == models.PostTypeShort

	videoID, err := y.uploadVideo(post, videoMedia, cred.AccessToken, isShort)
	if err != nil {
		utils.Errorf("youtube publish failed post_id=%s err=%v", post.ID, err)
		return models.PublishResult{
			Platform: models.YouTube,
			Success:  false,
			Message:  fmt.Sprintf("Error publishing to YouTube: %v", err),
		}
	}

	msg := "Published successfully on YouTube"
	if isShort {
		msg = "Published successfully as YouTube Short"
	}
	utils.Infof("youtube publish succeeded post_id=%s video_id=%s is_short=%t", post.ID, videoID, isShort)

	return models.PublishResult{
		Platform: models.YouTube,
		Success:  true,
		Message:  msg,
		PostID:   videoID,
	}
}

// uploadVideo uploads a video to YouTube using the resumable upload protocol.
// The flow is:
//  1. POST metadata to initiate a resumable upload → get upload URI
//  2. PUT the raw video bytes to the upload URI → get the completed video resource
func (y *YouTubePublisher) uploadVideo(post *models.Post, media *models.Media, accessToken string, isShort bool) (string, error) {
	// Build video metadata
	title := post.Content
	if len(title) > 100 {
		title = title[:100]
	}
	if title == "" {
		title = "Untitled"
	}
	description := post.Content

	// For Shorts, prepend #Shorts tag so YouTube recognises it
	tags := []string{}
	if isShort {
		tags = append(tags, "Shorts")
		if len(title) <= 92 {
			title = title + " #Shorts"
		}
	}

	videoResource := youtubeVideoResource{
		Snippet: &youtubeVideoSnippet{
			Title:       title,
			Description: description,
			Tags:        tags,
			CategoryID:  "22", // "People & Blogs" — safe default
		},
		Status: &youtubeVideoStatus{
			PrivacyStatus:           "public",
			SelfDeclaredMadeForKids: false,
		},
	}

	// --- Step 1: Initiate resumable upload ---
	uploadURI, err := y.initiateResumableUpload(videoResource, accessToken)
	if err != nil {
		return "", fmt.Errorf("failed to initiate YouTube upload: %w", err)
	}
	utils.Debugf("youtube resumable upload initiated post_id=%s", post.ID)

	// --- Step 2: Upload the video file ---
	videoID, err := y.uploadVideoFile(uploadURI, media)
	if err != nil {
		return "", fmt.Errorf("failed to upload video to YouTube: %w", err)
	}

	return videoID, nil
}

// initiateResumableUpload sends the video metadata and returns the resumable upload URI.
func (y *YouTubePublisher) initiateResumableUpload(resource youtubeVideoResource, accessToken string) (string, error) {
	utils.Debugf("youtube initiating resumable upload")

	metadataJSON, err := json.Marshal(resource)
	if err != nil {
		return "", fmt.Errorf("failed to marshal video metadata: %w", err)
	}

	endpoint := "https://www.googleapis.com/upload/youtube/v3/videos?uploadType=resumable&part=snippet,status"

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(metadataJSON))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
	req.Header.Set("X-Upload-Content-Type", "video/*")

	resp, err := y.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("youtube initiate upload request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		errMsg := y.parseYouTubeError(body)
		utils.Errorf("youtube initiate upload API error status=%d body=%s", resp.StatusCode, errMsg)
		return "", fmt.Errorf("YouTube API error (status %d): %s", resp.StatusCode, errMsg)
	}

	uploadURI := resp.Header.Get("Location")
	if uploadURI == "" {
		return "", fmt.Errorf("youtube did not return a resumable upload URI")
	}

	utils.Debugf("youtube resumable upload URI obtained")
	return uploadURI, nil
}

// uploadVideoFile uploads the raw video bytes to the resumable upload URI.
func (y *YouTubePublisher) uploadVideoFile(uploadURI string, media *models.Media) (string, error) {
	utils.Debugf("youtube uploading video file path=%s", media.Path)

	file, err := os.Open(media.Path)
	if err != nil {
		return "", fmt.Errorf("failed to open video file: %w", err)
	}
	defer file.Close()

	stat, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("failed to stat video file: %w", err)
	}

	contentType := media.MimeType
	if contentType == "" {
		contentType = "video/mp4"
	}

	req, err := http.NewRequest("PUT", uploadURI, file)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", contentType)
	req.ContentLength = stat.Size()

	resp, err := y.httpClient().Do(req)
	if err != nil {
		return "", fmt.Errorf("youtube video upload request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		errMsg := y.parseYouTubeError(body)
		return "", fmt.Errorf("YouTube upload failed (status %d): %s", resp.StatusCode, errMsg)
	}

	var insertResp youtubeInsertResponse
	if err := json.Unmarshal(body, &insertResp); err != nil {
		return "", fmt.Errorf("failed to parse YouTube upload response: %w", err)
	}

	if insertResp.ID == "" {
		return "", fmt.Errorf("youtube returned empty video ID")
	}

	utils.Debugf("youtube video upload success video_id=%s", insertResp.ID)
	return insertResp.ID, nil
}

// parseYouTubeError extracts a human-readable error from a YouTube API error body.
func (y *YouTubePublisher) parseYouTubeError(body []byte) string {
	var errResp youtubeErrorResponse
	if err := json.Unmarshal(body, &errResp); err == nil {
		if errResp.Error.Message != "" {
			return errResp.Error.Message
		}
		if len(errResp.Error.Errors) > 0 {
			return errResp.Error.Errors[0].Message
		}
	}
	return string(body)
}
