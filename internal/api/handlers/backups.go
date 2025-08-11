package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	"docker-deploy-app/internal/config"
	"docker-deploy-app/internal/models"
)

// BackupsHandler handles backup-related HTTP requests
type BackupsHandler struct {
	db     *sql.DB
	config *config.Config
}

// NewBackupsHandler creates a new backups handler
func NewBackupsHandler(db *sql.DB, config *config.Config) *BackupsHandler {
	return &BackupsHandler{
		db:     db,
		config: config,
	}
}

// List returns all backups
func (h *BackupsHandler) List(w http.ResponseWriter, r *http.Request) {
	backupType := r.URL.Query().Get("type")
	status := r.URL.Query().Get("status")
	limit := getIntParam(r, "limit", 50)
	offset := getIntParam(r, "offset", 0)

	query := `
		SELECT id, name, type, status, size_bytes, include_volumes, encrypted,
		       storage_path, deployment_ids, created_at, completed_at
		FROM backups WHERE 1=1`

	args := []interface{}{}
	argCount := 0

	if backupType != "" {
		argCount++
		query += fmt.Sprintf(" AND type = $%d", argCount)
		args = append(args, backupType)
	}

	if status != "" {
		argCount++
		query += fmt.Sprintf(" AND status = $%d", argCount)
		args = append(args, status)
	}

	query += " ORDER BY created_at DESC"
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

	var backups []map[string]interface{}
	for rows.Next() {
		var b models.Backup
		var deploymentIDsJSON string
		var completedAt sql.NullTime

		err := rows.Scan(
			&b.ID, &b.Name, &b.Type, &b.Status, &b.SizeBytes, &b.IncludeVolumes,
			&b.Encrypted, &b.StoragePath, &deploymentIDsJSON, &b.CreatedAt, &completedAt,
		)
		if err != nil {
			continue
		}

		if completedAt.Valid {
			b.CompletedAt = &completedAt.Time
		}

		b.UnmarshalDeploymentIDs(deploymentIDsJSON)

		backup := map[string]interface{}{
			"id":               b.ID,
			"name":             b.Name,
			"type":             b.Type,
			"status":           b.Status,
			"size_bytes":       b.SizeBytes,
			"size_formatted":   b.GetFormattedSize(),
			"include_volumes":  b.IncludeVolumes,
			"encrypted":        b.Encrypted,
			"deployment_count": len(b.DeploymentIDs),
			"created_at":       b.CreatedAt,
			"completed_at":     b.CompletedAt,
			"duration":         b.GetDuration(),
			"is_completed":     b.IsCompleted(),
			"is_failed":        b.IsFailed(),
		}

		backups = append(backups, backup)
	}

	response := map[string]interface{}{
		"backups": backups,
		"total":   len(backups),
		"limit":   limit,
		"offset":  offset,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Create creates a new backup
func (h *BackupsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name            string   `json:"name"`
		Type            string   `json:"type"`
		IncludeVolumes  bool     `json:"include_volumes"`
		Encrypted       bool     `json:"encrypted"`
		DeploymentIDs   []string `json:"deployment_ids"`
		AllDeployments  bool     `json:"all_deployments"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Backup name required", http.StatusBadRequest)
		return
	}

	// Get deployment IDs if all_deployments is true
	var deploymentIDs []string
	if req.AllDeployments {
		rows, err := h.db.Query("SELECT id FROM deployments WHERE status = 'running'")
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get deployments: %v", err), http.StatusInternalServerError)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var id string
			rows.Scan(&id)
			deploymentIDs = append(deploymentIDs, id)
		}
	} else {
		deploymentIDs = req.DeploymentIDs
	}

	if len(deploymentIDs) == 0 {
		http.Error(w, "No deployments specified", http.StatusBadRequest)
		return
	}

	// Generate backup ID
	backupID := fmt.Sprintf("backup_%d", time.Now().Unix())

	// Create backup record
	backup := &models.Backup{
		ID:             backupID,
		Name:           req.Name,
		Type:           models.BackupType(req.Type),
		Status:         models.BackupStatusCreating,
		IncludeVolumes: req.IncludeVolumes,
		Encrypted:      req.Encrypted,
		DeploymentIDs:  deploymentIDs,
		CreatedAt:      time.Now(),
	}

	// Set storage path
	backup.StoragePath = filepath.Join(h.config.Backup.Storage.Path, backupID+".tar.gz")

	// Save to database
	deploymentIDsJSON, _ := backup.MarshalDeploymentIDs()
	_, err := h.db.Exec(`
		INSERT INTO backups (id, name, type, status, size_bytes, include_volumes, encrypted,
		                     storage_path, deployment_ids, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		backup.ID, backup.Name, backup.Type, backup.Status, backup.SizeBytes,
		backup.IncludeVolumes, backup.Encrypted, backup.StoragePath,
		deploymentIDsJSON, backup.CreatedAt,
	)

	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create backup: %v", err), http.StatusInternalServerError)
		return
	}

	// Start backup process in background
	go h.performBackup(backup)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":      backup.ID,
		"name":    backup.Name,
		"status":  backup.Status,
		"message": "Backup started",
	})
}

