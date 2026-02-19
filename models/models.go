package models

import "time"

type Platform string

const (
	Twitter   Platform = "twitter"
	Facebook  Platform = "facebook"
	LinkedIn  Platform = "linkedin"
	Instagram Platform = "instagram"
	TikTok    Platform = "tiktok"
)

type PostStatus string

const (
	StatusDraft     PostStatus = "draft"
	StatusScheduled PostStatus = "scheduled"
	StatusPublished PostStatus = "published"
	StatusFailed    PostStatus = "failed"
)

type PostType string

const (
	PostTypeNormal PostType = "normal"
	PostTypeShort  PostType = "short"
)

type MediaType string

const (
	MediaImage MediaType = "image"
	MediaVideo MediaType = "video"
)

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Password  string    `json:"-"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type Media struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Filename  string    `json:"filename"`
	Path      string    `json:"path"`
	URL       string    `json:"url"`
	Type      MediaType `json:"type"`
	Size      int64     `json:"size"`
	MimeType  string    `json:"mime_type"`
	CreatedAt time.Time `json:"created_at"`
}

type Post struct {
	ID           string     `json:"id"`
	UserID       string     `json:"user_id"`
	Content      string     `json:"content"`
	PostType     PostType   `json:"post_type"`
	MediaIDs     []string   `json:"media_ids,omitempty"`
	Media        []*Media   `json:"media,omitempty"`
	Platforms    []Platform `json:"platforms"`
	Status       PostStatus `json:"status"`
	ScheduledFor *time.Time `json:"scheduled_for,omitempty"`
	PublishedAt  *time.Time `json:"published_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

type PlatformCredentials struct {
	ID               string    `json:"id"`
	UserID           string    `json:"user_id"`
	Platform         Platform  `json:"platform"`
	AccessToken      string    `json:"-"`
	RefreshToken     string    `json:"-"`
	Secret           string    `json:"-"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	TokenType        string    `json:"token_type"`
	// Platform-independent identity fields
	PlatformUserID   string    `json:"platform_user_id,omitempty"`
	PlatformPageID   string    `json:"platform_page_id,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type PublishResult struct {
	Platform Platform `json:"platform"`
	Success  bool     `json:"success"`
	Message  string   `json:"message"`
	PostID   string   `json:"post_id,omitempty"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type AuthResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

type PublishResponse struct {
	PostID  string          `json:"post_id"`
	Results []PublishResult `json:"results"`
}

type UploadResponse struct {
	Media *Media `json:"media"`
}