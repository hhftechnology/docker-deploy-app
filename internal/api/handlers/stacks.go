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

// StacksHandler handles stack-related HTTP requests
type StacksHandler struct {
	db           *sql.DB
	dockerClient *client.Client
	config       *config.Config
	compose      *docker.ComposeManager
	upgrader     websocket.Upgrader
}

// NewStacksHandler creates a new stacks handler
func NewStacksHandler(db *sql.DB, dockerClient *client.Client, config *config.Config) *StacksHandler {
	return &StacksHandler{
		db:           db,
		dockerClient: dockerClient,
		config:       config,
		compose:      docker.NewComposeManager("./deployments", time.Duration(config.Docker.ComposeTimeout)*time.Second),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool { return true },
		},
	}
}

// List returns all running stacks
func (h *StacksHandler) List(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	limit := getIntParam(r, "limit", 50)

	query := `
		SELECT d.id, d.stack_name, d.status, d.newt_injected, d.tunnel_url, 
		       d.created_at, t.name as template_name
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

	rows, err := h.db.Query(query, args...)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var stacks []map[string]interface{}
	for rows.Next() {
		var deploymentID, stackName, status, templateName string
		var newtInjected bool
		var tunnelURL sql.NullString
		var createdAt time.Time

		err := rows.Scan(&deploymentID, &stackName, &status, &newtInjected, 
			&tunnelURL, &createdAt, &templateName)
		if err != nil {
			continue
		}

		// Get stack details from Docker
		stackStatus, _ := h.compose.GetStackStatus(stackName)
		services, _ := h.compose.GetServices(stackName)

		stack := map[string]interface{}{
			"id":            deploymentID,
			"name":          stackName,
			"status":        stackStatus,
			"template_name": templateName,
			"services":      len(services),
			"running_services": h.countRunningServices(services),
			"newt_injected": newtInjected,
			"tunnel_url":    tunnelURL.String,
			"created_at":    createdAt,
		}

		stacks = append(stacks, stack)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"stacks": stacks,
		"total":  len(stacks),
	})
}

// Get returns detailed information about a specific stack
func (h *StacksHandler) Get(w http.ResponseWriter, r *http.Request) {
	stackID := chi.URLParam(r, "id")

	var stackName, templateName string
	var newtInjected bool
	var tunnelURL sql.NullString

	err := h.db.QueryRow(`
		SELECT d.stack_name, d.newt_injected, d.tunnel_url, t.name
		FROM deployments d
		LEFT JOIN templates t ON d.template_id = t.id
		WHERE d.id = $1`, stackID).Scan(&stackName, &newtInjected, &tunnelURL, &templateName)

	if err == sql.ErrNoRows {
		http.Error(w, "Stack not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	// Get services from Docker
	services, _ := h.compose.GetServices(stackName)
	status, _ := h.compose.GetStackStatus(stackName)

	response := map[string]interface{}{
		"id":            stackID,
		"name":          stackName,
		"status":        status,
		"template_name": templateName,
		"newt_injected": newtInjected,
		"tunnel_url":    tunnelURL.String,
		"services":      services,
		"service_count": len(services),
		"running_services": h.countRunningServices(services),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Start starts a stack
func (h *StacksHandler) Start(w http.ResponseWriter, r *http.Request) {
	stackID := chi.URLParam(r, "id")
	stackName := h.getStackName(stackID)
	if stackName == "" {
		http.Error(w, "Stack not found", http.StatusNotFound)
		return
	}

	if err := h.compose.Start(stackName); err != nil {
		http.Error(w, fmt.Sprintf("Failed to start stack: %v", err), http.StatusInternalServerError)
		return
	}

	h.updateDeploymentStatus(stackID, models.StatusRunning)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Stack started successfully",
	})
}

// Stop stops a stack
func (h *StacksHandler) Stop(w http.ResponseWriter, r *http.Request) {
	stackID := chi.URLParam(r, "id")
	stackName := h.getStackName(stackID)
	if stackName == "" {
		http.Error(w, "Stack not found", http.StatusNotFound)
		return
	}

	if err := h.compose.Stop(stackName); err != nil {
		http.Error(w, fmt.Sprintf("Failed to stop stack: %v", err), http.StatusInternalServerError)
		return
	}

	h.updateDeploymentStatus(stackID, models.StatusStopped)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Stack stopped successfully",
	})
}

// Restart restarts a stack
func (h *StacksHandler) Restart(w http.ResponseWriter, r *http.Request) {
	stackID := chi.URLParam(r, "id")
	stackName := h.getStackName(stackID)
	if stackName == "" {
		http.Error(w, "Stack not found", http.StatusNotFound)
		return
	}

	if err := h.compose.Restart(stackName); err != nil {
		http.Error(w, fmt.Sprintf("Failed to restart stack: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Stack restarted successfully",
	})
}

// GetLogs returns stack logs
func (h *StacksHandler) GetLogs(w http.ResponseWriter, r *http.Request) {
	stackID := chi.URLParam(r, "id")
	stackName := h.getStackName(stackID)
	if stackName == "" {
		http.Error(w, "Stack not found", http.StatusNotFound)
		return
	}

	tail := getIntParam(r, "tail", 100)
	cmd, err := h.compose.Logs(stackName, false, tail)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get logs: %v", err), http.StatusInternalServerError)
		return
	}

	output, err := cmd.Output()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read logs: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Write(output)
}

// StreamLogs streams stack logs via HTTP
func (h *StacksHandler) StreamLogs(w http.ResponseWriter, r *http.Request) {
	stackID := chi.URLParam(r, "id")
	stackName := h.getStackName(stackID)
	if stackName == "" {
		http.Error(w, "Stack not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	cmd, err := h.compose.Logs(stackName, true, 50)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to start log stream: %v", err), http.StatusInternalServerError)
		return
	}

	cmd.Stdout = w
	cmd.Run()
}

// WebSocketLogs handles WebSocket connections for real-time logs
func (h *StacksHandler) WebSocketLogs(w http.ResponseWriter, r *http.Request) {
	stackID := chi.URLParam(r, "id")
	stackName := h.getStackName(stackID)
	if stackName == "" {
		http.Error(w, "Stack not found", http.StatusNotFound)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	// Stream logs via WebSocket
	for {
		time.Sleep(1 * time.Second)
		message := map[string]interface{}{
			"timestamp": time.Now(),
			"message":   "Log streaming not fully implemented",
		}
		if err := conn.WriteJSON(message); err != nil {
			return
		}
	}
}

// GetStats returns resource usage statistics
func (h *StacksHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	stackID := chi.URLParam(r, "id")
	stackName := h.getStackName(stackID)
	if stackName == "" {
		http.Error(w, "Stack not found", http.StatusNotFound)
		return
	}

	services, _ := h.compose.GetServices(stackName)
	
	stats := map[string]interface{}{
		"stack_name":       stackName,
		"total_services":   len(services),
		"running_services": h.countRunningServices(services),
		"updated_at":       time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// GetNewtStatus returns Newt tunnel status
func (h *StacksHandler) GetNewtStatus(w http.ResponseWriter, r *http.Request) {
	stackID := chi.URLParam(r, "id")
	stackName := h.getStackName(stackID)
	if stackName == "" {
		http.Error(w, "Stack not found", http.StatusNotFound)
		return
	}

	var newtInjected bool
	var tunnelURL sql.NullString
	h.db.QueryRow("SELECT newt_injected, tunnel_url FROM deployments WHERE id = $1", 
		stackID).Scan(&newtInjected, &tunnelURL)

	response := map[string]interface{}{
		"newt_injected": newtInjected,
		"tunnel_url":    tunnelURL.String,
		"tunnel_active": tunnelURL.String != "",
		"status":        "unknown",
	}

	if newtInjected {
		// Check if newt container is running
		services, _ := h.compose.GetServices(stackName)
		for _, service := range services {
			if service.Name == "newt" {
				response["status"] = service.Status
				break
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Export exports stack configuration
func (h *StacksHandler) Export(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "Stack export not implemented", http.StatusNotImplemented)
}

// Helper functions
func (h *StacksHandler) getStackName(stackID string) string {
	var stackName string
	h.db.QueryRow("SELECT stack_name FROM deployments WHERE id = $1", stackID).Scan(&stackName)
	return stackName
}

func (h *StacksHandler) updateDeploymentStatus(deploymentID string, status models.DeploymentStatus) {
	h.db.Exec("UPDATE deployments SET status = $1, updated_at = $2 WHERE id = $3",
		status, time.Now(), deploymentID)
}

func (h *StacksHandler) countRunningServices(services []models.StackService) int {
	count := 0
	for _, service := range services {
		if service.Status == "running" {
			count++
		}
	}
	return count
}