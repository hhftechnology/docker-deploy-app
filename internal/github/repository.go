package github

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"docker-deploy-app/internal/models"
)

// RepositoryService handles GitHub repository operations
type RepositoryService struct {
	client *Client
	db     *sql.DB
}

// NewRepositoryService creates a new repository service
func NewRepositoryService(client *Client, db *sql.DB) *RepositoryService {
	return &RepositoryService{
		client: client,
		db:     db,
	}
}

// DiscoverTemplates discovers Docker Compose templates from repositories
func (rs *RepositoryService) DiscoverTemplates() error {
	// Get user repositories
	repos, err := rs.client.ListRepositories(1, 100)
	if err != nil {
		return fmt.Errorf("failed to list repositories: %w", err)
	}

	for _, repo := range repos {
		if err := rs.processRepository(repo); err != nil {
			fmt.Printf("Failed to process repository %s: %v\n", repo.FullName, err)
		}
	}

	return nil
}

// processRepository processes a single repository for templates
func (rs *RepositoryService) processRepository(repo *Repository) error {
	// Check if repository contains docker-compose files
	isDockerRepo, err := rs.client.IsDockerComposeRepo(parseOwnerRepo(repo.FullName))
	if err != nil || !isDockerRepo {
		return nil // Skip repositories without docker-compose
	}

	// Try to get template configuration
	owner, repoName := parseOwnerRepo(repo.FullName)
	templateConfig, err := rs.client.GetTemplateConfig(owner, repoName, repo.DefaultBranch)
	if err != nil {
		// Create default template config
		templateConfig = rs.createDefaultTemplateConfig(repo)
	}

	// Create or update template
	template := rs.buildTemplate(repo, templateConfig)
	return rs.saveTemplate(template)
}

// createDefaultTemplateConfig creates default template configuration
func (rs *RepositoryService) createDefaultTemplateConfig(repo *Repository) map[string]interface{} {
	// Determine category from repository name/description
	category := rs.guessCategory(repo.Name, repo.Description)
	
	// Extract tags from topics and language
	tags := append(repo.Topics, strings.ToLower(repo.Language))
	if category != "" {
		tags = append(tags, category)
	}

	return map[string]interface{}{
		"name":        repo.Name,
		"description": repo.Description,
		"category":    category,
		"tags":        tags,
		"variables":   []interface{}{},
		"icon":        rs.getDefaultIcon(category),
		"version":     "1.0.0",
	}
}

// buildTemplate builds a template from repository and config
func (rs *RepositoryService) buildTemplate(repo *Repository, config map[string]interface{}) *models.Template {
	template := &models.Template{
		ID:          rs.generateTemplateID(repo.FullName),
		RepoURL:     repo.CloneURL,
		Branch:      repo.DefaultBranch,
		Path:        "/",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		RequiresNewt: true, // Default to true for newt injection
	}

	// Set fields from config
	if name, ok := config["name"].(string); ok {
		template.Name = name
	} else {
		template.Name = repo.Name
	}

	if description, ok := config["description"].(string); ok {
		template.Description = description
	} else {
		template.Description = repo.Description
	}

	if category, ok := config["category"].(string); ok {
		template.Category = category
	}

	if icon, ok := config["icon"].(string); ok {
		template.Icon = icon
	}

	if version, ok := config["version"].(string); ok {
		template.Version = version
	}

	// Handle tags
	if tags, ok := config["tags"].([]interface{}); ok {
		for _, tag := range tags {
			if tagStr, ok := tag.(string); ok {
				template.Tags = append(template.Tags, tagStr)
			}
		}
	}

	// Handle variables
	if variables, ok := config["variables"].([]interface{}); ok {
		for _, variable := range variables {
			if varMap, ok := variable.(map[string]interface{}); ok {
				envVar := models.EnvVar{}
				
				if name, ok := varMap["name"].(string); ok {
					envVar.Name = name
				}
				if description, ok := varMap["description"].(string); ok {
					envVar.Description = description
				}
				if defaultVal, ok := varMap["default"].(string); ok {
					envVar.Default = defaultVal
				}
				if required, ok := varMap["required"].(bool); ok {
					envVar.Required = required
				}
				if varType, ok := varMap["type"].(string); ok {
					envVar.Type = varType
				}

				template.Variables = append(template.Variables, envVar)
			}
		}
	}

	// Handle newt configuration
	if newtConfig, ok := config["newt"].(map[string]interface{}); ok {
		template.NewtConfig = &models.NewtRequirements{}
		
		if minVersion, ok := newtConfig["min_version"].(string); ok {
			template.NewtConfig.MinVersion = minVersion
		}
		if features, ok := newtConfig["features"].([]interface{}); ok {
			for _, feature := range features {
				if featureStr, ok := feature.(string); ok {
					template.NewtConfig.Features = append(template.NewtConfig.Features, featureStr)
				}
			}
		}
		if networks, ok := newtConfig["networks"].([]interface{}); ok {
			for _, network := range networks {
				if networkStr, ok := network.(string); ok {
					template.NewtConfig.Networks = append(template.NewtConfig.Networks, networkStr)
				}
			}
		}
	}

	// Set publisher info
	owner, _ := parseOwnerRepo(repo.FullName)
	template.PublisherID = owner
	template.IsVerified = rs.isVerifiedPublisher(owner)

	return template
}

