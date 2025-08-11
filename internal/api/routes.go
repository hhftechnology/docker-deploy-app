package api

import (
	"database/sql"
	"net/http"

	"github.com/docker/docker/client"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"docker-deploy-app/internal/api/handlers"
	apiMiddleware "docker-deploy-app/internal/api/middleware"
	"docker-deploy-app/internal/config"
)

// Handler holds all dependencies for API handlers
type Handler struct {
	DB           *sql.DB
	DockerClient *client.Client
	Config       *config.Config
	
	// Individual handlers
	Templates   *handlers.TemplatesHandler
	Deployments *handlers.DeploymentsHandler
	Stacks      *handlers.StacksHandler
	Backups     *handlers.BackupsHandler
	Newt        *handlers.NewtHandler
	GitHub      *handlers.GitHubHandler
}

// NewHandler creates a new API handler with all dependencies
func NewHandler(db *sql.DB, dockerClient *client.Client, cfg *config.Config) *Handler {
	return &Handler{
		DB:           db,
		DockerClient: dockerClient,
		Config:       cfg,
		Templates:    handlers.NewTemplatesHandler(db, cfg),
		Deployments:  handlers.NewDeploymentsHandler(db, dockerClient, cfg),
		Stacks:       handlers.NewStacksHandler(db, dockerClient, cfg),
		Backups:      handlers.NewBackupsHandler(db, cfg),
		Newt:         handlers.NewNewtHandler(db, cfg),
		GitHub:       handlers.NewGitHubHandler(db, cfg),
	}
}

// SetupRoutes configures all API routes
func SetupRoutes(r chi.Router, h *Handler) {
	// API middleware
	r.Route("/api", func(r chi.Router) {
		// Common middleware for all API routes
		r.Use(middleware.Timeout(60 * time.Second))
		r.Use(apiMiddleware.JSONContentType)
		
		// Rate limiting if enabled
		if h.Config.Security.RateLimiting.Enabled {
			r.Use(apiMiddleware.RateLimit(h.Config.Security.RateLimiting.RequestsPerMinute))
		}

		// Authentication middleware if enabled
		if h.Config.Security.AuthEnabled {
			r.Use(apiMiddleware.Authentication(h.DB, h.Config.Security.APIKey))
		}

		// Health check endpoint (no auth required)
		r.Get("/health", h.handleHealth)

		// Template Marketplace routes
		r.Route("/marketplace", func(r chi.Router) {
			r.Get("/templates", h.Templates.ListMarketplaceTemplates)
			r.Get("/featured", h.Templates.GetFeaturedTemplates)
			r.Get("/trending", h.Templates.GetTrendingTemplates)
			r.Get("/top-rated", h.Templates.GetTopRatedTemplates)
			r.Get("/categories", h.Templates.GetCategories)
			r.Get("/search", h.Templates.SearchTemplates)
		})

		// Templates routes
		r.Route("/templates", func(r chi.Router) {
			r.Get("/", h.Templates.List)
			r.Get("/{id}", h.Templates.Get)
			r.Get("/{id}/preview", h.Templates.Preview)
			r.Post("/{id}/validate", h.Templates.Validate)
			r.Get("/{id}/versions", h.Templates.GetVersions)
			r.Post("/{id}/rate", h.Templates.Rate)
			r.Get("/{id}/reviews", h.Templates.GetReviews)
			r.Post("/{id}/review", h.Templates.SubmitReview)
			r.Post("/sync", h.Templates.Sync)
		})

		// Deployments routes
		r.Route("/deployments", func(r chi.Router) {
			r.Get("/", h.Deployments.List)
			r.Post("/", h.Deployments.Create)
			r.Get("/{id}", h.Deployments.Get)
			r.Delete("/{id}", h.Deployments.Delete)
			r.Get("/{id}/logs", h.Deployments.GetLogs)
			r.Get("/{id}/logs/stream", h.Deployments.StreamLogs)
			r.Get("/{id}/tunnel", h.Deployments.GetTunnelInfo)
			r.Post("/{id}/backup", h.Deployments.CreateBackup)
		})

		// Stacks routes
		r.Route("/stacks", func(r chi.Router) {
			r.Get("/", h.Stacks.List)
			r.Get("/{id}", h.Stacks.Get)
			r.Post("/{id}/start", h.Stacks.Start)
			r.Post("/{id}/stop", h.Stacks.Stop)
			r.Post("/{id}/restart", h.Stacks.Restart)
			r.Get("/{id}/logs", h.Stacks.GetLogs)
			r.Get("/{id}/logs/stream", h.Stacks.StreamLogs)
			r.Get("/{id}/stats", h.Stacks.GetStats)
			r.Get("/{id}/newt-status", h.Stacks.GetNewtStatus)
			r.Post("/{id}/export", h.Stacks.Export)
		})

		// Backups & Restore routes
		r.Route("/backups", func(r chi.Router) {
			r.Get("/", h.Backups.List)
			r.Post("/", h.Backups.Create)
			r.Get("/{id}", h.Backups.Get)
			r.Delete("/{id}", h.Backups.Delete)
			r.Post("/{id}/restore", h.Backups.Restore)
			r.Get("/{id}/download", h.Backups.Download)
			r.Post("/upload", h.Backups.Upload)
			r.Post("/test-restore", h.Backups.TestRestore)
			
			// Backup schedules
			r.Route("/schedules", func(r chi.Router) {
				r.Get("/", h.Backups.ListSchedules)
				r.Post("/", h.Backups.CreateSchedule)
				r.Put("/{id}", h.Backups.UpdateSchedule)
				r.Delete("/{id}", h.Backups.DeleteSchedule)
			})
		})

		// Newt configuration routes
		r.Route("/newt", func(r chi.Router) {
			r.Get("/config", h.Newt.GetConfig)
			r.Post("/config", h.Newt.UpdateConfig)
			r.Post("/validate", h.Newt.ValidateConfig)
			r.Get("/status", h.Newt.GetStatus)
			r.Post("/test-connection", h.Newt.TestConnection)
		})

		// GitHub integration routes
		r.Route("/github", func(r chi.Router) {
			r.Post("/connect", h.GitHub.Connect)
			r.Get("/repos", h.GitHub.ListRepositories)
			r.Post("/webhook", h.GitHub.HandleWebhook)
			r.Post("/sync", h.GitHub.SyncRepositories)
		})

		// WebSocket endpoints
		r.Route("/ws", func(r chi.Router) {
			// Remove rate limiting for WebSocket connections
			r.Use(apiMiddleware.RemoveRateLimit)
			r.Get("/deployments/{id}/logs", h.Deployments.WebSocketLogs)
			r.Get("/stacks/{id}/logs", h.Stacks.WebSocketLogs)
			r.Get("/system/events", h.handleSystemEvents)
		})

		// Admin routes (require admin role)
		r.Route("/admin", func(r chi.Router) {
			r.Use(apiMiddleware.RequireRole("admin"))
			
			r.Route("/users", func(r chi.Router) {
				r.Get("/", h.handleListUsers)
				r.Post("/", h.handleCreateUser)
				r.Get("/{id}", h.handleGetUser)
				r.Put("/{id}", h.handleUpdateUser)
				r.Delete("/{id}", h.handleDeleteUser)
			})
			
			r.Route("/system", func(r chi.Router) {
				r.Get("/info", h.handleSystemInfo)
				r.Get("/stats", h.handleSystemStats)
				r.Post("/cleanup", h.handleSystemCleanup)
			})
		})
	})
}

