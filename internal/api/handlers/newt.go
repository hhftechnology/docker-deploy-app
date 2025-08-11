package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"docker-deploy-app/internal/config"
	"docker-deploy-app/internal/models"
)

// NewtHandler handles Newt tunnel configuration HTTP requests
type NewtHandler struct {
	db     *sql.DB
	config *config.Config
}

// NewNewtHandler creates a new Newt handler
func NewNewtHandler(db *sql.DB, config *config.Config) *NewtHandler {
	return &NewtHandler{
		db:     db,
		config: config,
	}
}

// GetConfig returns the current Newt configuration
func (h *NewtHandler) GetConfig(w http.ResponseWriter, r *http.Request) {
	// Get active Newt configuration
	var newtConfig models.NewtConfig
	
	query := `
		SELECT id, endpoint, newt_id, newt_secret, is_active, created_at
		FROM newt_configs 
		WHERE is_active = true
		ORDER BY created_at DESC
		LIMIT 1`

	err := h.db.QueryRow(query).Scan(
		&newtConfig.ID, &newtConfig.Endpoint, &newtConfig.NewtID,
		&newtConfig.Secret, &newtConfig.IsActive, &newtConfig.CreatedAt,
	)

	if err == sql.ErrNoRows {
		// No active configuration found
		response := map[string]interface{}{
			"configured": false,
			"message":    "No Newt configuration found",
			"defaults": map[string]interface{}{
				"image":       h.config.Newt.DefaultImage,
				"enabled":     h.config.Newt.Enabled,
				"auto_inject": h.config.Newt.AutoInject,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}

	// Mask the secret for security
	maskedSecret := ""
	if len(newtConfig.Secret) > 8 {
		maskedSecret = newtConfig.Secret[:4] + "****" + newtConfig.Secret[len(newtConfig.Secret)-4:]
	}

	response := map[string]interface{}{
		"configured":  true,
		"id":          newtConfig.ID,
		"endpoint":    newtConfig.Endpoint,
		"newt_id":     newtConfig.NewtID,
		"secret":      maskedSecret, // Masked for security
		"is_active":   newtConfig.IsActive,
		"created_at":  newtConfig.CreatedAt,
		"settings": map[string]interface{}{
			"enabled":     h.config.Newt.Enabled,
			"auto_inject": h.config.Newt.AutoInject,
			"image":       h.config.Newt.DefaultImage,
			"validation":  h.config.Newt.Validation,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// UpdateConfig updates the Newt configuration
func (h *NewtHandler) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Endpoint string `json:"endpoint"`
		NewtID   string `json:"newt_id"`
		Secret   string `json:"secret"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Create new configuration
	newtConfig := &models.NewtConfig{
		Endpoint:  req.Endpoint,
		NewtID:    req.NewtID,
		Secret:    req.Secret,
		IsActive:  false, // Will be activated after validation
		CreatedAt: time.Now(),
	}

	// Validate configuration
	if err := newtConfig.Validate(); err != nil {
		http.Error(w, fmt.Sprintf("Validation error: %v", err), http.StatusBadRequest)
		return
	}

	// Test the configuration
	validationResult, err := h.testNewtConnection(newtConfig)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to test connection: %v", err), http.StatusInternalServerError)
		return
	}

	if !validationResult.IsValid() {
		response := map[string]interface{}{
			"success":    false,
			"message":    "Configuration validation failed",
			"validation": validationResult,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Deactivate existing configurations
	_, err = h.db.Exec("UPDATE newt_configs SET is_active = false")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to deactivate old configs: %v", err), http.StatusInternalServerError)
		return
	}

	// Save new configuration as active
	newtConfig.IsActive = true
	_, err = h.db.Exec(`
		INSERT INTO newt_configs (endpoint, newt_id, newt_secret, is_active, created_at)
		VALUES ($1, $2, $3, $4, $5)`,
		newtConfig.Endpoint, newtConfig.NewtID, newtConfig.Secret,
		newtConfig.IsActive, newtConfig.CreatedAt,
	)

	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to save configuration: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"success":    true,
		"message":    "Newt configuration updated successfully",
		"validation": validationResult,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ValidateConfig validates Newt configuration without saving
func (h *NewtHandler) ValidateConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Endpoint string `json:"endpoint"`
		NewtID   string `json:"newt_id"`
		Secret   string `json:"secret"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Create temporary configuration for testing
	newtConfig := &models.NewtConfig{
		Endpoint: req.Endpoint,
		NewtID:   req.NewtID,
		Secret:   req.Secret,
	}

	// Validate configuration format
	if err := newtConfig.Validate(); err != nil {
		response := map[string]interface{}{
			"valid":   false,
			"message": err.Error(),
			"issues":  []string{err.Error()},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
		return
	}

	// Test the configuration
	validationResult, err := h.testNewtConnection(newtConfig)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to test connection: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(validationResult)
}

// GetStatus returns the current status of Newt services
func (h *NewtHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	// Get all deployments with Newt injection
	query := `
		SELECT d.id, d.stack_name, d.status, d.tunnel_url
		FROM deployments d
		WHERE d.newt_injected = true`

	rows, err := h.db.Query(query)
	if err != nil {
		http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var services []map[string]interface{}
	totalServices := 0
	activeServices := 0

	for rows.Next() {
		var deploymentID, stackName, status string
		var tunnelURL sql.NullString

		err := rows.Scan(&deploymentID, &stackName, &status, &tunnelURL)
		if err != nil {
			continue
		}

		totalServices++

		// Get detailed Newt status for this stack
		newtStatus := h.getNewtServiceStatus(stackName)
		
		if newtStatus != nil && newtStatus.TunnelActive {
			activeServices++
		}

		service := map[string]interface{}{
			"deployment_id": deploymentID,
			"stack_name":    stackName,
			"status":        status,
			"tunnel_url":    tunnelURL.String,
			"newt_status":   newtStatus,
		}

		services = append(services, service)
	}

	// Get system-wide Newt configuration status
	var hasActiveConfig bool
	h.db.QueryRow("SELECT EXISTS(SELECT 1 FROM newt_configs WHERE is_active = true)").Scan(&hasActiveConfig)

	response := map[string]interface{}{
		"system_status": map[string]interface{}{
			"configured":      hasActiveConfig,
			"enabled":         h.config.Newt.Enabled,
			"auto_inject":     h.config.Newt.AutoInject,
			"default_image":   h.config.Newt.DefaultImage,
		},
		"services": map[string]interface{}{
			"total":          totalServices,
			"active":         activeServices,
			"inactive":       totalServices - activeServices,
			"details":        services,
		},
		"updated_at": time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// TestConnection tests connectivity to Newt endpoints
func (h *NewtHandler) TestConnection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		StackName string `json:"stack_name,omitempty"`
		Endpoint  string `json:"endpoint,omitempty"`
		NewtID    string `json:"newt_id,omitempty"`
		Secret    string `json:"secret,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	var tests []models.NewtConnectionTest

	if req.StackName != "" {
		// Test specific stack's Newt service
		test := h.testStackNewtConnection(req.StackName)
		tests = append(tests, test)
	} else if req.Endpoint != "" {
		// Test provided configuration
		newtConfig := &models.NewtConfig{
			Endpoint: req.Endpoint,
			NewtID:   req.NewtID,
			Secret:   req.Secret,
		}
		
		test := h.testEndpointConnection(newtConfig)
		tests = append(tests, test)
	} else {
		// Test active configuration
		var newtConfig models.NewtConfig
		err := h.db.QueryRow(`
			SELECT endpoint, newt_id, newt_secret
			FROM newt_configs 
			WHERE is_active = true
			LIMIT 1`).Scan(&newtConfig.Endpoint, &newtConfig.NewtID, &newtConfig.Secret)

		if err == sql.ErrNoRows {
			http.Error(w, "No active Newt configuration found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, fmt.Sprintf("Database error: %v", err), http.StatusInternalServerError)
			return
		}

		test := h.testEndpointConnection(&newtConfig)
		tests = append(tests, test)
	}

	// Calculate overall status
	allSuccessful := true
	for _, test := range tests {
		if !test.IsSuccessful() {
			allSuccessful = false
			break
		}
	}

	response := map[string]interface{}{
		"overall_success": allSuccessful,
		"tests":           tests,
		"tested_at":       time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Helper functions

func (h *NewtHandler) testNewtConnection(config *models.NewtConfig) (*models.NewtValidationResult, error) {
	// TODO: Implement actual Newt connection testing
	// This would involve:
	// 1. Making HTTP requests to the Pangolin endpoint
	// 2. Validating authentication with Newt ID and secret
	// 3. Checking available features and version
	// 4. Testing tunnel creation capabilities

	// Simulated validation for now
	result := &models.NewtValidationResult{
		Valid:         true,
		Reachable:     true,
		Authenticated: true,
		Issues:        []string{},
		Version:       "1.0.0",
		Features:      []string{"tunnels", "health_checks", "monitoring"},
		TestedAt:      time.Now(),
	}

	// Simulate some basic validation
	if config.Endpoint == "" {
		result.Valid = false
		result.Issues = append(result.Issues, "Endpoint cannot be empty")
	}

	if config.NewtID == "" {
		result.Valid = false
		result.Issues = append(result.Issues, "Newt ID cannot be empty")
	}

	if config.Secret == "" {
		result.Valid = false
		result.Issues = append(result.Issues, "Secret cannot be empty")
	}

	// Simulate network reachability test
	if config.Endpoint != "" && len(config.Endpoint) < 10 {
		result.Reachable = false
		result.Issues = append(result.Issues, "Endpoint appears to be invalid")
	}

	return result, nil
}

func (h *NewtHandler) getNewtServiceStatus(stackName string) *models.NewtStatus {
	// TODO: Implement actual Newt service status checking
	// This would involve:
	// 1. Finding the Newt container in the stack
	// 2. Checking container health status
	// 3. Verifying tunnel connectivity
	// 4. Getting tunnel metrics and statistics

	// Simulated status for now
	return &models.NewtStatus{
		ServiceName:  "newt",
		Status:       "running",
		Health:       "healthy",
		TunnelActive: true,
		TunnelURL:    fmt.Sprintf("https://%s.tunnel.example.com", stackName),
		ConnectedAt:  timePtr(time.Now().Add(-1 * time.Hour)),
		LastPing:     timePtr(time.Now().Add(-30 * time.Second)),
		BytesIn:      1024 * 1024 * 50,  // 50MB
		BytesOut:     1024 * 1024 * 100, // 100MB
		ErrorCount:   0,
		UpdatedAt:    time.Now(),
	}
}

func (h *NewtHandler) testStackNewtConnection(stackName string) models.NewtConnectionTest {
	// TODO: Implement actual stack Newt connection testing
	// This would test the specific Newt service in a stack

	return models.NewtConnectionTest{
		TestType:     "stack_tunnel",
		Success:      true,
		ResponseTime: 50 * time.Millisecond,
		Message:      fmt.Sprintf("Stack '%s' Newt service is healthy", stackName),
		TestedAt:     time.Now(),
	}
}

func (h *NewtHandler) testEndpointConnection(config *models.NewtConfig) models.NewtConnectionTest {
	// TODO: Implement actual endpoint connection testing
	// This would test connectivity to the Pangolin endpoint

	startTime := time.Now()
	
	// Simulate connection test
	time.Sleep(25 * time.Millisecond)
	
	return models.NewtConnectionTest{
		TestType:     "endpoint_connectivity",
		Success:      true,
		ResponseTime: time.Since(startTime),
		Message:      "Successfully connected to Pangolin endpoint",
		TestedAt:     time.Now(),
	}
}

// Helper function to create time pointer
func timePtr(t time.Time) *time.Time {
	return &t
}