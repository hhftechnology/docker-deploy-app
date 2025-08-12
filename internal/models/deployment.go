package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// DeploymentStatus represents the current status of a deployment
type DeploymentStatus string

const (
	StatusPending   DeploymentStatus = "pending"
	StatusDeploying DeploymentStatus = "deploying"
	StatusRunning   DeploymentStatus = "running"
	StatusStopped   DeploymentStatus = "stopped"
	StatusFailed    DeploymentStatus = "failed"
)

// Deployment represents a deployed Docker Compose stack
type Deployment struct {
	ID           string                 `json:"id" db:"id"`
	TemplateID   string                 `json:"template_id" db:"template_id"`
	StackName    string                 `json:"stack_name" db:"stack_name"`
	Status       DeploymentStatus       `json:"status" db:"status"`
	Config       map[string]interface{} `json:"config" db:"config"`
	NewtInjected bool                   `json:"newt_injected" db:"newt_injected"`
	TunnelURL    string                 `json:"tunnel_url" db:"tunnel_url"`
	CreatedAt    time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at" db:"updated_at"`
}

// DeploymentLog represents a log entry for a deployment
type DeploymentLog struct {
	ID           int       `json:"id" db:"id"`
	DeploymentID string    `json:"deployment_id" db:"deployment_id"`
	LogLevel     string    `json:"log_level" db:"log_level"`
	Message      string    `json:"message" db:"message"`
	Timestamp    time.Time `json:"timestamp" db:"timestamp"`
}

// DeploymentConfig holds configuration for creating a deployment
type DeploymentConfig struct {
	TemplateID      string            `json:"template_id"`
	StackName       string            `json:"stack_name"`
	Environment     map[string]string `json:"environment"`
	NewtConfig      *NewtConfig       `json:"newt_config"`
	AutoStart       bool              `json:"auto_start"`
	IncludeNewt     bool              `json:"include_newt"`
	OverrideExisting bool             `json:"override_existing"`
}

// NewtConfig holds Newt tunnel configuration
type NewtConfig struct {
	Endpoint     string            `json:"endpoint"`
	NewtID       string            `json:"newt_id"`
	Secret       string            `json:"secret"`
	Image        string            `json:"image"`
	LogLevel     string            `json:"log_level"`
	HealthFile   string            `json:"health_file"`
	CustomConfig map[string]string `json:"custom_config"`
}

// DeploymentStats represents deployment statistics
type DeploymentStats struct {
	ID             string            `json:"id"`
	StackName      string            `json:"stack_name"`
	ServiceCount   int               `json:"service_count"`
	RunningServices int              `json:"running_services"`
	CPUUsage       float64           `json:"cpu_usage"`
	MemoryUsage    int64             `json:"memory_usage"`
	MemoryLimit    int64             `json:"memory_limit"`
	NetworkRX      int64             `json:"network_rx"`
	NetworkTX      int64             `json:"network_tx"`
	VolumeInfo     []VolumeInfo      `json:"volume_info"`
}

// VolumeInfo represents information about a volume
type VolumeInfo struct {
	Name      string `json:"name"`
	MountPath string `json:"mount_path"`
	SizeBytes int64  `json:"size_bytes"`
	Driver    string `json:"driver"`
}

// Validation errors
var (
	ErrDeploymentTemplateRequired   = fmt.Errorf("template ID is required")
	ErrDeploymentStackNameRequired  = fmt.Errorf("stack name is required")
	ErrDeploymentInvalidStackName   = fmt.Errorf("invalid stack name format")
	ErrNewtConfigRequired          = fmt.Errorf("newt configuration is required when newt is enabled")
	ErrDeploymentNotFound          = fmt.Errorf("deployment not found")
)

// MarshalConfig converts config map to JSON string for database storage
func (d *Deployment) MarshalConfig() (string, error) {
	if d.Config == nil {
		return "{}", nil
	}
	data, err := json.Marshal(d.Config)
	return string(data), err
}

// UnmarshalConfig converts JSON string from database to config map
func (d *Deployment) UnmarshalConfig(data string) error {
	if data == "" || data == "null" {
		d.Config = make(map[string]interface{})
		return nil
	}
	return json.Unmarshal([]byte(data), &d.Config)
}

// Validate validates deployment configuration
func (d *Deployment) Validate() error {
	if strings.TrimSpace(d.TemplateID) == "" {
		return ErrDeploymentTemplateRequired
	}
	if strings.TrimSpace(d.StackName) == "" {
		return ErrDeploymentStackNameRequired
	}
	if !isValidStackName(d.StackName) {
		return ErrDeploymentInvalidStackName
	}
	return nil
}