// saveTemplate saves or updates a template in the database
func (rs *RepositoryService) saveTemplate(template *models.Template) error {
	// Check if template already exists
	var exists bool
	err := rs.db.QueryRow("SELECT EXISTS(SELECT 1 FROM templates WHERE id = $1)", template.ID).Scan(&exists)
	if err != nil {
		return err
	}

	// Marshal JSON fields
	tagsJSON, _ := template.MarshalTags()
	variablesJSON, _ := template.MarshalVariables()
	newtConfigJSON, _ := template.MarshalNewtConfig()

	if exists {
		// Update existing template
		_, err = rs.db.Exec(`
			UPDATE templates SET 
				name = $1, description = $2, icon = $3, category = $4, tags = $5,
				repo_url = $6, branch = $7, path = $8, version = $9, variables = $10,
				requires_newt = $11, newt_config = $12, publisher_id = $13, is_verified = $14,
				updated_at = $15
			WHERE id = $16`,
			template.Name, template.Description, template.Icon, template.Category, tagsJSON,
			template.RepoURL, template.Branch, template.Path, template.Version, variablesJSON,
			template.RequiresNewt, newtConfigJSON, template.PublisherID, template.IsVerified,
			template.UpdatedAt, template.ID)
	} else {
		// Insert new template
		_, err = rs.db.Exec(`
			INSERT INTO templates (
				id, name, description, icon, category, tags, repo_url, branch, path, version,
				variables, requires_newt, newt_config, publisher_id, is_verified, created_at, updated_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
			template.ID, template.Name, template.Description, template.Icon, template.Category, tagsJSON,
			template.RepoURL, template.Branch, template.Path, template.Version, variablesJSON,
			template.RequiresNewt, newtConfigJSON, template.PublisherID, template.IsVerified,
			template.CreatedAt, template.UpdatedAt)
	}

	return err
}

// SyncRepository syncs a specific repository
func (rs *RepositoryService) SyncRepository(repoURL string) error {
	owner, repoName, err := ParseRepoURL(repoURL)
	if err != nil {
		return err
	}

	repo, err := rs.client.GetRepository(owner, repoName)
	if err != nil {
		return err
	}

	return rs.processRepository(repo)
}

// GetDockerComposeContent gets docker-compose file content
func (rs *RepositoryService) GetDockerComposeContent(templateID string) ([]byte, error) {
	// Get template info
	var repoURL, branch, path string
	err := rs.db.QueryRow(`
		SELECT repo_url, branch, path 
		FROM templates WHERE id = $1`, templateID).Scan(&repoURL, &branch, &path)
	
	if err != nil {
		return nil, err
	}

	owner, repoName, err := ParseRepoURL(repoURL)
	if err != nil {
		return nil, err
	}

	// Try different compose file names
	composeFiles := []string{
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yml",
		"compose.yaml",
	}

	for _, filename := range composeFiles {
		filePath := filename
		if path != "/" {
			filePath = strings.TrimSuffix(path, "/") + "/" + filename
		}

		content, err := rs.client.GetRawFileContent(owner, repoName, filePath, branch)
		if err == nil {
			return content, nil
		}
	}

	return nil, fmt.Errorf("no docker-compose file found")
}

// Helper functions

func (rs *RepositoryService) generateTemplateID(fullName string) string {
	// Use repository full name as template ID, replacing special characters
	id := strings.ToLower(fullName)
	id = strings.ReplaceAll(id, "/", "-")
	id = strings.ReplaceAll(id, "_", "-")
	return id
}

func (rs *RepositoryService) guessCategory(name, description string) string {
	text := strings.ToLower(name + " " + description)

	categories := map[string][]string{
		"web": {"web", "website", "frontend", "react", "vue", "angular", "nextjs", "nuxt", "nginx", "apache"},
		"database": {"database", "db", "mysql", "postgres", "mongodb", "redis", "elasticsearch"},
		"monitoring": {"monitoring", "metrics", "grafana", "prometheus", "alertmanager", "jaeger"},
		"development": {"dev", "development", "testing", "ci", "cd", "jenkins", "gitlab"},
		"networking": {"network", "proxy", "load-balancer", "traefik", "caddy", "haproxy"},
		"security": {"security", "auth", "oauth", "keycloak", "vault", "ssl", "cert"},
		"analytics": {"analytics", "data", "spark", "kafka", "elastic", "kibana"},
		"ai-ml": {"ai", "ml", "machine-learning", "tensorflow", "pytorch", "jupyter"},
	}

	for category, keywords := range categories {
		for _, keyword := range keywords {
			if strings.Contains(text, keyword) {
				return category
			}
		}
	}

	return "web" // Default category
}

func (rs *RepositoryService) getDefaultIcon(category string) string {
	icons := map[string]string{
		"web":         "🌐",
		"database":    "🗄️",
		"monitoring":  "📊",
		"development": "🛠️",
		"networking":  "🌐",
		"security":    "🔒",
		"analytics":   "📈",
		"ai-ml":       "🤖",
	}

	if icon, exists := icons[category]; exists {
		return icon
	}
	return "📦"
}

func (rs *RepositoryService) isVerifiedPublisher(publisher string) bool {
	verifiedPublishers := []string{
		"docker",
		"bitnami",
		"linuxserver",
		"nextcloud",
		"wordpress",
		"ghost",
		"grafana",
		"prometheus",
		"elastic",
	}

	publisher = strings.ToLower(publisher)
	for _, verified := range verifiedPublishers {
		if publisher == verified {
			return true
		}
	}

	return false
}

func parseOwnerRepo(fullName string) (string, string) {
	parts := strings.Split(fullName, "/")
	if len(parts) >= 2 {
		return parts[0], parts[1]
	}
	return "", ""
}

// CleanupDeletedRepositories removes templates for repositories that no longer exist
func (rs *RepositoryService) CleanupDeletedRepositories() error {
	// Get all templates
	rows, err := rs.db.Query("SELECT id, repo_url FROM templates")
	if err != nil {
		return err
	}
	defer rows.Close()

	var templatesToDelete []string

	for rows.Next() {
		var templateID, repoURL string
		if err := rows.Scan(&templateID, &repoURL); err != nil {
			continue
		}

		owner, repoName, err := ParseRepoURL(repoURL)
		if err != nil {
			continue
		}

		// Check if repository still exists
		_, err = rs.client.GetRepository(owner, repoName)
		if err != nil {
			// Repository doesn't exist or is inaccessible
			templatesToDelete = append(templatesToDelete, templateID)
		}
	}

	// Delete templates for non-existent repositories
	for _, templateID := range templatesToDelete {
		_, err := rs.db.Exec("DELETE FROM templates WHERE id = $1", templateID)
		if err != nil {
			fmt.Printf("Failed to delete template %s: %v\n", templateID, err)
		}
	}

	return nil
}