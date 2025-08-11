package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"docker-deploy-app/internal/config"
	"docker-deploy-app/internal/models"
)

// TemplatesHandler handles template-related HTTP requests
type TemplatesHandler struct {
	db     *sql.DB
	config *config.Config
}

// NewTemplatesHandler creates a new templates handler
func NewTemplatesHandler(db *sql.DB, config *config.Config) *TemplatesHandler {
	return &TemplatesHandler{
		db:     db,
		config: config,
	}
}

// List returns all templates
func (h *TemplatesHandler) List(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	verified := r.URL.Query().Get("verified")
	limit := getIntParam(r, "limit", 50)
	offset := getIntParam(r, "offset", 0)

	query := `
		SELECT id, name, description, icon, category, tags, repo_url, branch, path, version,
		       variables, requires_newt, newt_config, publisher_id, is_verified,
		       download_count, avg_rating, total_ratings, created_at, updated_at
		FROM templates WHERE 1=1`
	
	args := []interface{}{}
	argCount := 0

	if category != "" {
		argCount++
		query += fmt.Sprintf(" AND category = $%d", argCount)
		args = append(args, category)
	}

	if verified == "true" {
		argCount++
		query += fmt.Sprintf(" AND is_verified = $%d", argCount)
		args = append(args, true)
	}

	query += " ORDER BY avg_rating DESC, download_count DESC"
	argCount++
	query += fmt.Sprintf(" LIMIT $%d", argCount)
	args = append(args, limit)
	
	argCount++
	query += fmt.Sprintf(" OFFSET $%d", argCount)
	args = append(args, offset)

	rows, err := h.db.Query(query, args...)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var templates []models.Template
	for rows.Next() {
		var t models.Template
		var tagsJSON, variablesJSON, newtConfigJSON string
		
		err := rows.Scan(
			&t.ID, &t.Name, &t.Description, &t.Icon, &t.Category, &tagsJSON,
			&t.RepoURL, &t.Branch, &t.Path, &t.Version, &variablesJSON,
			&t.RequiresNewt, &newtConfigJSON, &t.PublisherID, &t.IsVerified,
			&t.DownloadCount, &t.AvgRating, &t.TotalRatings, &t.CreatedAt, &t.UpdatedAt,
		)
		if err != nil {
			http.Error(w, fmt.Sprintf("Scan error: %v", err), http.StatusInternalServerError)
			return
		}

		// Unmarshal JSON fields
		t.UnmarshalTags(tagsJSON)
		t.UnmarshalVariables(variablesJSON)
		t.UnmarshalNewtConfig(newtConfigJSON)

		templates = append(templates, t)
	}

	response := map[string]interface{}{
		"templates": templates,
		"total":     len(templates),
		"limit":     limit,
		"offset":    offset,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Get returns a specific template by ID
func (h *TemplatesHandler) Get(w http.ResponseWriter, r *http.Request) {
	templateID := chi.URLParam(r, "id")
	if templateID == "" {
		http.Error(w, "Template ID required", http.StatusBadRequest)
		return
	}

	var t models.Template
	var tagsJSON, variablesJSON, newtConfigJSON string

	query := `
		SELECT id, name, description, icon, category, tags, repo_url, branch, path, version,
		       variables, requires_newt, newt_config, publisher_id, is_verified,
		       download_count, avg_rating, total_ratings, created_at, updated_at
		FROM templates WHERE id = $1`

	err := h.db.QueryRow(query, templateID).Scan(
		&t.ID, &t.Name, &t.Description, &t.Icon, &t.Category, &tagsJSON,
		&t.RepoURL, &t.Branch, &t.Path, &t.Version, &variablesJSON,
		&t.RequiresNewt, &newtConfigJSON, &t.PublisherID, &t.IsVerified,
		&t.DownloadCount, &t.AvgRating, &t.TotalRatings, &t.CreatedAt, &t.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		http.Error(w, "Template not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	// Unmarshal JSON fields
	t.UnmarshalTags(tagsJSON)
	t.UnmarshalVariables(variablesJSON)
	t.UnmarshalNewtConfig(newtConfigJSON)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(t)
}

// ListMarketplaceTemplates returns marketplace templates with ratings
func (h *TemplatesHandler) ListMarketplaceTemplates(w http.ResponseWriter, r *http.Request) {
	category := r.URL.Query().Get("category")
	minRating := getFloatParam(r, "min_rating", 0)
	limit := getIntParam(r, "limit", 20)
	
	query := `
		SELECT id, name, description, icon, category, tags, requires_newt, is_verified,
		       download_count, avg_rating, total_ratings
		FROM templates 
		WHERE total_ratings >= $1 AND avg_rating >= $2`
	
	args := []interface{}{h.config.Marketplace.MinRatingsForDisplay, minRating}
	argCount := 2

	if category != "" {
		argCount++
		query += fmt.Sprintf(" AND category = $%d", argCount)
		args = append(args, category)
	}

	query += " ORDER BY avg_rating DESC, total_ratings DESC"
	argCount++
	query += fmt.Sprintf(" LIMIT $%d", argCount)
	args = append(args, limit)

	rows, err := h.db.Query(query, args...)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var templates []map[string]interface{}
	for rows.Next() {
		var t models.Template
		var tagsJSON string
		
		err := rows.Scan(
			&t.ID, &t.Name, &t.Description, &t.Icon, &t.Category, &tagsJSON,
			&t.RequiresNewt, &t.IsVerified, &t.DownloadCount, &t.AvgRating, &t.TotalRatings,
		)
		if err != nil {
			continue
		}

		t.UnmarshalTags(tagsJSON)

		template := map[string]interface{}{
			"id":            t.ID,
			"name":          t.Name,
			"description":   t.Description,
			"icon":          t.Icon,
			"category":      t.Category,
			"tags":          t.Tags,
			"requires_newt": t.RequiresNewt,
			"is_verified":   t.IsVerified,
			"download_count": t.DownloadCount,
			"avg_rating":    t.AvgRating,
			"total_ratings": t.TotalRatings,
			"is_popular":    t.IsPopular(),
		}

		templates = append(templates, template)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"templates": templates,
		"total":     len(templates),
	})
}

// GetFeaturedTemplates returns featured templates
func (h *TemplatesHandler) GetFeaturedTemplates(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT id, name, description, icon, category, tags, requires_newt, is_verified,
		       download_count, avg_rating, total_ratings
		FROM templates 
		WHERE is_verified = true AND avg_rating >= 4.5 AND total_ratings >= 10
		ORDER BY avg_rating DESC, download_count DESC
		LIMIT $1`

	rows, err := h.db.Query(query, h.config.Marketplace.FeaturedTemplateCount)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var templates []models.Template
	for rows.Next() {
		var t models.Template
		var tagsJSON string
		
		err := rows.Scan(
			&t.ID, &t.Name, &t.Description, &t.Icon, &t.Category, &tagsJSON,
			&t.RequiresNewt, &t.IsVerified, &t.DownloadCount, &t.AvgRating, &t.TotalRatings,
		)
		if err != nil {
			continue
		}

		t.UnmarshalTags(tagsJSON)
		templates = append(templates, t)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"templates": templates,
	})
}

// GetTrendingTemplates returns trending templates based on recent activity
func (h *TemplatesHandler) GetTrendingTemplates(w http.ResponseWriter, r *http.Request) {
	days := getIntParam(r, "days", 7)
	limit := getIntParam(r, "limit", 10)

	query := `
		SELECT t.id, t.name, t.description, t.icon, t.category, t.tags, t.requires_newt,
		       t.is_verified, t.download_count, t.avg_rating, t.total_ratings,
		       COUNT(d.id) as recent_deploys
		FROM templates t
		LEFT JOIN deployments d ON t.id = d.template_id 
		    AND d.created_at > datetime('now', '-' || $1 || ' days')
		GROUP BY t.id
		ORDER BY recent_deploys DESC, t.download_count DESC
		LIMIT $2`

	rows, err := h.db.Query(query, days, limit)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var templates []map[string]interface{}
	for rows.Next() {
		var t models.Template
		var tagsJSON string
		var recentDeploys int
		
		err := rows.Scan(
			&t.ID, &t.Name, &t.Description, &t.Icon, &t.Category, &tagsJSON,
			&t.RequiresNewt, &t.IsVerified, &t.DownloadCount, &t.AvgRating,
			&t.TotalRatings, &recentDeploys,
		)
		if err != nil {
			continue
		}

		t.UnmarshalTags(tagsJSON)

		template := map[string]interface{}{
			"id":              t.ID,
			"name":            t.Name,
			"description":     t.Description,
			"icon":            t.Icon,
			"category":        t.Category,
			"tags":            t.Tags,
			"requires_newt":   t.RequiresNewt,
			"is_verified":     t.IsVerified,
			"download_count":  t.DownloadCount,
			"avg_rating":      t.AvgRating,
			"total_ratings":   t.TotalRatings,
			"recent_deploys":  recentDeploys,
		}

		templates = append(templates, template)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"templates": templates,
	})
}

// GetTopRatedTemplates returns top-rated templates
func (h *TemplatesHandler) GetTopRatedTemplates(w http.ResponseWriter, r *http.Request) {
	limit := getIntParam(r, "limit", 10)

	query := `
		SELECT id, name, description, icon, category, tags, requires_newt, is_verified,
		       download_count, avg_rating, total_ratings
		FROM templates 
		WHERE total_ratings >= $1
		ORDER BY avg_rating DESC, total_ratings DESC
		LIMIT $2`

	rows, err := h.db.Query(query, h.config.Marketplace.MinRatingsForDisplay, limit)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var templates []models.Template
	for rows.Next() {
		var t models.Template
		var tagsJSON string
		
		err := rows.Scan(
			&t.ID, &t.Name, &t.Description, &t.Icon, &t.Category, &tagsJSON,
			&t.RequiresNewt, &t.IsVerified, &t.DownloadCount, &t.AvgRating, &t.TotalRatings,
		)
		if err != nil {
			continue
		}

		t.UnmarshalTags(tagsJSON)
		templates = append(templates, t)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"templates": templates,
	})
}

// GetCategories returns all available template categories
func (h *TemplatesHandler) GetCategories(w http.ResponseWriter, r *http.Request) {
	categories := h.config.Marketplace.Categories
	
	// Get count for each category
	categoryStats := make(map[string]int)
	for _, category := range categories {
		var count int
		h.db.QueryRow("SELECT COUNT(*) FROM templates WHERE category = $1", category).Scan(&count)
		categoryStats[category] = count
	}

	response := map[string]interface{}{
		"categories": categories,
		"stats":      categoryStats,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// SearchTemplates searches templates by name, description, or tags
func (h *TemplatesHandler) SearchTemplates(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Search query required", http.StatusBadRequest)
		return
	}

	category := r.URL.Query().Get("category")
	limit := getIntParam(r, "limit", 20)

	searchQuery := `
		SELECT id, name, description, icon, category, tags, requires_newt, is_verified,
		       download_count, avg_rating, total_ratings
		FROM templates 
		WHERE (name LIKE $1 OR description LIKE $1 OR tags LIKE $1)`

	args := []interface{}{"%" + query + "%"}
	argCount := 1

	if category != "" {
		argCount++
		searchQuery += fmt.Sprintf(" AND category = $%d", argCount)
		args = append(args, category)
	}

	searchQuery += " ORDER BY avg_rating DESC, download_count DESC"
	argCount++
	searchQuery += fmt.Sprintf(" LIMIT $%d", argCount)
	args = append(args, limit)

	rows, err := h.db.Query(searchQuery, args...)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var templates []models.Template
	for rows.Next() {
		var t models.Template
		var tagsJSON string
		
		err := rows.Scan(
			&t.ID, &t.Name, &t.Description, &t.Icon, &t.Category, &tagsJSON,
			&t.RequiresNewt, &t.IsVerified, &t.DownloadCount, &t.AvgRating, &t.TotalRatings,
		)
		if err != nil {
			continue
		}

		t.UnmarshalTags(tagsJSON)
		templates = append(templates, t)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"templates": templates,
		"query":     query,
		"total":     len(templates),
	})
}

// Preview returns a preview of the docker-compose.yml with newt injected
func (h *TemplatesHandler) Preview(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Template preview not implemented", http.StatusNotImplemented)
}

// Validate validates a template for newt compatibility
func (h *TemplatesHandler) Validate(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Template validation not implemented", http.StatusNotImplemented)
}

// GetVersions returns version history for a template
func (h *TemplatesHandler) GetVersions(w http.ResponseWriter, r *http.Request) {
	templateID := chi.URLParam(r, "id")
	
	// For now, return current version only
	// In a full implementation, this would track version history
	response := map[string]interface{}{
		"template_id": templateID,
		"versions": []map[string]interface{}{
			{
				"version":    "1.0.0",
				"created_at": "2024-01-01T00:00:00Z",
				"is_current": true,
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Rate submits a rating for a template
func (h *TemplatesHandler) Rate(w http.ResponseWriter, r *http.Request) {
	templateID := chi.URLParam(r, "id")
	
	var req struct {
		UserID string `json:"user_id"`
		Rating int    `json:"rating"`
		Review string `json:"review"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Rating < 1 || req.Rating > 5 {
		http.Error(w, "Rating must be between 1 and 5", http.StatusBadRequest)
		return
	}

	// Insert or update rating
	_, err := h.db.Exec(`
		INSERT OR REPLACE INTO template_ratings 
		(template_id, user_id, rating, review, created_at, updated_at)
		VALUES ($1, $2, $3, $4, datetime('now'), datetime('now'))`,
		templateID, req.UserID, req.Rating, req.Review)

	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Rating submitted successfully",
	})
}

// GetReviews returns reviews for a template
func (h *TemplatesHandler) GetReviews(w http.ResponseWriter, r *http.Request) {
	templateID := chi.URLParam(r, "id")
	limit := getIntParam(r, "limit", 10)
	
	query := `
		SELECT id, user_id, rating, review, helpful_count, created_at
		FROM template_ratings 
		WHERE template_id = $1 AND review != ''
		ORDER BY helpful_count DESC, created_at DESC
		LIMIT $2`

	rows, err := h.db.Query(query, templateID, limit)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var reviews []models.TemplateRating
	for rows.Next() {
		var review models.TemplateRating
		err := rows.Scan(
			&review.ID, &review.UserID, &review.Rating, &review.Review,
			&review.HelpfulCount, &review.CreatedAt,
		)
		if err != nil {
			continue
		}
		reviews = append(reviews, review)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"reviews": reviews,
	})
}

// SubmitReview submits a review for a template
func (h *TemplatesHandler) SubmitReview(w http.ResponseWriter, r *http.Request) {
	// Alias for Rate method since they're the same
	h.Rate(w, r)
}

// Sync synchronizes templates from GitHub
func (h *TemplatesHandler) Sync(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Template sync not implemented", http.StatusNotImplemented)
}

// Helper functions
func getIntParam(r *http.Request, param string, defaultValue int) int {
	value := r.URL.Query().Get(param)
	if value == "" {
		return defaultValue
	}
	if intValue, err := strconv.Atoi(value); err == nil {
		return intValue
	}
	return defaultValue
}

func getFloatParam(r *http.Request, param string, defaultValue float64) float64 {
	value := r.URL.Query().Get(param)
	if value == "" {
		return defaultValue
	}
	if floatValue, err := strconv.ParseFloat(value, 64); err == nil {
		return floatValue
	}
	return defaultValue
}