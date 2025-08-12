package models

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Template represents a Docker Compose template
type Template struct {
	ID            string                 `json:"id" db:"id"`
	Name          string                 `json:"name" db:"name"`
	Description   string                 `json:"description" db:"description"`
	Icon          string                 `json:"icon" db:"icon"`
	Category      string                 `json:"category" db:"category"`
	Tags          []string               `json:"tags" db:"tags"`
	RepoURL       string                 `json:"repo_url" db:"repo_url"`
	Branch        string                 `json:"branch" db:"branch"`
	Path          string                 `json:"path" db:"path"`
	Version       string                 `json:"version" db:"version"`
	Variables     []TemplateVariable     `json:"variables" db:"variables"`
	RequiresNewt  bool                   `json:"requires_newt" db:"requires_newt"`
	NewtConfig    *TemplateNewtConfig    `json:"newt_config" db:"newt_config"`
	PublisherID   string                 `json:"publisher_id" db:"publisher_id"`
	IsVerified    bool                   `json:"is_verified" db:"is_verified"`
	DownloadCount int                    `json:"download_count" db:"download_count"`
	AvgRating     float64                `json:"avg_rating" db:"avg_rating"`
	TotalRatings  int                    `json:"total_ratings" db:"total_ratings"`
	CreatedAt     time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time              `json:"updated_at" db:"updated_at"`
}

// TemplateVariable represents an environment variable for a template
type TemplateVariable struct {
	Name         string `json:"name"`
	Label        string `json:"label"`
	Description  string `json:"description"`
	Type         string `json:"type"` // text, password, number, boolean, select
	Required     bool   `json:"required"`
	DefaultValue string `json:"default_value"`
	Options      []TemplateVariableOption `json:"options,omitempty"` // For select type
	Validation   *TemplateVariableValidation `json:"validation,omitempty"`
}

// TemplateVariableOption represents an option for select-type variables
type TemplateVariableOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// TemplateVariableValidation represents validation rules for variables
type TemplateVariableValidation struct {
	MinLength *int    `json:"min_length,omitempty"`
	MaxLength *int    `json:"max_length,omitempty"`
	Pattern   *string `json:"pattern,omitempty"`
	Min       *int    `json:"min,omitempty"`
	Max       *int    `json:"max,omitempty"`
}

// TemplateNewtConfig represents newt-specific configuration for a template
type TemplateNewtConfig struct {
	AutoInject       bool              `json:"auto_inject"`
	RequiredPorts    []int             `json:"required_ports"`
	HealthCheck      *NewtHealthCheck  `json:"health_check,omitempty"`
	CustomConfig     map[string]string `json:"custom_config,omitempty"`
	NetworkMode      string            `json:"network_mode,omitempty"`
	ExposeAllPorts   bool              `json:"expose_all_ports"`
}

// NewtHealthCheck represents health check configuration for newt
type NewtHealthCheck struct {
	Enabled     bool   `json:"enabled"`
	Endpoint    string `json:"endpoint"`
	Interval    string `json:"interval"`
	Timeout     string `json:"timeout"`
	Retries     int    `json:"retries"`
	StartPeriod string `json:"start_period"`
}

// TemplateRating represents a user rating for a template
type TemplateRating struct {
	ID         int       `json:"id" db:"id"`
	TemplateID string    `json:"template_id" db:"template_id"`
	UserID     *string   `json:"user_id" db:"user_id"` // Nullable for anonymous ratings
	Rating     int       `json:"rating" db:"rating"`
	Review     string    `json:"review" db:"review"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`
}

// TemplateMetadata represents additional metadata for templates
type TemplateMetadata struct {
	Documentation string            `json:"documentation"`
	License       string            `json:"license"`
	Maintainer    string            `json:"maintainer"`
	Keywords      []string          `json:"keywords"`
	Dependencies  []string          `json:"dependencies"`
	Resources     TemplateResources `json:"resources"`
}

// TemplateResources represents resource requirements for a template
type TemplateResources struct {
	MinCPU    string `json:"min_cpu"`
	MinMemory string `json:"min_memory"`
	MinDisk   string `json:"min_disk"`
}

// Validation errors
var (
	ErrTemplateNameRequired     = fmt.Errorf("template name is required")
	ErrTemplateRepoURLRequired  = fmt.Errorf("template repository URL is required")
	ErrTemplateInvalidCategory  = fmt.Errorf("invalid template category")
	ErrTemplateInvalidVariable  = fmt.Errorf("invalid template variable")
)

// MarshalTags converts tags slice to JSON string for database storage
func (t *Template) MarshalTags() (string, error) {
	if t.Tags == nil {
		return "[]", nil
	}
	data, err := json.Marshal(t.Tags)
	return string(data), err
}