// Get returns a specific backup
func (h *BackupsHandler) Get(w http.ResponseWriter, r *http.Request) {
	backupID := chi.URLParam(r, "id")
	if backupID == "" {
		http.Error(w, "Backup ID required", http.StatusBadRequest)
		return
	}

	var b models.Backup
	var deploymentIDsJSON string
	var completedAt sql.NullTime

	query := `
		SELECT id, name, type, status, size_bytes, include_volumes, encrypted,
		       storage_path, deployment_ids, created_at, completed_at
		FROM backups WHERE id = $1`

	err := h.db.QueryRow(query, backupID).Scan(
		&b.ID, &b.Name, &b.Type, &b.Status, &b.SizeBytes, &b.IncludeVolumes,
		&b.Encrypted, &b.StoragePath, &deploymentIDsJSON, &b.CreatedAt, &completedAt,
	)

	if err == sql.ErrNoRows {
		http.Error(w, "Backup not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	if completedAt.Valid {
		b.CompletedAt = &completedAt.Time
	}

	b.UnmarshalDeploymentIDs(deploymentIDsJSON)

	// Get deployment details
	var deployments []map[string]interface{}
	for _, deploymentID := range b.DeploymentIDs {
		var stackName, templateName string
		err := h.db.QueryRow(`
			SELECT d.stack_name, t.name
			FROM deployments d
			LEFT JOIN templates t ON d.template_id = t.id
			WHERE d.id = $1`, deploymentID).Scan(&stackName, &templateName)

		if err == nil {
			deployments = append(deployments, map[string]interface{}{
				"id":            deploymentID,
				"stack_name":    stackName,
				"template_name": templateName,
			})
		}
	}

	response := map[string]interface{}{
		"id":               b.ID,
		"name":             b.Name,
		"type":             b.Type,
		"status":           b.Status,
		"size_bytes":       b.SizeBytes,
		"size_formatted":   b.GetFormattedSize(),
		"include_volumes":  b.IncludeVolumes,
		"encrypted":        b.Encrypted,
		"storage_path":     b.StoragePath,
		"deployments":      deployments,
		"deployment_count": len(deployments),
		"created_at":       b.CreatedAt,
		"completed_at":     b.CompletedAt,
		"duration":         b.GetDuration(),
		"is_completed":     b.IsCompleted(),
		"is_failed":        b.IsFailed(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Delete removes a backup
func (h *BackupsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	backupID := chi.URLParam(r, "id")
	if backupID == "" {
		http.Error(w, "Backup ID required", http.StatusBadRequest)
		return
	}

	// Get backup info
	var storagePath string
	err := h.db.QueryRow("SELECT storage_path FROM backups WHERE id = $1", backupID).Scan(&storagePath)

	if err == sql.ErrNoRows {
		http.Error(w, "Backup not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	// Delete backup file if it exists
	if storagePath != "" {
		os.Remove(storagePath)
	}

	// Remove from database
	_, err = h.db.Exec("DELETE FROM backups WHERE id = $1", backupID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete backup: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Backup deleted successfully",
	})
}

// Restore restores from a backup
func (h *BackupsHandler) Restore(w http.ResponseWriter, r *http.Request) {
	backupID := chi.URLParam(r, "id")
	
	var req models.RestoreConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	req.BackupID = backupID
	if err := req.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("Validation error: %v", err), http.StatusBadRequest)
		return
	}

	// Check if backup exists and is completed
	var status models.BackupStatus
	var storagePath string
	err := h.db.QueryRow("SELECT status, storage_path FROM backups WHERE id = $1", backupID).Scan(&status, &storagePath)

	if err == sql.ErrNoRows {
		http.Error(w, "Backup not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	if status != models.BackupStatusCompleted {
		http.Error(w, "Backup is not completed", http.StatusBadRequest)
		return
	}

	// Check if backup file exists
	if _, err := os.Stat(storagePath); os.IsNotExist(err) {
		http.Error(w, "Backup file not found", http.StatusNotFound)
		return
	}

	// Start restore process in background
	go h.performRestore(&req)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":    "Restore started",
		"backup_id":  backupID,
		"selective":  req.Selective,
		"test_mode":  req.TestRestore,
	})
}

// Download downloads a backup file
func (h *BackupsHandler) Download(w http.ResponseWriter, r *http.Request) {
	backupID := chi.URLParam(r, "id")

	// Get backup info
	var storagePath, name string
	var status models.BackupStatus
	err := h.db.QueryRow("SELECT storage_path, name, status FROM backups WHERE id = $1", backupID).Scan(&storagePath, &name, &status)

	if err == sql.ErrNoRows {
		http.Error(w, "Backup not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	if status != models.BackupStatusCompleted {
		http.Error(w, "Backup is not completed", http.StatusBadRequest)
		return
	}

	// Check if file exists
	if _, err := os.Stat(storagePath); os.IsNotExist(err) {
		http.Error(w, "Backup file not found", http.StatusNotFound)
		return
	}

	// Serve the file
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s.tar.gz\"", name))
	w.Header().Set("Content-Type", "application/gzip")
	
	http.ServeFile(w, r, storagePath)
}

// Upload uploads a backup file
func (h *BackupsHandler) Upload(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Backup upload not implemented", http.StatusNotImplemented)
}

// TestRestore performs a test restore without applying changes
func (h *BackupsHandler) TestRestore(w http.ResponseWriter, r *http.Request) {
	var req models.RestoreConfig
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	req.TestRestore = true
	if err := req.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("Validation error: %v", err), http.StatusBadRequest)
		return
	}

	// Perform test restore validation
	result := h.validateRestore(&req)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// Backup Schedules

