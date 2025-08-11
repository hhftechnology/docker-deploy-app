package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/docker/docker/client"
	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"docker-deploy-app/internal/config"
	"docker-deploy-app/internal/docker"
	"docker-deploy-app/internal/models"
)

// DeploymentsHandler handles deployment-related HTTP requests
type DeploymentsHandler struct {
	db           *sql.DB
	dockerClient *client.Client
	config       *config.Config
	compose      *docker.ComposeManager
	upgrader     websocket.Upgrader
}

// NewDeploymentsHandler creates a new deployments handler
func NewDeploymentsHandler(db *sql.DB, dockerClient *client.Client, config *config.Config) *DeploymentsHandler {
	return &DeploymentsHandler{
		db:           db,
		dockerClient: dockerClient,
		config:       config,
		compose:      docker.NewComposeManager("./deployments", time.Duration(config.Docker.ComposeTimeout)*time.Second),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true }, // Allow all origins for demo
		},
	}
}

// List returns all deployments
func (h *DeploymentsHandler) List(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	limit := getIntParam(r, "limit", 50)
	offset := getIntParam(r, "offset", 0)

	query := `
		SELECT d.id, d.template_id, d.stack_name, d.status, d.config, d.newt_injected,
		       d.tunnel_url, d.created_at, d.updated_at, t.name as template_name
		FROM deployments d
		LEFT JOIN templates t ON d.template_id = t.id
		WHERE 1=1`

	args := []interface{}{}
	argCount := 0

	if status != "" {
		argCount++
		query += fmt.Sprintf(" AND d.status = $%d", argCount)
		args = append(args, status)
	}

	query += " ORDER BY d.created_at DESC"
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

	var deployments []map[string]interface{}
	for rows.Next() {
		var d models.Deployment
		var configJSON, templateName string

		err := rows.Scan(
			&d.ID, &d.TemplateID, &d.StackName, &d.Status, &configJSON,
			&d.NewtInjected, &d.TunnelURL, &d.CreatedAt, &d.UpdatedAt, &templateName,
		)
		if err != nil {
			continue
		}

		d.UnmarshalConfig(configJSON)

		deployment := map[string]interface{}{
			"id":            d.ID,
			"template_id":   d.TemplateID,
			"template_name": templateName,
			"stack_name":    d.StackName,
			"status":        d.Status,
			"config":        d.Config,
			"newt_injected": d.NewtInjected,
			"tunnel_url":    d.TunnelURL,
			"created_at":    d.CreatedAt,
			"updated_at":    d.UpdatedAt,
			"is_running":    d.IsRunning(),
			"can_start":     d.CanStart(),
			"can_stop":      d.CanStop(),
		}

		deployments = append(deployments, deployment)
	}

	response := map[string]interface{}{
		"deployments": deployments,
		"total":       len(deployments),
		"limit":       limit,
		"offset":      offset,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Create creates a new deployment
func (h *DeploymentsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req models.DeploymentConfig

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if err := req.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("Validation error: %v", err), http.StatusBadRequest)
		return
	}

	// Check if template exists
	var template models.Template
	var tagsJSON, variablesJSON, newtConfigJSON string
	err := h.db.QueryRow(`
		SELECT id, name, description, requires_newt, variables, newt_config
		FROM templates WHERE id = $1`, req.TemplateID).Scan(
		&template.ID, &template.Name, &template.Description,
		&template.RequiresNewt, &variablesJSON, &newtConfigJSON,
	)

	if err == sql.ErrNoRows {
		http.Error(w, "Template not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	template.UnmarshalVariables(variablesJSON)
	template.UnmarshalNewtConfig(newtConfigJSON)

	// Check if stack name is unique
	var existingID string
	err = h.db.QueryRow("SELECT id FROM deployments WHERE stack_name = $1", req.StackName).Scan(&existingID)
	if err != sql.ErrNoRows {
		http.Error(w, "Stack name already exists", http.StatusConflict)
		return
	}

	// Generate deployment ID
	deploymentID := fmt.Sprintf("deploy_%d", time.Now().Unix())

	// Create deployment record
	deployment := &models.Deployment{
		ID:           deploymentID,
		TemplateID:   req.TemplateID,
		StackName:    req.StackName,
		Status:       models.StatusPending,
		NewtInjected: req.IncludeNewt,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	// Set configuration
	deployment.Config = map[string]interface{}{
		"environment":  req.Environment,
		"auto_start":   req.AutoStart,
		"include_newt": req.IncludeNewt,
	}

	if req.NewtConfig != nil {
		deployment.Config["newt_config"] = req.NewtConfig
	}

	// Save to database
	configJSON, _ := deployment.MarshalConfig()
	_, err = h.db.Exec(`
		INSERT INTO deployments (id, template_id, stack_name, status, config, newt_injected, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		deployment.ID, deployment.TemplateID, deployment.StackName, deployment.Status,
		configJSON, deployment.NewtInjected, deployment.CreatedAt, deployment.UpdatedAt,
	)

	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create deployment: %v", err), http.StatusInternalServerError)
		return
	}

	// Start deployment process in background
	go h.performDeployment(deployment, &template, &req)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":         deployment.ID,
		"stack_name": deployment.StackName,
		"status":     deployment.Status,
		"message":    "Deployment started",
	})
}

// Get returns a specific deployment
func (h *DeploymentsHandler) Get(w http.ResponseWriter, r *http.Request) {
	deploymentID := chi.URLParam(r, "id")
	if deploymentID == "" {
		http.Error(w, "Deployment ID required", http.StatusBadRequest)
		return
	}

	var d models.Deployment
	var configJSON, templateName string

	query := `
		SELECT d.id, d.template_id, d.stack_name, d.status, d.config, d.newt_injected,
		       d.tunnel_url, d.created_at, d.updated_at, t.name as template_name
		FROM deployments d
		LEFT JOIN templates t ON d.template_id = t.id
		WHERE d.id = $1`

	err := h.db.QueryRow(query, deploymentID).Scan(
		&d.ID, &d.TemplateID, &d.StackName, &d.Status, &configJSON,
		&d.NewtInjected, &d.TunnelURL, &d.CreatedAt, &d.UpdatedAt, &templateName,
	)

	if err == sql.ErrNoRows {
		http.Error(w, "Deployment not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	d.UnmarshalConfig(configJSON)

	response := map[string]interface{}{
		"id":            d.ID,
		"template_id":   d.TemplateID,
		"template_name": templateName,
		"stack_name":    d.StackName,
		"status":        d.Status,
		"config":        d.Config,
		"newt_injected": d.NewtInjected,
		"tunnel_url":    d.TunnelURL,
		"created_at":    d.CreatedAt,
		"updated_at":    d.UpdatedAt,
		"is_running":    d.IsRunning(),
		"can_start":     d.CanStart(),
		"can_stop":      d.CanStop(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Delete removes a deployment
func (h *DeploymentsHandler) Delete(w http.ResponseWriter, r *http.Request) {
	deploymentID := chi.URLParam(r, "id")
	if deploymentID == "" {
		http.Error(w, "Deployment ID required", http.StatusBadRequest)
		return
	}

	// Get deployment info
	var stackName string
	var status models.DeploymentStatus
	err := h.db.QueryRow("SELECT stack_name, status FROM deployments WHERE id = $1", deploymentID).Scan(&stackName, &status)

	if err == sql.ErrNoRows {
		http.Error(w, "Deployment not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	// Stop and remove the stack if it's running
	if status == models.StatusRunning {
		if err := h.compose.Down(stackName, true); err != nil {
			http.Error(w, fmt.Sprintf("Failed to stop stack: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// Remove from database
	_, err = h.db.Exec("DELETE FROM deployments WHERE id = $1", deploymentID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to delete deployment: %v", err), http.StatusInternalServerError)
		return
	}

	// Also delete logs
	h.db.Exec("DELETE FROM deployment_logs WHERE deployment_id = $1", deploymentID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Deployment deleted successfully",
	})
}

// GetLogs returns deployment logs
func (h *DeploymentsHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	deploymentID := chi.URLParam(r, "id")
	limit := getIntParam(r, "limit", 100)

	query := `
		SELECT log_level, message, timestamp
		FROM deployment_logs 
		WHERE deployment_id = $1
		ORDER BY timestamp DESC
		LIMIT $2`

	rows, err := h.db.Query(query, deploymentID, limit)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var logs []models.DeploymentLog
	for rows.Next() {
		var log models.DeploymentLog
		err := rows.Scan(&log.LogLevel, &log.Message, &log.Timestamp)
		if err != nil {
			continue
		}
		log.DeploymentID = deploymentID
		logs = append(logs, log)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"logs": logs,
	})
}

// StreamLogs streams deployment logs via HTTP (for non-WebSocket clients)
func (h *DeploymentsHandler) StreamLogs(w http.ResponseWriter, r *http.Request) {
	deploymentID := chi.URLParam(r, "id")
	
	// Check if deployment exists
	var exists bool
	err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM deployments WHERE id = $1)", deploymentID).Scan(&exists)
	if err != nil || !exists {
		http.Error(w, "Deployment not found", http.StatusNotFound)
		return
	}

	// Set headers for streaming
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Get recent logs first
	logs, _ := h.getRecentLogs(deploymentID, 50)
	for _, log := range logs {
		fmt.Fprintf(w, "[%s] %s: %s\n", log.Timestamp.Format("15:04:05"), log.LogLevel, log.Message)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}

// WebSocketLogs handles WebSocket connections for real-time logs
func (h *DeploymentsHandler) WebSocketLogs(w http.ResponseWriter, r *http.Request) {
	deploymentID := chi.URLParam(r, "id")

	// Check if deployment exists
	var exists bool
	err := h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM deployments WHERE id = $1)", deploymentID).Scan(&exists)
	if err != nil || !exists {
		http.Error(w, "Deployment not found", http.StatusNotFound)
		return
	}

	// Upgrade to WebSocket
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, "Failed to upgrade to WebSocket", http.StatusBadRequest)
		return
	}
	defer conn.Close()

	// Send recent logs first
	logs, _ := h.getRecentLogs(deploymentID, 50)
	for _, log := range logs {
		message := map[string]interface{}{
			"timestamp": log.Timestamp,
			"level":     log.LogLevel,
			"message":   log.Message,
		}
		conn.WriteJSON(message)
	}

	// Keep connection alive and send new logs
	// This is a simplified implementation - in production you'd use a proper pub/sub system
	for {
		time.Sleep(1 * time.Second)
		// Check for new logs (this is inefficient, use pub/sub in production)
		newLogs, _ := h.getRecentLogs(deploymentID, 5)
		for _, log := range newLogs {
			message := map[string]interface{}{
				"timestamp": log.Timestamp,
				"level":     log.LogLevel,
				"message":   log.Message,
			}
			if err := conn.WriteJSON(message); err != nil {
				return // Connection closed
			}
		}
	}
}

// GetTunnelInfo returns tunnel information for a deployment
func (h *DeploymentsHandler) GetTunnelInfo(w http.ResponseWriter, r *http.Request) {
	deploymentID := chi.URLParam(r, "id")

	var tunnelURL string
	var newt_injected bool
	err := h.db.QueryRow("SELECT tunnel_url, newt_injected FROM deployments WHERE id = $1", deploymentID).Scan(&tunnelURL, &newt_injected)

	if err == sql.ErrNoRows {
		http.Error(w, "Deployment not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"deployment_id":  deploymentID,
		"tunnel_url":     tunnelURL,
		"newt_injected":  newt_injected,
		"tunnel_active":  tunnelURL != "",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// CreateBackup creates a backup of the deployment
func (h *DeploymentsHandler) CreateBackup(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Deployment backup not implemented", http.StatusNotImplemented)
}

// performDeployment handles the actual deployment process
func (h *DeploymentsHandler) performDeployment(deployment *models.Deployment, template *models.Template, config *models.DeploymentConfig) {
	// Update status to deploying
	h.updateDeploymentStatus(deployment.ID, models.StatusDeploying)
	h.addDeploymentLog(deployment.ID, "info", "Starting deployment process")

	// TODO: Implement actual deployment logic:
	// 1. Fetch docker-compose.yml from GitHub
	// 2. Inject Newt service if needed
	// 3. Create environment file
	// 4. Deploy using docker compose
	// 5. Monitor deployment status
	// 6. Update database with final status

	// Simulate deployment process
	time.Sleep(5 * time.Second)

	// For now, just mark as successful
	h.updateDeploymentStatus(deployment.ID, models.StatusRunning)
	h.addDeploymentLog(deployment.ID, "info", "Deployment completed successfully")

	// Set tunnel URL if newt is injected
	if deployment.NewtInjected {
		tunnelURL := fmt.Sprintf("https://%s.tunnel.example.com", deployment.StackName)
		h.updateTunnelURL(deployment.ID, tunnelURL)
	}
}

// Helper functions
func (h *DeploymentsHandler) updateDeploymentStatus(deploymentID string, status models.DeploymentStatus) {
	h.db.Exec("UPDATE deployments SET status = $1, updated_at = $2 WHERE id = $3",
		status, time.Now(), deploymentID)
}

func (h *DeploymentsHandler) addDeploymentLog(deploymentID, level, message string) {
	h.db.Exec("INSERT INTO deployment_logs (deployment_id, log_level, message, timestamp) VALUES ($1, $2, $3, $4)",
		deploymentID, level, message, time.Now())
}

func (h *DeploymentsHandler) updateTunnelURL(deploymentID, tunnelURL string) {
	h.db.Exec("UPDATE deployments SET tunnel_url = $1 WHERE id = $2", tunnelURL, deploymentID)
}

func (h *DeploymentsHandler) getRecentLogs(deploymentID string, limit int) ([]models.DeploymentLog, error) {
	query := `
		SELECT log_level, message, timestamp
		FROM deployment_logs 
		WHERE deployment_id = $1
		ORDER BY timestamp DESC
		LIMIT $2`

	rows, err := h.db.Query(query, deploymentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []models.DeploymentLog
	for rows.Next() {
		var log models.DeploymentLog
		err := rows.Scan(&log.LogLevel, &log.Message, &log.Timestamp)
		if err != nil {
			continue
		}
		log.DeploymentID = deploymentID
		logs = append(logs, log)
	}

	return logs, nil
}