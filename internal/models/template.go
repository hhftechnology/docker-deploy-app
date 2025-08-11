package models

import (
	"time"
	"encoding/json"
)

// Template represents a Docker Compose template
type Template struct {
	ID              string                 `json:"id" db:"id"`
	Name            string                 `json:"name" db:"name"`
	Description     string                 `json:"description" db:"description"`
	Icon            string                 `json:"icon" db:"icon"`
	Category        string                 `json:"category" db:"category"`
	Tags            []string               `json:"tags" db:"tags"`
	RepoURL         string                 `json:"repo_url" db:"repo_url"`
	Branch          string                 `json:"branch" db:"branch"`
	Path            string                 `json:"path" db:"path"`
	Version         string                 `json:"version" db:"version"`
	Variables       []EnvVar               `json:"variables" db:"variables"`
	RequiresNewt    bool                   `json:"requires_newt" db:"requires_newt"`
	NewtConfig      *NewtRequirements      `json:"newt_config,omitempty" db:"newt_config"`
	PublisherID     string                 `json:"publisher_id" db:"publisher_id"`
	IsVerified      bool                   `json:"is_verified" db:"is_verified"`
	DownloadCount   int                    `json:"download_count" db:"download_count"`
	AvgRating       float64                `json:"avg_rating" db:"avg_rating"`
	TotalRatings    int                    `json:"total_ratings" db:"total_ratings"`
	CreatedAt       time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at" db:"updated_at"`
}

// EnvVar represents an environment variable configuration
type EnvVar struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Default     string `json:"default"`
	Required    bool   `json:"required"`
	Type        string `json:"type"` // text, password, number, boolean
}

// NewtRequirements defines newt-specific requirements for a template
type NewtRequirements struct {
	MinVersion string   `json:"min_version"`
	Features   []string `json:"features"`
	Networks   []string `json:"networks"`
}

// TemplateRating represents a user rating for a template
type TemplateRating struct {
	ID           int       `json:"id" db:"id"`
	TemplateID   string    `json:"template_id" db:"template_id"`
	UserID       string    `json:"user_id" db:"user_id"`
	Rating       int       `json:"rating" db:"rating"`
	Review       string    `json:"review" db:"review"`
	HelpfulCount int       `json:"helpful_count" db:"helpful_count"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time `json:"updated_at" db:"updated_at"`
}

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
	if data == "" {
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
	if data == "" {
		t.Variables = []EnvVar{}
		return nil
	}
	return json.Unmarshal([]byte(data), &t.Variables)
}

// MarshalNewtConfig converts newt config to JSON string for database storage
func (t *Template) MarshalNewtConfig() (string, error) {
	if t.NewtConfig == nil {
		return "", nil
	}
	data, err := json.Marshal(t.NewtConfig)
	return string(data), err
}

// UnmarshalNewtConfig converts JSON string from database to newt config
func (t *Template) UnmarshalNewtConfig(data string) error {
	if data == "" {
		t.NewtConfig = nil
		return nil
	}
	return json.Unmarshal([]byte(data), &t.NewtConfig)
}

// Validate validates template data
func (t *Template) Validate() error {
	if t.Name == "" {
		return ErrTemplateNameRequired
	}
	if t.RepoURL == "" {
		return ErrTemplateRepoRequired
	}
	if t.Category == "" {
		return ErrTemplateCategoryRequired
	}
	return nil
}

// GetRatingDistribution returns rating distribution (1-5 stars)
func (t *Template) GetRatingDistribution() map[int]int {
	// This would be calculated from the database
	// Placeholder implementation
	return map[int]int{
		5: int(float64(t.TotalRatings) * 0.4),
		4: int(float64(t.TotalRatings) * 0.3),
		3: int(float64(t.TotalRatings) * 0.2),
		2: int(float64(t.TotalRatings) * 0.08),
		1: int(float64(t.TotalRatings) * 0.02),
	}
}

// IsPopular returns true if template has significant usage
func (t *Template) IsPopular() bool {
	return t.DownloadCount >= 100 && t.TotalRatings >= 10 && t.AvgRating >= 4.0
}

// IsTrending returns true if template has recent activity
func (t *Template) IsTrending() bool {
	// This would check recent deployments from database
	// Placeholder implementation
	return t.DownloadCount >= 50
}