// handleHealth returns system health status
func (h *Handler) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Check database connection
	if err := h.DB.Ping(); err != nil {
		http.Error(w, "Database connection failed", http.StatusServiceUnavailable)
		return
	}

	// Check Docker connection
	if _, err := h.DockerClient.Ping(r.Context()); err != nil {
		http.Error(w, "Docker connection failed", http.StatusServiceUnavailable)
		return
	}

	response := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
		"services": map[string]string{
			"database": "healthy",
			"docker":   "healthy",
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleSystemEvents handles WebSocket connections for system events
func (h *Handler) handleSystemEvents(w http.ResponseWriter, r *http.Request) {
	// Upgrade to WebSocket connection
	// Implementation would use gorilla/websocket
	// This is a placeholder for the WebSocket handler
	http.Error(w, "WebSocket system events not implemented", http.StatusNotImplemented)
}

// handleListUsers lists all users (admin only)
func (h *Handler) handleListUsers(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "User management not implemented", http.StatusNotImplemented)
}

// handleCreateUser creates a new user (admin only)
func (h *Handler) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "User management not implemented", http.StatusNotImplemented)
}

// handleGetUser gets a specific user (admin only)
func (h *Handler) handleGetUser(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "User management not implemented", http.StatusNotImplemented)
}

// handleUpdateUser updates a user (admin only)
func (h *Handler) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "User management not implemented", http.StatusNotImplemented)
}

// handleDeleteUser deletes a user (admin only)
func (h *Handler) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "User management not implemented", http.StatusNotImplemented)
}

// handleSystemInfo returns system information (admin only)
func (h *Handler) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]interface{}{
		"version":     "1.0.0",
		"go_version":  runtime.Version(),
		"os":          runtime.GOOS,
		"arch":        runtime.GOARCH,
		"started_at":  time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// handleSystemStats returns system statistics (admin only)
func (h *Handler) handleSystemStats(w http.ResponseWriter, r *http.Request) {
	// Get database stats
	var templateCount, deploymentCount, backupCount int
	h.DB.QueryRow("SELECT COUNT(*) FROM templates").Scan(&templateCount)
	h.DB.QueryRow("SELECT COUNT(*) FROM deployments").Scan(&deploymentCount)
	h.DB.QueryRow("SELECT COUNT(*) FROM backups").Scan(&backupCount)

	stats := map[string]interface{}{
		"templates":   templateCount,
		"deployments": deploymentCount,
		"backups":     backupCount,
		"uptime":      time.Since(time.Now()).String(), // Placeholder
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleSystemCleanup performs system cleanup (admin only)
func (h *Handler) handleSystemCleanup(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "System cleanup not implemented", http.StatusNotImplemented)
}