// UnmarshalTags converts JSON string from database to tags slice
func (t *Template) UnmarshalTags(data string) error {
	if data == "" || data == "null" {
		t.Tags = []string{}
		return nil
	}
	return json.Unmarshal([]byte(data), &t.Tags)
}

// MarshalVariables converts variables slice to JSON string for database storage
func (t *Template) MarshalVariables() (string, error) {
	if t.Variables == nil {
		return "[]", nil
	}
	data, err := json.Marshal(t.Variables)
	return string(data), err
}

// UnmarshalVariables converts JSON string from database to variables slice
func (t *Template) UnmarshalVariables(data string) error {
	if data == "" || data == "null" {
		t.Variables = []TemplateVariable{}
		return nil
	}
	return json.Unmarshal([]byte(data), &t.Variables)
}

// MarshalNewtConfig converts newt config to JSON string for database storage
func (t *Template) MarshalNewtConfig() (string, error) {
	if t.NewtConfig == nil {
		return "{}", nil
	}
	data, err := json.Marshal(t.NewtConfig)
	return string(data), err
}

// UnmarshalNewtConfig converts JSON string from database to newt config
func (t *Template) UnmarshalNewtConfig(data string) error {
	if data == "" || data == "null" {
		t.NewtConfig = nil
		return nil
	}
	return json.Unmarshal([]byte(data), &t.NewtConfig)
}

// Validate validates the template data
func (t *Template) Validate() error {
	if strings.TrimSpace(t.Name) == "" {
		return ErrTemplateNameRequired
	}

	if strings.TrimSpace(t.RepoURL) == "" {
		return ErrTemplateRepoURLRequired
	}

	// Validate category
	validCategories := []string{
		"web", "database", "monitoring", "networking", "development",
		"ai-ml", "security", "analytics", "cms", "e-commerce", "other",
	}
	if t.Category != "" && !contains(validCategories, t.Category) {
		return ErrTemplateInvalidCategory
	}

	// Validate variables
	for _, variable := range t.Variables {
		if err := variable.Validate(); err != nil {
			return fmt.Errorf("%w: %v", ErrTemplateInvalidVariable, err)
		}
	}

	return nil
}

// Validate validates a template variable
func (v *TemplateVariable) Validate() error {
	if strings.TrimSpace(v.Name) == "" {
		return fmt.Errorf("variable name is required")
	}

	validTypes := []string{"text", "password", "number", "boolean", "select"}
	if !contains(validTypes, v.Type) {
		return fmt.Errorf("invalid variable type: %s", v.Type)
	}

	if v.Type == "select" && len(v.Options) == 0 {
		return fmt.Errorf("select type variables must have options")
	}

	return nil
}

// GetVariable returns a variable by name
func (t *Template) GetVariable(name string) *TemplateVariable {
	for _, variable := range t.Variables {
		if variable.Name == name {
			return &variable
		}
	}
	return nil
}

// HasTag checks if template has a specific tag
func (t *Template) HasTag(tag string) bool {
	for _, t := range t.Tags {
		if strings.EqualFold(t, tag) {
			return true
		}
	}
	return false
}

// AddTag adds a tag to the template if it doesn't exist
func (t *Template) AddTag(tag string) {
	if !t.HasTag(tag) {
		t.Tags = append(t.Tags, tag)
	}
}

// RemoveTag removes a tag from the template
func (t *Template) RemoveTag(tag string) {
	for i, t := range t.Tags {
		if strings.EqualFold(t, tag) {
			t.Tags = append(t.Tags[:i], t.Tags[i+1:]...)
			break
		}
	}
}

// UpdateRating updates the template's average rating
func (t *Template) UpdateRating(newRating int, isUpdate bool, oldRating int) {
	if isUpdate {
		// Recalculate average when updating existing rating
		total := t.AvgRating * float64(t.TotalRatings)
		total = total - float64(oldRating) + float64(newRating)
		t.AvgRating = total / float64(t.TotalRatings)
	} else {
		// Add new rating
		total := t.AvgRating * float64(t.TotalRatings)
		t.TotalRatings++
		t.AvgRating = (total + float64(newRating)) / float64(t.TotalRatings)
	}
}

// IncrementDownloadCount increments the download counter
func (t *Template) IncrementDownloadCount() {
	t.DownloadCount++
	t.UpdatedAt = time.Now()
}

// IsPopular returns true if the template is considered popular
func (t *Template) IsPopular() bool {
	return t.DownloadCount >= 100 || (t.AvgRating >= 4.0 && t.TotalRatings >= 10)
}

// GetFullRepoPath returns the complete path to the template in the repository
func (t *Template) GetFullRepoPath() string {
	if t.Path == "" || t.Path == "/" {
		return t.RepoURL
	}
	return fmt.Sprintf("%s/tree/%s/%s", strings.TrimSuffix(t.RepoURL, ".git"), t.Branch, strings.TrimPrefix(t.Path, "/"))
}

// Helper function to check if slice contains string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}