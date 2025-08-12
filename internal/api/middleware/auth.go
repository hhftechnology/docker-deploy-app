package middleware

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"docker-deploy-app/internal/models"
)

type contextKey string

const (
	UserKey contextKey = "user"
)

// Authentication middleware for API key or session-based auth
func Authentication(db *sql.DB, apiKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth for health check
			if r.URL.Path == "/api/health" {
				next.ServeHTTP(w, r)
				return
			}

			user := authenticateRequest(r, db, apiKey)
			if user == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Add user to context
			ctx := context.WithValue(r.Context(), UserKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireRole middleware to check user role
func RequireRole(role string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := getUserFromContext(r.Context())
			if user == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			if !hasRole(user, role) {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func authenticateRequest(r *http.Request, db *sql.DB, systemAPIKey string) *models.User {
	// Try API key authentication first
	apiKey := extractAPIKey(r)
	if apiKey != "" {
		if apiKey == systemAPIKey {
			// System API key - return admin user
			return models.CreateAnonymousUser()
		}
		
		// Check database for API key
		user := authenticateAPIKey(db, apiKey)
		if user != nil {
			return user
		}
	}

	// Try session authentication
	sessionToken := extractSessionToken(r)
	if sessionToken != "" {
		user := authenticateSession(db, sessionToken)
		if user != nil {
			return user
		}
	}

	// No authentication required - return anonymous user
	return models.CreateAnonymousUser()
}

func extractAPIKey(r *http.Request) string {
	// Check Authorization header
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	
	// Check X-API-Key header
	return r.Header.Get("X-API-Key")
}

func extractSessionToken(r *http.Request) string {
	cookie, err := r.Cookie("session_token")
	if err != nil {
		return ""
	}
	return cookie.Value
}

func authenticateAPIKey(db *sql.DB, key string) *models.User {
	var userID string
	var active bool
	var expiresAt sql.NullTime

	err := db.QueryRow(`
		SELECT user_id, active, expires_at 
		FROM api_keys 
		WHERE key = $1`, key).Scan(&userID, &active, &expiresAt)

	if err != nil {
		return nil
	}

	if !active {
		return nil
	}

	if expiresAt.Valid && time.Now().After(expiresAt.Time) {
		return nil
	}

	// Update last used
	db.Exec("UPDATE api_keys SET last_used = $1 WHERE key = $2", time.Now(), key)

	// Get user
	return getUserByID(db, userID)
}

func authenticateSession(db *sql.DB, token string) *models.User {
	var userID string
	var expiresAt time.Time

	err := db.QueryRow(`
		SELECT user_id, expires_at 
		FROM sessions 
		WHERE token = $1`, token).Scan(&userID, &expiresAt)

	if err != nil {
		return nil
	}

	if time.Now().After(expiresAt) {
		// Clean up expired session
		db.Exec("DELETE FROM sessions WHERE token = $1", token)
		return nil
	}

	return getUserByID(db, userID)
}

func getUserByID(db *sql.DB, userID string) *models.User {
	var user models.User
	var lastLogin sql.NullTime

	err := db.QueryRow(`
		SELECT id, username, email, display_name, role, active, last_login, created_at, updated_at
		FROM users WHERE id = $1`, userID).Scan(
		&user.ID, &user.Username, &user.Email, &user.DisplayName,
		&user.Role, &user.Active, &lastLogin, &user.CreatedAt, &user.UpdatedAt)

	if err != nil {
		return nil
	}

	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}

	if !user.Active {
		return nil
	}

	return &user
}

func getUserFromContext(ctx context.Context) *models.User {
	user, ok := ctx.Value(UserKey).(*models.User)
	if !ok {
		return nil
	}
	return user
}

func hasRole(user *models.User, requiredRole string) bool {
	switch requiredRole {
	case "admin":
		return user.Role == models.RoleAdmin
	case "operator":
		return user.Role == models.RoleOperator || user.Role == models.RoleAdmin
	case "viewer":
		return true // All authenticated users can view
	default:
		return false
	}
}