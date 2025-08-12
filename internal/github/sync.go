package github

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// SyncService handles template synchronization from GitHub
type SyncService struct {
	client    *Client
	db        *sql.DB
	repoSvc   *RepositoryService
	isRunning bool
	mu        sync.RWMutex
	stopChan  chan struct{}
}

// SyncResult represents the result of a sync operation
type SyncResult struct {
	StartTime        time.Time `json:"start_time"`
	EndTime          time.Time `json:"end_time"`
	Duration         string    `json:"duration"`
	RepositoriesFound int      `json:"repositories_found"`
	TemplatesCreated int       `json:"templates_created"`
	TemplatesUpdated int       `json:"templates_updated"`
	TemplatesDeleted int       `json:"templates_deleted"`
	Errors           []string  `json:"errors"`
	Success          bool      `json:"success"`
}

// NewSyncService creates a new sync service
func NewSyncService(client *Client, db *sql.DB) *SyncService {
	return &SyncService{
		client:   client,
		db:       db,
		repoSvc:  NewRepositoryService(client, db),
		stopChan: make(chan struct{}),
	}
}

// StartPeriodicSync starts periodic synchronization
func (ss *SyncService) StartPeriodicSync(interval time.Duration) {
	if ss.IsRunning() {
		return
	}

	ss.mu.Lock()
	ss.isRunning = true
	ss.mu.Unlock()

	go ss.syncLoop(interval)
	log.Printf("Started periodic GitHub sync with interval: %v", interval)
}

// StopPeriodicSync stops periodic synchronization
func (ss *SyncService) StopPeriodicSync() {
	ss.mu.Lock()
	defer ss.mu.Unlock()

	if !ss.isRunning {
		return
	}

	close(ss.stopChan)
	ss.isRunning = false
	log.Println("Stopped periodic GitHub sync")
}

// IsRunning returns true if periodic sync is running
func (ss *SyncService) IsRunning() bool {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	return ss.isRunning
}

// SyncAll performs a full synchronization of all repositories
func (ss *SyncService) SyncAll() (*SyncResult, error) {
	result := &SyncResult{
		StartTime: time.Now(),
		Errors:    []string{},
	}

	log.Println("Starting full GitHub sync...")

	// Get all repositories
	repos, err := ss.getAllRepositories()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Failed to get repositories: %v", err))
		result.Success = false
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime).String()
		return result, err
	}

	result.RepositoriesFound = len(repos)

	// Process each repository
	for _, repo := range repos {
		if err := ss.processRepository(repo, result); err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("Failed to process %s: %v", repo.FullName, err))
		}
	}

	// Cleanup deleted repositories
	deleted, err := ss.cleanupDeletedRepositories()
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("Cleanup failed: %v", err))
	} else {
		result.TemplatesDeleted = deleted
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime).String()
	result.Success = len(result.Errors) == 0

	// Save sync result
	ss.saveSyncResult(result)

	log.Printf("GitHub sync completed: %d repos, %d created, %d updated, %d deleted, %d errors",
		result.RepositoriesFound, result.TemplatesCreated, result.TemplatesUpdated,
		result.TemplatesDeleted, len(result.Errors))

	return result, nil
}

// SyncRepository syncs a specific repository
func (ss *SyncService) SyncRepository(repoURL string) error {
	owner, repoName, err := ParseRepoURL(repoURL)
	if err != nil {
		return err
	}

	repo, err := ss.client.GetRepository(owner, repoName)
	if err != nil {
		return fmt.Errorf("failed to get repository: %w", err)
	}

	result := &SyncResult{
		StartTime: time.Now(),
		Errors:    []string{},
	}

	err = ss.processRepository(repo, result)
	if err != nil {
		return err
	}

	log.Printf("Synced repository %s: %d created, %d updated",
		repo.FullName, result.TemplatesCreated, result.TemplatesUpdated)

	return nil
}

// GetLastSyncResult returns the last sync result
func (ss *SyncService) GetLastSyncResult() (*SyncResult, error) {
	var result SyncResult
	var errorsJSON string

	err := ss.db.QueryRow(`
		SELECT start_time, end_time, duration, repositories_found, templates_created,
		       templates_updated, templates_deleted, errors, success
		FROM sync_results ORDER BY start_time DESC LIMIT 1
	`).Scan(
		&result.StartTime, &result.EndTime, &result.Duration,
		&result.RepositoriesFound, &result.TemplatesCreated,
		&result.TemplatesUpdated, &result.TemplatesDeleted,
		&errorsJSON, &result.Success)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	// Parse errors JSON
	if errorsJSON != "" {
		if err := json.Unmarshal([]byte(errorsJSON), &result.Errors); err != nil {
			result.Errors = []string{}
		}
	}

	return &result, nil
}

// syncLoop runs the periodic sync loop
func (ss *SyncService) syncLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if _, err := ss.SyncAll(); err != nil {
				log.Printf("Periodic sync failed: %v", err)
			}
		case <-ss.stopChan:
			return
		}
	}
}

// getAllRepositories gets all accessible repositories
func (ss *SyncService) getAllRepositories() ([]*Repository, error) {
	var allRepos []*Repository
	page := 1
	perPage := 100

	for {
		repos, err := ss.client.ListRepositories(page, perPage)
		if err != nil {
			return nil, err
		}

		if len(repos) == 0 {
			break
		}

		// Filter for repositories that might contain Docker Compose files
		for _, repo := range repos {
			if ss.shouldProcessRepository(repo) {
				allRepos = append(allRepos, repo)
			}
		}

		if len(repos) < perPage {
			break
		}

		page++
	}

	return allRepos, nil
}

