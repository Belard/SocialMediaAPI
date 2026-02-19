package publishers

import (
	"SocialMediaAPI/config"
	"SocialMediaAPI/models"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type InstagramPublisher struct {
	client *http.Client
}

type instagramErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    int    `json:"code"`
	} `json:"error"`
}

func NewInstagramPublisher(client *http.Client) *InstagramPublisher {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &InstagramPublisher{client: client}
}

func (i *InstagramPublisher) httpClient() *http.Client {
	if i.client == nil {
		i.client = &http.Client{Timeout: 30 * time.Second}
	}
	return i.client
}

func (i *InstagramPublisher) Publish(post *models.Post, cred *models.PlatformCredentials) models.PublishResult {
	if cred == nil || cred.AccessToken == "" {
		return models.PublishResult{
			Platform: models.Instagram,
			Success:  false,
			Message:  "Missing Instagram credentials",
		}
	}

	if cred.PlatformUserID == "" {
		return models.PublishResult{
			Platform: models.Instagram,
			Success:  false,
			Message:  "Instagram account not connected correctly. Reconnect via OAuth to fetch Instagram Business Account ID",
		}
	}

	imageMedia := []*models.Media{}
	for _, media := range post.Media {
		if media.Type == models.MediaImage {
			imageMedia = append(imageMedia, media)
		}
	}

	if len(imageMedia) == 0 {
		return models.PublishResult{
			Platform: models.Instagram,
			Success:  false,
			Message:  "Instagram requires at least one image",
		}
	}

	if strings.Contains(strings.ToLower(imageMedia[0].URL), "localhost") || strings.Contains(strings.ToLower(imageMedia[0].URL), "127.0.0.1") {
		return models.PublishResult{
			Platform: models.Instagram,
			Success:  false,
			Message:  "Instagram cannot fetch local media URLs. Use a public BASE_URL (e.g. HTTPS domain or tunnel) so Meta servers can access your files",
		}
	}

	var postID string
	var err error
	if len(imageMedia) == 1 {
		postID, err = i.publishSingleImage(post.Content, imageMedia[0].URL, cred.PlatformUserID, cred.AccessToken)
	} else {
		postID, err = i.publishCarousel(post.Content, imageMedia, cred.PlatformUserID, cred.AccessToken)
	}

	if err != nil {
		return models.PublishResult{
			Platform: models.Instagram,
			Success:  false,
			Message:  fmt.Sprintf("Error publishing to Instagram: %v", err),
		}
	}

	return models.PublishResult{
		Platform: models.Instagram,
		Success:  true,
		Message:  "Published successfully on Instagram",
		PostID:   postID,
	}
}

func (i *InstagramPublisher) publishSingleImage(caption, imageURL, instagramUserID, accessToken string) (string, error) {
	containerID, err := i.createMediaContainer(instagramUserID, accessToken, map[string]string{
		"image_url": imageURL,
		"caption":   caption,
	})
	if err != nil {
		return "", err
	}

	if err := i.waitContainerReady(containerID, accessToken); err != nil {
		return "", err
	}

	return i.publishContainer(instagramUserID, accessToken, containerID)
}

func (i *InstagramPublisher) publishCarousel(caption string, media []*models.Media, instagramUserID, accessToken string) (string, error) {
	children := make([]string, 0, len(media))
	for _, m := range media {
		containerID, err := i.createMediaContainer(instagramUserID, accessToken, map[string]string{
			"image_url":        m.URL,
			"is_carousel_item": "true",
		})
		if err != nil {
			return "", err
		}
		if err := i.waitContainerReady(containerID, accessToken); err != nil {
			return "", err
		}
		children = append(children, containerID)
	}

	carouselContainerID, err := i.createMediaContainer(instagramUserID, accessToken, map[string]string{
		"media_type": "CAROUSEL",
		"children":   strings.Join(children, ","),
		"caption":    caption,
	})
	if err != nil {
		return "", err
	}

	if err := i.waitContainerReady(carouselContainerID, accessToken); err != nil {
		return "", err
	}

	return i.publishContainer(instagramUserID, accessToken, carouselContainerID)
}

func (i *InstagramPublisher) createMediaContainer(instagramUserID, accessToken string, values map[string]string) (string, error) {
	cfg := config.Load()
	endpoint := fmt.Sprintf("https://graph.instagram.com/%s/%s/media", cfg.InstagramVersion, instagramUserID)

	form := url.Values{}
	for k, v := range values {
		form.Set(k, v)
	}
	form.Set("access_token", accessToken)

	req, err := http.NewRequest("POST", endpoint, bytes.NewBufferString(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := i.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Instagram media container API error: %s", i.parseInstagramError(body))
	}

	var data struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", err
	}
	if data.ID == "" {
		return "", fmt.Errorf("Instagram media container API returned empty container id")
	}

	return data.ID, nil
}

func (i *InstagramPublisher) publishContainer(instagramUserID, accessToken, containerID string) (string, error) {
	cfg := config.Load()
	endpoint := fmt.Sprintf("https://graph.instagram.com/%s/%s/media_publish", cfg.InstagramVersion, instagramUserID)

	form := url.Values{}
	form.Set("creation_id", containerID)
	form.Set("access_token", accessToken)

	req, err := http.NewRequest("POST", endpoint, bytes.NewBufferString(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := i.httpClient().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Instagram publish API error: %s", i.parseInstagramError(body))
	}

	var data struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return "", err
	}

	return data.ID, nil
}

func (i *InstagramPublisher) waitContainerReady(containerID, accessToken string) error {
	cfg := config.Load()
	endpoint := fmt.Sprintf("https://graph.instagram.com/%s/%s?fields=status_code&access_token=%s", cfg.InstagramVersion, containerID, url.QueryEscape(accessToken))

	for attempt := 0; attempt < 10; attempt++ {
		resp, err := i.httpClient().Get(endpoint)
		if err != nil {
			return err
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("Instagram container status API error: %s", i.parseInstagramError(body))
		}

		var status struct {
			StatusCode string `json:"status_code"`
		}
		if err := json.Unmarshal(body, &status); err != nil {
			return err
		}

		if status.StatusCode == "FINISHED" || status.StatusCode == "PUBLISHED" || status.StatusCode == "" {
			return nil
		}

		if status.StatusCode == "ERROR" {
			return fmt.Errorf("Instagram media processing failed")
		}

		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("Instagram media processing timeout")
}

func (i *InstagramPublisher) parseInstagramError(body []byte) string {
	var igErr instagramErrorResponse
	if err := json.Unmarshal(body, &igErr); err == nil && igErr.Error.Message != "" {
		return igErr.Error.Message
	}
	return string(body)
}
