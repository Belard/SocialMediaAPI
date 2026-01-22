package middleware

import (
	"SocialMediaAPI/services"
	"SocialMediaAPI/utils"
	"context"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

func AuthMiddleware(authService *services.AuthService) mux.MiddlewareFunc {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				utils.RespondWithError(w, http.StatusUnauthorized, "Missing authorization header")
				return
			}

			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				utils.RespondWithError(w, http.StatusUnauthorized, "Invalid authorization header")
				return
			}

			claims, err := authService.ValidateToken(parts[1])
			if err != nil {
				utils.RespondWithError(w, http.StatusUnauthorized, "Invalid token")
				return
			}

			ctx := context.WithValue(r.Context(), "userID", claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}