// shouldProcessRepository determines if a repository should be processed
func (ss *SyncService) shouldProcessRepository(repo *Repository) bool {
	// Skip forks unless they have significant changes
	if repo.Fork {
		return false
	}

	// Skip archived repositories
	// Note: Repository struct doesn't have Archived field in our definition
	// In a full implementation, you'd check repo.Archived

	// Check if repository might contain Docker Compose files
	repoName := strings.ToLower(repo.Name)
	description := strings.ToLower(repo.Description)

	// Look for Docker-related keywords
	dockerKeywords := []string{
		"docker", "compose", "container", "deployment",
		"stack", "service", "microservice",
	}

	text := repoName + " " + description
	for _, keyword := range dockerKeywords {
		if strings.Contains(text, keyword) {
			return true
		}
	}

	// Check if it's in a relevant language
	relevantLanguages := []string{
		"Dockerfile", "Shell", "JavaScript", "TypeScript",
		"Python", "Go", "Java", "PHP", "Ruby",
	}

	for _, lang := range relevantLanguages {
		if repo.Language == lang {
			return true
		}
	}

	return false
}

// processRepository processes a single repository
func (ss *SyncService) processRepository(repo *Repository, result *SyncResult) error {
	// Check if repository actually contains docker-compose files
	owner, repoName := parseOwnerRepo(repo.FullName)
	isDockerRepo, err := ss.client.IsDockerComposeRepo(owner, repoName)
	if err != nil {
		return err
	}

	if !isDockerRepo {
		return nil // Skip repositories without docker-compose
	}

	// Check if template already exists
	templateID := ss.generateTemplateID(repo.FullName)
	exists, err := ss.templateExists(templateID)
	if err != nil {
		return err
	}

	// Process the repository
	if err := ss.repoSvc.processRepository(repo); err != nil {
		return err
	}

	// Update counters
	if exists {
		result.TemplatesUpdated++
	} else {
		result.TemplatesCreated++
	}

	return nil
}

// cleanupDeletedRepositories removes templates for deleted repositories
func (ss *SyncService) cleanupDeletedRepositories() (int, error) {
	if err := ss.repoSvc.CleanupDeletedRepositories(); err != nil {
		return 0, err
	}
	// Return 0 as we don't track the count in the repository service
	return 0, nil
}

// templateExists checks if a template exists in the database
func (ss *SyncService) templateExists(templateID string) (bool, error) {
	var exists bool
	err := ss.db.QueryRow("SELECT EXISTS(SELECT 1 FROM templates WHERE id = $1)", templateID).Scan(&exists)
	return exists, err
}

// generateTemplateID generates a template ID from repository full name
func (ss *SyncService) generateTemplateID(fullName string) string {
	id := strings.ToLower(fullName)
	id = strings.ReplaceAll(id, "/", "-")
	id = strings.ReplaceAll(id, "_", "-")
	return id
}

// saveSyncResult saves sync result to database
func (ss *SyncService) saveSyncResult(result *SyncResult) {
	errorsJSON, _ := json.Marshal(result.Errors)

	// Create sync_results table if it doesn't exist
	ss.db.Exec(`
		CREATE TABLE IF NOT EXISTS sync_results (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			start_time DATETIME,
			end_time DATETIME,
			duration TEXT,
			repositories_found INTEGER,
			templates_created INTEGER,
			templates_updated INTEGER,
			templates_deleted INTEGER,
			errors TEXT,
			success BOOLEAN,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)

	// Insert sync result
	_, err := ss.db.Exec(`
		INSERT INTO sync_results (
			start_time, end_time, duration, repositories_found, templates_created,
			templates_updated, templates_deleted, errors, success
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		result.StartTime, result.EndTime, result.Duration,
		result.RepositoriesFound, result.TemplatesCreated,
		result.TemplatesUpdated, result.TemplatesDeleted,
		string(errorsJSON), result.Success)

	if err != nil {
		log.Printf("Failed to save sync result: %v", err)
	}

	// Clean up old sync results (keep last 10)
	ss.db.Exec(`
		DELETE FROM sync_results 
		WHERE id NOT IN (
			SELECT id FROM sync_results 
			ORDER BY start_time DESC 
			LIMIT 10
		)
	`)
}

// GetSyncHistory returns sync history
func (ss *SyncService) GetSyncHistory(limit int) ([]*SyncResult, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := ss.db.Query(`
		SELECT start_time, end_time, duration, repositories_found, templates_created,
		       templates_updated, templates_deleted, errors, success
		FROM sync_results 
		ORDER BY start_time DESC 
		LIMIT $1`, limit)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*SyncResult
	for rows.Next() {
		var result SyncResult
		var errorsJSON string

		err := rows.Scan(
			&result.StartTime, &result.EndTime, &result.Duration,
			&result.RepositoriesFound, &result.TemplatesCreated,
			&result.TemplatesUpdated, &result.TemplatesDeleted,
			&errorsJSON, &result.Success)

		if err != nil {
			continue
		}

		// Parse errors JSON
		if errorsJSON != "" {
			json.Unmarshal([]byte(errorsJSON), &result.Errors)
		}

		results = append(results, &result)
	}

	return results, nil
}