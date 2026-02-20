package main

import (
	"log"
	"net/http"

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
	defer scheduler.Stop()

	handler := handlers.NewHandler(db, publisher, authService, storage)
	oauthHandler := oauth.NewOAuthHandler(db, oauthStateService)

	r := setupRoutes(handler, oauthHandler, authService)

	log.Printf("Server starting on port %s...", cfg.Port)
	log.Printf("Upload directory: %s", cfg.UploadDir)
	printEndpoints()

	if cfg.TLSEnabled {
		log.Printf("TLS enabled — listening on https://localhost:%s", cfg.Port)
		if err := http.ListenAndServeTLS(":"+cfg.Port, cfg.TLSCertFile, cfg.TLSKeyFile, r); err != nil {
			log.Fatal(err)
		}
	} else {
		log.Printf("TLS disabled — listening on http://localhost:%s", cfg.Port)
		if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
			log.Fatal(err)
		}
	}
}

func setupRoutes(h *handlers.Handler, oh *oauth.OAuthHandler, authService *services.AuthService) *mux.Router {
	r := mux.NewRouter()

	// Public routes
	r.HandleFunc("/health", h.HealthCheck).Methods("GET")
	r.HandleFunc("/api/auth/register", h.Register).Methods("POST")
	r.HandleFunc("/api/auth/login", h.Login).Methods("POST")

	// OAuth routes (public - no JWT required for callback)
	r.HandleFunc("/auth/facebook/callback", oh.HandleFacebookCallback).Methods("GET")
	r.HandleFunc("/auth/instagram/callback", oh.HandleInstagramCallback).Methods("GET")
	r.HandleFunc("/auth/tiktok/callback", oh.HandleTikTokCallback).Methods("GET")

	r.HandleFunc("/oauth/success", oh.OAuthSuccessPage).Methods("GET")
	r.HandleFunc("/oauth/error", oh.OAuthErrorPage).Methods("GET")

	// Static file serving
	uploadDir := config.Load().UploadDir
	r.PathPrefix("/uploads/").Handler(http.StripPrefix("/uploads/",
		http.FileServer(http.Dir(uploadDir))))

	// Protected routes
	protected := r.PathPrefix("/api").Subrouter()
	protected.Use(middleware.AuthMiddleware(authService))

	// OAuth initiation (requires JWT)
	protected.HandleFunc("/auth/facebook", oh.InitiateFacebookOAuth).Methods("GET")
	protected.HandleFunc("/auth/instagram", oh.InitiateInstagramOAuth).Methods("GET")
	protected.HandleFunc("/auth/tiktok", oh.InitiateTikTokOAuth).Methods("GET")

	// Credentials
	protected.HandleFunc("/credentials", h.SaveCredentials).Methods("POST")
	protected.HandleFunc("/credentials/status", h.GetConnectedPlatforms).Methods("GET")
	protected.HandleFunc("/credentials/disconnect", h.DisconnectPlatform).Methods("DELETE")

	// Media
	protected.HandleFunc("/media", h.UploadMedia).Methods("POST")
	protected.HandleFunc("/media", h.GetMedia).Methods("GET")
	protected.HandleFunc("/media/{id}", h.DeleteMedia).Methods("DELETE")

	// Posts
	protected.HandleFunc("/posts", h.CreatePost).Methods("POST")
	protected.HandleFunc("/posts", h.GetPosts).Methods("GET")
	protected.HandleFunc("/posts/{id}", h.GetPost).Methods("GET")

	return r
}

func printEndpoints() {
	log.Println("Endpoints available:")
	log.Println("  POST   /api/auth/register         - Register new user")
	log.Println("  POST   /api/auth/login            - Login")
	log.Println("  GET    /api/auth/facebook         - Initiate Facebook OAuth (auth)")
	log.Println("  GET    /api/auth/instagram        - Initiate Instagram OAuth (auth)")
	log.Println("  GET    /api/auth/tiktok           - Initiate TikTok OAuth (auth)")
	log.Println("  GET    /auth/facebook/callback    - Facebook OAuth callback")
	log.Println("  GET    /auth/instagram/callback   - Instagram OAuth callback")
	log.Println("  GET    /auth/tiktok/callback      - TikTok OAuth callback")
	log.Println("  GET    /oauth/success             - OAuth success page")
	log.Println("  GET    /oauth/error               - OAuth error page")
	log.Println("  GET    /api/credentials/status    - Get connected platforms (auth)")
	log.Println("  POST   /api/credentials           - Save platform credentials (auth)")
	log.Println("  DELETE /api/credentials/disconnect - Disconnect platform (auth)")
	log.Println("  POST   /api/media                 - Upload media (auth)")
	log.Println("  GET    /api/media                 - Get user media (auth)")
	log.Println("  DELETE /api/media/{id}            - Delete media (auth)")
	log.Println("  POST   /api/posts                 - Create/schedule post (auth)")
	log.Println("  GET    /api/posts                 - Get user posts (auth)")
	log.Println("  GET    /api/posts/{id}            - Get specific post (auth)")
	log.Println("  GET    /health                    - Health check")
	log.Println("  GET    /uploads/*                 - Serve uploaded files")
}
