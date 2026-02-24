package main

import (
	"context"
	"crypto/tls"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"SocialMediaAPI/config"
	"SocialMediaAPI/database"
	"SocialMediaAPI/handlers"
	"SocialMediaAPI/handlers/oauth"
	"SocialMediaAPI/middleware"
	"SocialMediaAPI/services"

	"github.com/gorilla/mux"
)

func main() {
	cfg := config.Load()

	db, err := database.NewDatabase(cfg.DatabaseURL)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}

	storage, err := services.NewStorageService(cfg.UploadDir, cfg.BaseURL, cfg.MaxImageUploadSize, cfg.MaxVideoUploadSize)
	if err != nil {
		log.Fatal("Failed to initialize storage:", err)
	}

	authService := services.NewAuthService(db)
	publisher := services.NewPublisherService(db)
	oauthStateService := services.NewOAuthStateService()

	scheduler := services.NewScheduler(db, publisher)
	scheduler.Start()

	handler := handlers.NewHandler(db, publisher, authService, storage)
	oauthHandler := oauth.NewOAuthHandler(db, oauthStateService)

	r := setupRoutes(handler, oauthHandler, authService, cfg)

	log.Printf("Server starting on port %s...", cfg.Port)
	log.Printf("Upload directory: %s", cfg.UploadDir)
	printEndpoints()

	// ── HTTP server with timeouts ───────────────────────────────────
	// ReadTimeout:  max time to read the entire request (headers + body).
	//              Set high enough for large file uploads (100 MB at ~1 MB/s ≈ 100s).
	// WriteTimeout: max time from end of request read to end of response write.
	//              Must cover the longest handler (e.g. publishing to multiple platforms).
	// IdleTimeout:  max time a keep-alive connection sits idle before being closed.
	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  2 * time.Minute,
		WriteTimeout: 2 * time.Minute,
		IdleTimeout:  120 * time.Second,
	}

	if cfg.TLSEnabled {
		srv.TLSConfig = &tls.Config{
			MinVersion: tls.VersionTLS12,
		}
	}

	// Start server in a goroutine so we can listen for shutdown signals.
	go func() {
		var err error
		if cfg.TLSEnabled {
			log.Printf("TLS enabled — listening on https://localhost:%s", cfg.Port)
			err = srv.ListenAndServeTLS(cfg.TLSCertFile, cfg.TLSKeyFile)
		} else {
			log.Printf("TLS disabled — listening on http://localhost:%s", cfg.Port)
			err = srv.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// ── Graceful shutdown ───────────────────────────────────────────
	// Wait for SIGINT (Ctrl+C) or SIGTERM (docker stop / kill).
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("Received signal %s — shutting down gracefully...", sig)

	// Give in-flight requests up to 30 seconds to finish.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	scheduler.Stop()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Fatalf("Forced shutdown: %v", err)
	}
	log.Println("Server stopped cleanly")
}

