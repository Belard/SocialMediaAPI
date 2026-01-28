package main

import (
	"fmt"
	"log"
	"net/http"

	"SocialMediaAPI/config"
	"SocialMediaAPI/database"
	"SocialMediaAPI/handlers"
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

	storage, err := services.NewStorageService(cfg.UploadDir, cfg.BaseURL)
	if err != nil {
		log.Fatal("Failed to initialize storage:", err)
	}

	authService := services.NewAuthService(db)
	publisher := services.NewPublisherService(db)
	oauthStateService := services.NewOAuthStateService()
	
	scheduler := services.NewScheduler(db, publisher)
	scheduler.Start()
	defer scheduler.Stop()

	handler := handlers.NewHandler(db, publisher, authService, storage, oauthStateService)

	r := setupRoutes(handler, authService)

	log.Printf("Server starting on port %s...", cfg.Port)
	log.Printf("Upload directory: %s", cfg.UploadDir)
	printEndpoints()

	if err := http.ListenAndServe(":"+cfg.Port, r); err != nil {
		log.Fatal(err)
	}
}

func setupRoutes(h *handlers.Handler, authService *services.AuthService) *mux.Router {
	r := mux.NewRouter()

	// Public routes
	r.HandleFunc("/health", h.HealthCheck).Methods("GET")
	r.HandleFunc("/api/auth/register", h.Register).Methods("POST")
	r.HandleFunc("/api/auth/login", h.Login).Methods("POST")

	// OAuth routes (public - no JWT required for callback)
	r.HandleFunc("/auth/facebook/callback", h.HandleFacebookCallback).Methods("GET")
	
	r.HandleFunc("/oauth/success", func(w http.ResponseWriter, r *http.Request) {
		platform := r.URL.Query().Get("platform")
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fmt.Sprintf(`
			<!DOCTYPE html>
			<html>
			<head>
				<title>OAuth Success</title>
				<style>
					body {
						font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
						display: flex;
						justify-content: center;
						align-items: center;
						height: 100vh;
						margin: 0;
						background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%);
					}
					.container {
						background: white;
						padding: 40px;
						border-radius: 12px;
						box-shadow: 0 10px 40px rgba(0,0,0,0.1);
						text-align: center;
						max-width: 400px;
					}
					h1 { color: #2d3748; margin-bottom: 10px; }
					.success-icon {
						font-size: 64px;
						margin-bottom: 20px;
					}
					p { color: #718096; }
				</style>
			</head>
			<body>
				<div class="container">
					<div class="success-icon">✅</div>
					<h1>Successfully Connected!</h1>
					<p>Your %s account has been connected.</p>
					<p style="font-size: 14px; margin-top: 20px;">You can close this window now.</p>
				</div>
				<script>
					if (window.opener) {
						window.opener.postMessage({type: 'oauth_success', platform: '%s'}, '*');
						setTimeout(() => window.close(), 3000);
					}
				</script>
			</body>
			</html>
		`, platform, platform)))
	}).Methods("GET")
	
	r.HandleFunc("/oauth/error", func(w http.ResponseWriter, r *http.Request) {
		errorType := r.URL.Query().Get("error")
		description := r.URL.Query().Get("description")
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(fmt.Sprintf(`
			<!DOCTYPE html>
			<html>
			<head>
				<title>OAuth Error</title>
				<style>
					body {
						font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
						display: flex;
						justify-content: center;
						align-items: center;
						height: 100vh;
						margin: 0;
						background: linear-gradient(135deg, #f093fb 0%%, #f5576c 100%%);
					}
					.container {
						background: white;
						padding: 40px;
						border-radius: 12px;
						box-shadow: 0 10px 40px rgba(0,0,0,0.1);
						text-align: center;
						max-width: 400px;
					}
					h1 { color: #e53e3e; margin-bottom: 10px; }
					.error-icon {
						font-size: 64px;
						margin-bottom: 20px;
					}
					p { color: #718096; }
					.error-details {
						background: #fed7d7;
						padding: 15px;
						border-radius: 6px;
						margin-top: 20px;
						font-size: 14px;
						color: #c53030;
					}
				</style>
			</head>
			<body>
				<div class="container">
					<div class="error-icon">❌</div>
					<h1>Connection Failed</h1>
					<p>There was a problem connecting your account.</p>
					<div class="error-details">
						<strong>Error:</strong> %s<br>
						<strong>Details:</strong> %s
					</div>
					<p style="font-size: 14px; margin-top: 20px;">Please try again or contact support.</p>
				</div>
				<script>
					setTimeout(() => window.close(), 5000);
				</script>
			</body>
			</html>
		`, errorType, description)))
	}).Methods("GET")

	// Static file serving
	uploadDir := config.Load().UploadDir
	r.PathPrefix("/uploads/").Handler(http.StripPrefix("/uploads/", 
		http.FileServer(http.Dir(uploadDir))))

	// Protected routes
	protected := r.PathPrefix("/api").Subrouter()
	protected.Use(middleware.AuthMiddleware(authService))

	// Facebook OAuth (requires JWT)
	protected.HandleFunc("/auth/facebook", h.InitiateFacebookOAuth).Methods("GET")
	
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
	log.Println("  GET    /auth/facebook/callback    - Facebook OAuth callback")
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
