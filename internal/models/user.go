package models

import (
	"time"
	"crypto/rand"
	"encoding/hex"
)

// UserRole represents a user's role in the system
type UserRole string

const (
	RoleViewer   UserRole = "viewer"
	RoleOperator UserRole = "operator"
	RoleAdmin    UserRole = "admin"
)

// User represents a user account
type User struct {
	ID          string    `json:"id" db:"id"`
	Username    string    `json:"username" db:"username"`
	Email       string    `json:"email" db:"email"`
	DisplayName string    `json:"display_name" db:"display_name"`
	Role        UserRole  `json:"role" db:"role"`
	Active      bool      `json:"active" db:"active"`
	LastLogin   *time.Time `json:"last_login" db:"last_login"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at" db:"updated_at"`
}

// Session represents a user session
type Session struct {
	ID        string    `json:"id" db:"id"`
	UserID    string    `json:"user_id" db:"user_id"`
	Token     string    `json:"token" db:"token"`
	ExpiresAt time.Time `json:"expires_at" db:"expires_at"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	IPAddress string    `json:"ip_address" db:"ip_address"`
	UserAgent string    `json:"user_agent" db:"user_agent"`
}

// APIKey represents an API key for programmatic access
type APIKey struct {
	ID          string    `json:"id" db:"id"`
	UserID      string    `json:"user_id" db:"user_id"`
	Name        string    `json:"name" db:"name"`
	Key         string    `json:"key" db:"key"`
	Permissions []string  `json:"permissions" db:"permissions"`
	Active      bool      `json:"active" db:"active"`
	LastUsed    *time.Time `json:"last_used" db:"last_used"`
	ExpiresAt   *time.Time `json:"expires_at" db:"expires_at"`
	CreatedAt   time.Time `json:"created_at" db:"created_at"`
}

// UserPreferences represents user-specific preferences
type UserPreferences struct {
	UserID             string `json:"user_id" db:"user_id"`
	Theme              string `json:"theme" db:"theme"` // light, dark, auto
	Language           string `json:"language" db:"language"`
	NotificationsEmail bool   `json:"notifications_email" db:"notifications_email"`
	NotificationsWeb   bool   `json:"notifications_web" db:"notifications_web"`
	DefaultView        string `json:"default_view" db:"default_view"`
	UpdatedAt          time.Time `json:"updated_at" db:"updated_at"`
}

// Permission represents a permission in the system
type Permission string

const (
	PermissionViewTemplates   Permission = "view_templates"
	PermissionDeployTemplates Permission = "deploy_templates"
	PermissionManageStacks    Permission = "manage_stacks"
	PermissionViewLogs        Permission = "view_logs"
	PermissionManageBackups   Permission = "manage_backups"
	PermissionManageUsers     Permission = "manage_users"
	PermissionSystemConfig    Permission = "system_config"
	PermissionAPIAccess       Permission = "api_access"
)

// Validate validates user data
func (u *User) Validate() error {
	if u.Username == "" {
		return ErrUserUsernameRequired
	}
	if u.Email == "" {
		return ErrUserEmailRequired
	}
	if u.Role == "" {
		u.Role = RoleViewer
	}
	return nil
}

// IsActive returns true if user account is active
func (u *User) IsActive() bool {
	return u.Active
}

// IsAdmin returns true if user has admin role
func (u *User) IsAdmin() bool {
	return u.Role == RoleAdmin
}

// CanManageUsers returns true if user can manage other users
func (u *User) CanManageUsers() bool {
	return u.Role == RoleAdmin
}

// CanManageSystem returns true if user can manage system configuration
func (u *User) CanManageSystem() bool {
	return u.Role == RoleAdmin
}

// CanDeploy returns true if user can deploy templates
func (u *User) CanDeploy() bool {
	return u.Role == RoleOperator || u.Role == RoleAdmin
}

// CanManageStacks returns true if user can manage stacks
func (u *User) CanManageStacks() bool {
	return u.Role == RoleOperator || u.Role == RoleAdmin
}

// GetPermissions returns all permissions for the user's role
func (u *User) GetPermissions() []Permission {
	switch u.Role {
	case RoleAdmin:
		return []Permission{
			PermissionViewTemplates,
			PermissionDeployTemplates,
			PermissionManageStacks,
			PermissionViewLogs,
			PermissionManageBackups,
			PermissionManageUsers,
			PermissionSystemConfig,
			PermissionAPIAccess,
		}
	case RoleOperator:
		return []Permission{
			PermissionViewTemplates,
			PermissionDeployTemplates,
			PermissionManageStacks,
			PermissionViewLogs,
			PermissionManageBackups,
			PermissionAPIAccess,
		}
	case RoleViewer:
		return []Permission{
			PermissionViewTemplates,
			PermissionViewLogs,
		}
	default:
		return []Permission{}
	}
}

// HasPermission checks if user has a specific permission
func (u *User) HasPermission(permission Permission) bool {
	permissions := u.GetPermissions()
	for _, p := range permissions {
		if p == permission {
			return true
		}
	}
	return false
}

// UpdateLastLogin updates the user's last login time
func (u *User) UpdateLastLogin() {
	now := time.Now()
	u.LastLogin = &now
	u.UpdatedAt = now
}

// IsExpired returns true if session has expired
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// IsValid returns true if session is valid and not expired
func (s *Session) IsValid() bool {
	return !s.IsExpired()
}

// Extend extends the session expiration time
func (s *Session) Extend(duration time.Duration) {
	s.ExpiresAt = time.Now().Add(duration)
}

// GenerateToken generates a new session token
func (s *Session) GenerateToken() error {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return err
	}
	s.Token = hex.EncodeToString(bytes)
	return nil
}

// IsActive returns true if API key is active
func (ak *APIKey) IsActive() bool {
	return ak.Active
}

// IsExpired returns true if API key has expired
func (ak *APIKey) IsExpired() bool {
	if ak.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*ak.ExpiresAt)
}

// IsValid returns true if API key is valid and not expired
func (ak *APIKey) IsValid() bool {
	return ak.IsActive() && !ak.IsExpired()
}

// HasPermission checks if API key has a specific permission
func (ak *APIKey) HasPermission(permission string) bool {
	for _, p := range ak.Permissions {
		if p == permission {
			return true
		}
	}
	return false
}

// UpdateLastUsed updates the last used timestamp
func (ak *APIKey) UpdateLastUsed() {
	now := time.Now()
	ak.LastUsed = &now
}

// GenerateKey generates a new API key
func (ak *APIKey) GenerateKey() error {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return err
	}
	ak.Key = "dpapp_" + hex.EncodeToString(bytes)
	return nil
}

// GetDefaultPreferences returns default user preferences
func GetDefaultPreferences() *UserPreferences {
	return &UserPreferences{
		Theme:              "auto",
		Language:           "en",
		NotificationsEmail: true,
		NotificationsWeb:   true,
		DefaultView:        "marketplace",
		UpdatedAt:          time.Now(),
	}
}

// Update updates user preferences
func (up *UserPreferences) Update(updates map[string]interface{}) {
	if theme, ok := updates["theme"].(string); ok {
		up.Theme = theme
	}
	if language, ok := updates["language"].(string); ok {
		up.Language = language
	}
	if notifEmail, ok := updates["notifications_email"].(bool); ok {
		up.NotificationsEmail = notifEmail
	}
	if notifWeb, ok := updates["notifications_web"].(bool); ok {
		up.NotificationsWeb = notifWeb
	}
	if defaultView, ok := updates["default_view"].(string); ok {
		up.DefaultView = defaultView
	}
	up.UpdatedAt = time.Now()
}

// CreateAnonymousUser creates an anonymous user for systems without authentication
func CreateAnonymousUser() *User {
	return &User{
		ID:          "anonymous",
		Username:    "anonymous",
		Email:       "anonymous@localhost",
		DisplayName: "Anonymous User",
		Role:        RoleAdmin, // Full access when auth is disabled
		Active:      true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}