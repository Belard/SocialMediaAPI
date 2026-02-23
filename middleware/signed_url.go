package middleware

import (
	"net/http"
	"os"
	"strings"

	"SocialMediaAPI/services"
	"SocialMediaAPI/utils"
)

// noListingDir wraps http.Dir to prevent directory listing.
// Opening a directory returns os.ErrNotExist so that http.FileServer
// cannot enumerate contents.
type noListingDir struct {
	dir http.Dir
}

func (d noListingDir) Open(name string) (http.File, error) {
	f, err := d.dir.Open(name)
	if err != nil {
		return nil, err
	}

	stat, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	if stat.IsDir() {
		f.Close()
		return nil, os.ErrNotExist
	}

	return f, nil
}

// SignedFileServer returns an http.Handler that serves static files from dir
// with two independent authentication strategies:
//
//  1. **Signed URL** — HMAC "token" + "expires" query-string parameters
//     (used by external platform servers: Instagram, Facebook, TikTok, etc.)
//
//  2. **JWT Bearer token** — standard Authorization header; the userID extracted
//     from the JWT must match the first path segment (the owner directory).
//     (used by authenticated frontend/API clients.)
//
// Requests that satisfy neither strategy receive 403 Forbidden.
// Directory listings are blocked via noListingDir.
func SignedFileServer(dir string, signingKey []byte, authService *services.AuthService) http.Handler {
	fs := http.FileServer(noListingDir{http.Dir(dir)})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authenticateBySignedURL(r, signingKey) && !authenticateByJWT(r, authService) {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		w.Header().Set("Cache-Control", "private, max-age=3600")
		fs.ServeHTTP(w, r)
	})
}

// authenticateBySignedURL validates HMAC "token" + "expires" query-string
// parameters against the request path. Returns true when the signature is
// present, non-expired, and matches.
func authenticateBySignedURL(r *http.Request, signingKey []byte) bool {
	token := r.URL.Query().Get("token")
	expires := r.URL.Query().Get("expires")
	if token == "" || expires == "" {
		return false
	}
	return utils.ValidateSignedURL(r.URL.Path, token, expires, signingKey)
}

// authenticateByJWT extracts a Bearer token from the Authorization header,
// validates it, and checks that the JWT's userID matches the owner directory
// in the request path (first path segment after StripPrefix).
func authenticateByJWT(r *http.Request, authService *services.AuthService) bool {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return false
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return false
	}

	claims, err := authService.ValidateToken(parts[1])
	if err != nil {
		return false
	}

	ownerID := extractOwnerFromPath(r.URL.Path)
	return ownerID != "" && ownerID == claims.UserID
}

// extractOwnerFromPath returns the first non-empty path segment, which in
// this application is the owning userID (e.g. "/abc-123/file.jpg" → "abc-123").
func extractOwnerFromPath(path string) string {
	// Trim leading slash, then grab everything before the next slash.
	trimmed := strings.TrimPrefix(path, "/")
	if idx := strings.Index(trimmed, "/"); idx > 0 {
		return trimmed[:idx]
	}
	return trimmed
}
