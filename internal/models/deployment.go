package models

import (
	"encoding/json"
	"fmt"
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
}

// NewtConfig holds Newt tunnel configuration
type NewtConfig struct {
	Endpoint string `json:"endpoint"`
	NewtID   string `json:"newt_id"`
	Secret   string `json:"secret"`
	Image    string `json:"image"`
}

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
	if data == "" {
		d.Config = make(map[string]interface{})
		return nil
	}
	return json.Unmarshal([]byte(data), &d.Config)
}

// Validate validates deployment configuration
func (d *Deployment) Validate() error {
	if d.TemplateID == "" {
		return ErrDeploymentTemplateRequired
	}
	if d.StackName == "" {
		return ErrDeploymentStackNameRequired
	}
	return nil
}

// Validate validates deployment config
func (dc *DeploymentConfig) Validate() error {
	if dc.TemplateID == "" {
		return ErrDeploymentTemplateRequired
	}
	if dc.StackName == "" {
		return ErrDeploymentStackNameRequired
	}
	if dc.IncludeNewt && dc.NewtConfig == nil {
		return ErrNewtConfigRequired
	}
	return nil
}

// IsRunning returns true if deployment is in running state
func (d *Deployment) IsRunning() bool {
	return d.Status == StatusRunning
}

// IsFailed returns true if deployment failed
func (d *Deployment) IsFailed() bool {
	return d.Status == StatusFailed
}

// CanStart returns true if deployment can be started
func (d *Deployment) CanStart() bool {
	return d.Status == StatusStopped || d.Status == StatusFailed
}

// CanStop returns true if deployment can be stopped
func (d *Deployment) CanStop() bool {
	return d.Status == StatusRunning || d.Status == StatusDeploying
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

// GetServiceURL returns the URL for accessing a service through the tunnel
func (d *Deployment) GetServiceURL(serviceName string, port int) string {
	if d.TunnelURL == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s:%d", d.TunnelURL, serviceName, port)
}

// LogLevel constants
const (
	LogLevelInfo    = "info"
	LogLevelWarning = "warning"
	LogLevelError   = "error"
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