package middleware

import (
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

// CORSConfig holds the allowed origins, methods, and headers for CORS.
type CORSConfig struct {
	// AllowedOrigins is the set of origins permitted to make cross-origin
	// requests. Use ["*"] to allow any origin (not recommended in production
	// with credentials).
	AllowedOrigins []string

	// AllowedMethods lists the HTTP methods the client may use.
	AllowedMethods []string

	// AllowedHeaders lists the request headers the client may send.
	AllowedHeaders []string

	// AllowCredentials indicates whether the browser should include
	// credentials (cookies, Authorization header, TLS client certs) in
	// cross-origin requests.
	AllowCredentials bool

	// MaxAge is the value of Access-Control-Max-Age in seconds.
	// Browsers cache the preflight response for this duration.
	MaxAge string
}

// DefaultCORSConfig returns a sensible production default.
// AllowedOrigins is intentionally empty – callers MUST set it.
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization", "X-Requested-With"},
		AllowCredentials: true,
		MaxAge:           "86400", // 24 hours
	}
}

// CORS returns gorilla/mux middleware that sets appropriate CORS headers
// and handles preflight OPTIONS requests.
//
// Behaviour:
//   - If AllowedOrigins contains "*", every origin is reflected back (open
//     CORS). This disables AllowCredentials automatically — browsers refuse
//     credentials with a wildcard origin.
//   - Otherwise, only origins present in the allow-list are reflected.
//   - Preflight (OPTIONS) requests receive a 204 No Content immediately.
func CORS(cfg CORSConfig) mux.MiddlewareFunc {
	// Pre-compute a lookup set for O(1) origin checks.
	allowAll := false
	originSet := make(map[string]bool, len(cfg.AllowedOrigins))
	for _, o := range cfg.AllowedOrigins {
		if o == "*" {
			allowAll = true
		}
		originSet[strings.TrimRight(o, "/")] = true
	}

	methods := strings.Join(cfg.AllowedMethods, ", ")
	headers := strings.Join(cfg.AllowedHeaders, ", ")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Only set CORS headers when an Origin header is present
			// (same-origin requests and non-browser clients omit it).
			if origin != "" {
				normalised := strings.TrimRight(origin, "/")

				if allowAll {
					// Wildcard: reflect the exact origin (required for
					// browsers to accept the response).
					w.Header().Set("Access-Control-Allow-Origin", origin)
					// Credentials cannot be used with wildcard origins,
					// so we intentionally do NOT set Allow-Credentials.
				} else if originSet[normalised] {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					if cfg.AllowCredentials {
						w.Header().Set("Access-Control-Allow-Credentials", "true")
					}
				} else {
					// Origin not allowed – skip CORS headers entirely.
					// The browser will block the response on the client side.
					if r.Method == http.MethodOptions {
						// Still respond to the preflight so the TCP
						// connection is closed cleanly.
						w.WriteHeader(http.StatusForbidden)
						return
					}
					next.ServeHTTP(w, r)
					return
				}

				w.Header().Set("Access-Control-Allow-Methods", methods)
				w.Header().Set("Access-Control-Allow-Headers", headers)
				if cfg.MaxAge != "" {
					w.Header().Set("Access-Control-Max-Age", cfg.MaxAge)
				}
				// Tell caches the response varies by Origin.
				w.Header().Add("Vary", "Origin")
			}

			// Handle preflight
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
