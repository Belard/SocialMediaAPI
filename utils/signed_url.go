package utils

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strconv"
	"time"

	"SocialMediaAPI/models"
)

// SignURL appends HMAC-SHA256 token and expiration query parameters to a URL.
// The resulting URL can be validated by ValidateSignedURL without requiring any
// authentication headers — making it suitable for external platform fetches
// (e.g. Instagram, Facebook) while preventing unauthorised access.
func SignURL(rawURL string, key []byte, validFor time.Duration) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL // return unchanged on malformed input
	}

	expires := strconv.FormatInt(time.Now().Add(validFor).Unix(), 10)
	token := generateHMAC(parsed.Path, expires, key)

	q := parsed.Query()
	q.Set("expires", expires)
	q.Set("token", token)
	parsed.RawQuery = q.Encode()

	return parsed.String()
}

// ValidateSignedURL checks whether the HMAC token and expiration are valid for
// the given request path. Returns false if the token is missing, expired, or
// does not match the expected signature.
func ValidateSignedURL(path, token, expires string, key []byte) bool {
	if token == "" || expires == "" {
		return false
	}

	expiresUnix, err := strconv.ParseInt(expires, 10, 64)
	if err != nil {
		return false
	}

	// Reject expired URLs.
	if time.Now().Unix() > expiresUnix {
		return false
	}

	expected := generateHMAC(path, expires, key)
	return hmac.Equal([]byte(token), []byte(expected))
}

// generateHMAC produces a hex-encoded HMAC-SHA256 over "path\nexpires".
func generateHMAC(path, expires string, key []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(path + "\n" + expires))
	return hex.EncodeToString(mac.Sum(nil))
}

// SignMediaURL signs a single media URL and returns a shallow copy of the
// Media struct with the signed URL. The original is never mutated.
func SignMediaURL(m *models.Media, key []byte, validFor time.Duration) *models.Media {
	copy := *m
	copy.URL = SignURL(m.URL, key, validFor)
	return &copy
}

// SignMediaList returns a new slice of Media with all URLs signed.
// Original media objects are not mutated.
func SignMediaList(media []*models.Media, key []byte, validFor time.Duration) []*models.Media {
	signed := make([]*models.Media, len(media))
	for i, m := range media {
		signed[i] = SignMediaURL(m, key, validFor)
	}
	return signed
}
