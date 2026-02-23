package middleware

import (
	"SocialMediaAPI/utils"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
)

// visitor tracks token-bucket state for a single IP address.
type visitor struct {
	tokens   float64
	maxBurst float64
	rate     float64 // tokens replenished per second
	lastSeen time.Time
}

// RateLimiter implements a per-IP token-bucket rate limiter.
// Tokens replenish at a fixed rate per second, up to a configurable burst.
// When tokens are exhausted the client receives 429 Too Many Requests.
type RateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     float64 // tokens per second
	burst    float64 // max tokens (burst capacity)
}

// NewRateLimiter creates a rate limiter that allows `rps` sustained requests
// per second with an initial burst capacity of `burst`.
// A background goroutine evicts visitors that haven't been seen for >5 min.
func NewRateLimiter(rps float64, burst float64) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rps,
		burst:    burst,
	}
	go rl.cleanupLoop()
	return rl
}

// cleanupLoop removes stale visitor entries every 3 minutes to prevent
// unbounded memory growth.
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		for ip, v := range rl.visitors {
			if time.Since(v.lastSeen) > 5*time.Minute {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// allow checks whether the visitor identified by `ip` may proceed.
// It replenishes tokens based on elapsed time and consumes one token.
func (rl *RateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[ip]
	if !exists {
		rl.visitors[ip] = &visitor{
			tokens:   rl.burst - 1, // consume one token immediately
			maxBurst: rl.burst,
			rate:     rl.rate,
			lastSeen: time.Now(),
		}
		return true
	}

	// Replenish tokens based on time elapsed since last request
	now := time.Now()
	elapsed := now.Sub(v.lastSeen).Seconds()
	v.tokens += elapsed * v.rate
	if v.tokens > v.maxBurst {
		v.tokens = v.maxBurst
	}
	v.lastSeen = now

	if v.tokens < 1 {
		return false
	}
	v.tokens--
	return true
}

// extractIP returns the client IP from the request.
// It prefers X-Real-IP, then X-Forwarded-For, then the connection
// remote address (with port stripped).
func extractIP(r *http.Request) string {
	// X-Real-IP is typically set by a reverse proxy (nginx, etc.)
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	// X-Forwarded-For can contain a comma-separated chain; take the first.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// First entry is the original client IP
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return xff[:i]
			}
		}
		return xff
	}
	// Fall back to the TCP remote address (strip port)
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// Limit returns gorilla/mux middleware that enforces the rate limit globally.
func (rl *RateLimiter) Limit() mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := extractIP(r)
			if !rl.allow(ip) {
				w.Header().Set("Retry-After", "1")
				utils.RespondWithError(w, http.StatusTooManyRequests, "Rate limit exceeded. Try again later.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// LimitHandler wraps a single http.HandlerFunc with a stricter rate limiter.
// Useful for sensitive endpoints such as login and register.
func (rl *RateLimiter) LimitHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)
		if !rl.allow(ip) {
			w.Header().Set("Retry-After", "5")
			utils.RespondWithError(w, http.StatusTooManyRequests, "Too many attempts. Please slow down.")
			return
		}
		next(w, r)
	}
}