// Validate validates deployment config
func (dc *DeploymentConfig) Validate() error {
	if strings.TrimSpace(dc.TemplateID) == "" {
		return ErrDeploymentTemplateRequired
	}
	if strings.TrimSpace(dc.StackName) == "" {
		return ErrDeploymentStackNameRequired
	}
	if !isValidStackName(dc.StackName) {
		return ErrDeploymentInvalidStackName
	}
	if dc.IncludeNewt && dc.NewtConfig == nil {
		return ErrNewtConfigRequired
	}
	if dc.NewtConfig != nil {
		if err := dc.NewtConfig.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Validate validates newt configuration
func (nc *NewtConfig) Validate() error {
	if strings.TrimSpace(nc.Endpoint) == "" {
		return fmt.Errorf("newt endpoint is required")
	}
	if strings.TrimSpace(nc.NewtID) == "" {
		return fmt.Errorf("newt ID is required")
	}
	if strings.TrimSpace(nc.Secret) == "" {
		return fmt.Errorf("newt secret is required")
	}
	return nil
}

// IsRunning returns true if deployment is in running state
func (d *Deployment) IsRunning() bool {
	return d.Status == StatusRunning
}

// IsPending returns true if deployment is pending
func (d *Deployment) IsPending() bool {
	return d.Status == StatusPending
}

// IsDeploying returns true if deployment is in progress
func (d *Deployment) IsDeploying() bool {
	return d.Status == StatusDeploying
}

// IsFailed returns true if deployment failed
func (d *Deployment) IsFailed() bool {
	return d.Status == StatusFailed
}

// IsStopped returns true if deployment is stopped
func (d *Deployment) IsStopped() bool {
	return d.Status == StatusStopped
}

// CanStart returns true if deployment can be started
func (d *Deployment) CanStart() bool {
	return d.Status == StatusStopped || d.Status == StatusFailed
}

// CanStop returns true if deployment can be stopped
func (d *Deployment) CanStop() bool {
	return d.Status == StatusRunning || d.Status == StatusDeploying
}

// CanRestart returns true if deployment can be restarted
func (d *Deployment) CanRestart() bool {
	return d.Status == StatusRunning || d.Status == StatusStopped
}

// CanDelete returns true if deployment can be deleted
func (d *Deployment) CanDelete() bool {
	return d.Status != StatusDeploying
}

// SetStatus updates the deployment status and timestamp
func (d *Deployment) SetStatus(status DeploymentStatus) {
	d.Status = status
	d.UpdatedAt = time.Now()
}

// GetEnvVar retrieves an environment variable from config
func (d *Deployment) GetEnvVar(key string) (string, bool) {
	if d.Config == nil {
		return "", false
	}
	if env, exists := d.Config["environment"]; exists {
		if envMap, ok := env.(map[string]interface{}); ok {
			if value, exists := envMap[key]; exists {
				if strValue, ok := value.(string); ok {
					return strValue, true
				}
			}
		}
	}
	return "", false
}

// SetEnvVar sets an environment variable in config
func (d *Deployment) SetEnvVar(key, value string) {
	if d.Config == nil {
		d.Config = make(map[string]interface{})
	}
	if _, exists := d.Config["environment"]; !exists {
		d.Config["environment"] = make(map[string]interface{})
	}
	if envMap, ok := d.Config["environment"].(map[string]interface{}); ok {
		envMap[key] = value
	}
}

// GetNewtConfig returns the newt configuration from deployment config
func (d *Deployment) GetNewtConfig() *NewtConfig {
	if d.Config == nil {
		return nil
	}
	if newtConfigInterface, exists := d.Config["newt_config"]; exists {
		if newtConfigMap, ok := newtConfigInterface.(map[string]interface{}); ok {
			// Convert map back to NewtConfig struct
			configJSON, _ := json.Marshal(newtConfigMap)
			var newtConfig NewtConfig
			if err := json.Unmarshal(configJSON, &newtConfig); err == nil {
				return &newtConfig
			}
		}
	}
	return nil
}

// GetServiceURL returns the URL for accessing a service through the tunnel
func (d *Deployment) GetServiceURL(serviceName string, port int) string {
	if d.TunnelURL == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s:%d", strings.TrimSuffix(d.TunnelURL, "/"), serviceName, port)
}

// GetComposeProjectName returns the docker-compose project name
func (d *Deployment) GetComposeProjectName() string {
	return strings.ToLower(strings.ReplaceAll(d.StackName, " ", "_"))
}

// GetDuration returns how long the deployment has been running
func (d *Deployment) GetDuration() time.Duration {
	if d.Status == StatusRunning {
		return time.Since(d.UpdatedAt)
	}
	return d.UpdatedAt.Sub(d.CreatedAt)
}

// IsOlderThan returns true if deployment is older than the specified duration
func (d *Deployment) IsOlderThan(duration time.Duration) bool {
	return time.Since(d.CreatedAt) > duration
}

// LogLevel constants
const (
	LogLevelInfo    = "info"
	LogLevelWarning = "warning"
	LogLevelError   = "error"
	LogLevelDebug   = "debug"
)

// NewDeploymentLog creates a new deployment log entry
func NewDeploymentLog(deploymentID, level, message string) *DeploymentLog {
	return &DeploymentLog{
		DeploymentID: deploymentID,
		LogLevel:     level,
		Message:      message,
		Timestamp:    time.Now(),
	}
}

// Helper function to validate stack name format
func isValidStackName(name string) bool {
	if len(name) == 0 || len(name) > 63 {
		return false
	}
	// Allow letters, numbers, hyphens, and underscores
	for _, char := range name {
		if !((char >= 'a' && char <= 'z') || 
			 (char >= 'A' && char <= 'Z') || 
			 (char >= '0' && char <= '9') || 
			 char == '-' || char == '_') {
			return false
		}
	}
	// Must start and end with alphanumeric character
	return (name[0] >= 'a' && name[0] <= 'z') || 
		   (name[0] >= 'A' && name[0] <= 'Z') || 
		   (name[0] >= '0' && name[0] <= '9')
}