package main

import (
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
	
	scheduler := services.NewScheduler(db, publisher)
	scheduler.Start()
	defer scheduler.Stop()

	handler := handlers.NewHandler(db, publisher, authService, storage)

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

	// Static file serving
	r.PathPrefix("/uploads/").Handler(http.StripPrefix("/uploads/", 
		http.FileServer(http.Dir(config.Load().UploadDir))))

	// Protected routes
	protected := r.PathPrefix("/api").Subrouter()
	protected.Use(middleware.AuthMiddleware(authService))

	protected.HandleFunc("/media", h.UploadMedia).Methods("POST")
	protected.HandleFunc("/media", h.GetMedia).Methods("GET")
	protected.HandleFunc("/media/{id}", h.DeleteMedia).Methods("DELETE")
	protected.HandleFunc("/posts", h.CreatePost).Methods("POST")
	protected.HandleFunc("/posts", h.GetPosts).Methods("GET")
	protected.HandleFunc("/posts/{id}", h.GetPost).Methods("GET")
	protected.HandleFunc("/credentials", h.SaveCredentials).Methods("POST")

	return r
}

func printEndpoints() {
	log.Println("Endpoints available:")
	log.Println("  POST   /api/auth/register    - Register new user")
	log.Println("  POST   /api/auth/login       - Login")
	log.Println("  POST   /api/media            - Upload media (auth)")
	log.Println("  GET    /api/media            - Get user media (auth)")
	log.Println("  DELETE /api/media/{id}       - Delete media (auth)")
	log.Println("  POST   /api/posts            - Create/schedule post (auth)")
	log.Println("  GET    /api/posts            - Get user posts (auth)")
	log.Println("  GET    /api/posts/{id}       - Get specific post (auth)")
	log.Println("  POST   /api/credentials      - Save credentials (auth)")
	log.Println("  GET    /health               - Health check")
	log.Println("  GET    /uploads/*            - Serve uploaded files")
}