func setupRoutes(h *handlers.Handler, oh *oauth.OAuthHandler, authService *services.AuthService, cfg *config.Config) *mux.Router {
	r := mux.NewRouter()

	// ── CORS ────────────────────────────────────────────────────────
	corsCfg := middleware.DefaultCORSConfig()
	if len(cfg.CORSAllowedOrigins) > 0 {
		corsCfg.AllowedOrigins = cfg.CORSAllowedOrigins
	} else {
		// Default: allow same-origin only (no origins → no CORS headers).
		// Set CORS_ALLOWED_ORIGINS env var for production frontends.
		log.Println("WARNING: CORS_ALLOWED_ORIGINS is not set — cross-origin requests will be blocked by browsers")
	}
	r.Use(middleware.CORS(corsCfg))

	// ── Global rate limiter (per-IP) ────────────────────────────────
	globalLimiter := middleware.NewRateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst)
	r.Use(globalLimiter.Limit())

	// ── Stricter limiter for auth endpoints ─────────────────────────
	authLimiter := middleware.NewRateLimiter(cfg.AuthRateLimitRPS, cfg.AuthRateLimitBurst)

	// Public routes
	r.HandleFunc("/health", h.HealthCheck).Methods("GET")
	// Body limits: 1 MB for JSON routes, MaxUploadSize for file uploads.
	// Applied per-handler (not globally) so upload routes aren't capped at 1 MB.
	jsonLimit := int64(1 << 20) // 1 MB

	r.HandleFunc("/api/auth/register", middleware.BodyLimitHandler(jsonLimit, authLimiter.LimitHandler(h.Register))).Methods("POST")
	r.HandleFunc("/api/auth/login", middleware.BodyLimitHandler(jsonLimit, authLimiter.LimitHandler(h.Login))).Methods("POST")

	// OAuth routes (public - no JWT required for callback)
	r.HandleFunc("/auth/facebook/callback", oh.HandleFacebookCallback).Methods("GET")
	r.HandleFunc("/auth/instagram/callback", oh.HandleInstagramCallback).Methods("GET")
	r.HandleFunc("/auth/tiktok/callback", oh.HandleTikTokCallback).Methods("GET")
	r.HandleFunc("/auth/twitter/callback", oh.HandleTwitterCallback).Methods("GET")
	r.HandleFunc("/auth/youtube/callback", oh.HandleYouTubeCallback).Methods("GET")

	r.HandleFunc("/oauth/success", oh.OAuthSuccessPage).Methods("GET")
	r.HandleFunc("/oauth/error", oh.OAuthErrorPage).Methods("GET")

	// Static file serving — signed URLs required (HMAC + expiration).
	r.PathPrefix("/uploads/").Handler(http.StripPrefix("/uploads/",
		middleware.SignedFileServer(cfg.UploadDir, cfg.MediaSigningKey, authService)))

	// Protected routes
	protected := r.PathPrefix("/api").Subrouter()
	protected.Use(middleware.AuthMiddleware(authService))

	// OAuth initiation (requires JWT)
	protected.HandleFunc("/auth/facebook", oh.InitiateFacebookOAuth).Methods("GET")
	protected.HandleFunc("/auth/instagram", oh.InitiateInstagramOAuth).Methods("GET")
	protected.HandleFunc("/auth/tiktok", oh.InitiateTikTokOAuth).Methods("GET")
	protected.HandleFunc("/auth/twitter", oh.InitiateTwitterOAuth).Methods("GET")
	protected.HandleFunc("/auth/youtube", oh.InitiateYouTubeOAuth).Methods("GET")

	// Credentials
	protected.HandleFunc("/credentials", middleware.BodyLimitHandler(jsonLimit, h.SaveCredentials)).Methods("POST")
	protected.HandleFunc("/credentials/status", h.GetConnectedPlatforms).Methods("GET")
	protected.HandleFunc("/credentials/disconnect", h.DisconnectPlatform).Methods("DELETE")

	// Media (upload gets a higher body limit to allow large files)
	protected.HandleFunc("/media", middleware.BodyLimitHandler(cfg.MaxUploadSize, h.UploadMedia)).Methods("POST")
	protected.HandleFunc("/media", h.GetMedia).Methods("GET")
	protected.HandleFunc("/media/{id}", h.DeleteMedia).Methods("DELETE")

	// Posts
	protected.HandleFunc("/posts", middleware.BodyLimitHandler(jsonLimit, h.CreatePost)).Methods("POST")
	protected.HandleFunc("/posts", h.GetPosts).Methods("GET")
	protected.HandleFunc("/posts/{id}", h.GetPost).Methods("GET")

	return r
}

func printEndpoints() {
	log.Println("Endpoints available:")
	log.Println("  POST   /api/auth/register          - Register new user")
	log.Println("  POST   /api/auth/login             - Login")
	log.Println("  GET    /api/auth/facebook          - Initiate Facebook OAuth (auth)")
	log.Println("  GET    /api/auth/instagram         - Initiate Instagram OAuth (auth)")
	log.Println("  GET    /api/auth/tiktok            - Initiate TikTok OAuth (auth)")
	log.Println("  GET    /api/auth/twitter           - Initiate Twitter OAuth (auth)")
	log.Println("  GET    /api/auth/youtube           - Initiate YouTube OAuth (auth)")
	log.Println("  GET    /auth/facebook/callback     - Facebook OAuth callback")
	log.Println("  GET    /auth/instagram/callback    - Instagram OAuth callback")
	log.Println("  GET    /auth/tiktok/callback       - TikTok OAuth callback")
	log.Println("  GET    /auth/twitter/callback      - Twitter OAuth callback")
	log.Println("  GET    /auth/youtube/callback      - YouTube OAuth callback")
	log.Println("  GET    /oauth/success              - OAuth success page")
	log.Println("  GET    /oauth/error                - OAuth error page")
	log.Println("  GET    /api/credentials/status     - Get connected platforms (auth)")
	log.Println("  POST   /api/credentials            - Save platform credentials (auth)")
	log.Println("  DELETE /api/credentials/disconnect - Disconnect platform (auth)")
	log.Println("  POST   /api/media                  - Upload media (auth)")
	log.Println("  GET    /api/media                  - Get user media (auth)")
	log.Println("  DELETE /api/media/{id}             - Delete media (auth)")
	log.Println("  POST   /api/posts                  - Create/schedule post (auth)")
	log.Println("  GET    /api/posts                  - Get user posts (auth)")
	log.Println("  GET    /api/posts/{id}             - Get specific post (auth)")
	log.Println("  GET    /health                     - Health check")
	log.Println("  GET    /uploads/*                  - Serve uploaded files")
}