// ListSchedules returns all backup schedules
func (h *BackupsHandler) ListSchedules(w http.ResponseWriter, r *http.Request) {
	query := `
		SELECT id, name, cron_expression, include_volumes, encrypt, enabled,
		       last_run, next_run, created_at
		FROM backup_schedules
		ORDER BY created_at DESC`

	rows, err := h.db.Query(query)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var schedules []models.BackupSchedule
	for rows.Next() {
		var s models.BackupSchedule
		var lastRun, nextRun sql.NullTime

		err := rows.Scan(
			&s.ID, &s.Name, &s.CronExpression, &s.IncludeVolumes, &s.Encrypt,
			&s.Enabled, &lastRun, &nextRun, &s.CreatedAt,
		)
		if err != nil {
			continue
		}

		if lastRun.Valid {
			s.LastRun = &lastRun.Time
		}
		if nextRun.Valid {
			s.NextRun = &nextRun.Time
		}

		schedules = append(schedules, s)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"schedules": schedules,
	})
}

// CreateSchedule creates a new backup schedule
func (h *BackupsHandler) CreateSchedule(w http.ResponseWriter, r *http.Request) {
	var schedule models.BackupSchedule

	if err := json.NewDecoder(r.Body).Decode(&schedule); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if schedule.Name == "" || schedule.CronExpression == "" {
		http.Error(w, "Name and cron expression required", http.StatusBadRequest)
		return
	}

	schedule.CreatedAt = time.Now()
	schedule.UpdateNextRun() // Calculate next run time

	_, err := h.db.Exec(`
		INSERT INTO backup_schedules (name, cron_expression, include_volumes, encrypt, enabled, next_run, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		schedule.Name, schedule.CronExpression, schedule.IncludeVolumes,
		schedule.Encrypt, schedule.Enabled, schedule.NextRun, schedule.CreatedAt,
	)

	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create schedule: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Schedule created successfully",
	})
}

// UpdateSchedule updates a backup schedule
func (h *BackupsHandler) UpdateSchedule(w http.ResponseWriter, r *http.Request) {
	scheduleID := chi.URLParam(r, "id")
	
	var updates map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Build update query dynamically
	setParts := []string{}
	args := []interface{}{}
	argCount := 0

	for field, value := range updates {
		argCount++
		setParts = append(setParts, fmt.Sprintf("%s = $%d", field, argCount))
		args = append(args, value)
	}

	if len(setParts) == 0 {
		http.Error(w, "No fields to update", http.StatusBadRequest)
		return
	}

	argCount++
	query := fmt.Sprintf("UPDATE backup_schedules SET %s WHERE id = $%d", 
		strings.Join(setParts, ", "), argCount)
	args = append(args, scheduleID)

	_, err := h.db.Exec(query, args...)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to update schedule: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Schedule updated successfully",
	})
}

// DeleteSchedule deletes a backup schedule
func (h *BackupsHandler) DeleteSchedule(w http.ResponseWriter, r *http.Request) {
	scheduleID := chi.URLParam(r, "id")

	_, err := h.db.Exec("DELETE FROM backup_schedules WHERE id = $1", scheduleID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete schedule: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Schedule deleted successfully",
	})
}

// Helper functions

func (h *BackupsHandler) performBackup(backup *models.Backup) {
	// TODO: Implement actual backup logic:
	// 1. Create backup directory
	// 2. Export docker-compose files for each deployment
	// 3. Backup volume data if requested
	// 4. Create backup metadata
	// 5. Create compressed archive
	// 6. Encrypt if requested
	// 7. Update database with final status and size

	// Simulate backup process
	time.Sleep(10 * time.Second)

	// Update status to completed
	now := time.Now()
	h.db.Exec(`
		UPDATE backups SET status = $1, size_bytes = $2, completed_at = $3 
		WHERE id = $4`,
		models.BackupStatusCompleted, 1024*1024*10, now, backup.ID, // 10MB simulated size
	)
}

func (h *BackupsHandler) performRestore(config *models.RestoreConfig) {
	// TODO: Implement actual restore logic:
	// 1. Extract backup archive
	// 2. Validate backup contents
	// 3. Stop existing deployments if overwrite is enabled
	// 4. Restore docker-compose files
	// 5. Restore volume data if included
	// 6. Deploy restored stacks
	// 7. Verify deployment success

	// Simulate restore process
	time.Sleep(15 * time.Second)
}

func (h *BackupsHandler) validateRestore(config *models.RestoreConfig) map[string]interface{} {
	// TODO: Implement restore validation:
	// 1. Check backup file integrity
	// 2. Validate backup format version
	// 3. Check for conflicts with existing deployments
	// 4. Estimate restore time and resource requirements

	return map[string]interface{}{
		"valid":          true,
		"conflicts":      []string{},
		"warnings":       []string{},
		"estimated_time": "5 minutes",
		"required_space": "100MB",
	}
}