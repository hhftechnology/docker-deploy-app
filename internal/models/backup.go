package models

import (
	"encoding/json"
	"time"
)

// BackupStatus represents the current status of a backup
type BackupStatus string

const (
	BackupStatusCreating  BackupStatus = "creating"
	BackupStatusCompleted BackupStatus = "completed"
	BackupStatusFailed    BackupStatus = "failed"
)

// BackupType represents the type of backup
type BackupType string

const (
	BackupTypeManual    BackupType = "manual"
	BackupTypeScheduled BackupType = "scheduled"
	BackupTypeAuto      BackupType = "auto"
)

// Backup represents a backup record
type Backup struct {
	ID             string         `json:"id" db:"id"`
	Name           string         `json:"name" db:"name"`
	Type           BackupType     `json:"type" db:"type"`
	Status         BackupStatus   `json:"status" db:"status"`
	SizeBytes      int64          `json:"size_bytes" db:"size_bytes"`
	IncludeVolumes bool           `json:"include_volumes" db:"include_volumes"`
	Encrypted      bool           `json:"encrypted" db:"encrypted"`
	StoragePath    string         `json:"storage_path" db:"storage_path"`
	DeploymentIDs  []string       `json:"deployment_ids" db:"deployment_ids"`
	CreatedAt      time.Time      `json:"created_at" db:"created_at"`
	CompletedAt    *time.Time     `json:"completed_at" db:"completed_at"`
}

// BackupSchedule represents a scheduled backup configuration
type BackupSchedule struct {
	ID             int        `json:"id" db:"id"`
	Name           string     `json:"name" db:"name"`
	CronExpression string     `json:"cron_expression" db:"cron_expression"`
	IncludeVolumes bool       `json:"include_volumes" db:"include_volumes"`
	Encrypt        bool       `json:"encrypt" db:"encrypt"`
	Enabled        bool       `json:"enabled" db:"enabled"`
	LastRun        *time.Time `json:"last_run" db:"last_run"`
	NextRun        *time.Time `json:"next_run" db:"next_run"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
}

// BackupConfig holds configuration for creating a backup
type BackupConfig struct {
	Name            string                 `json:"name"`
	Type            BackupType             `json:"type"`
	IncludeVolumes  bool                   `json:"include_volumes"`
	Encrypted       bool                   `json:"encrypted"`
	Deployments     []DeploymentBackup     `json:"deployments"`
	ComposeFiles    map[string]string      `json:"compose_files"`
	EnvConfigs      map[string]interface{} `json:"env_configs"`
	NewtConfigs     map[string]interface{} `json:"newt_configs"`
	StorageConfig   *StorageConfig         `json:"storage_config,omitempty"`
}

// DeploymentBackup represents backup data for a single deployment
type DeploymentBackup struct {
	ID         string            `json:"id"`
	StackName  string            `json:"stack_name"`
	Services   []ServiceBackup   `json:"services"`
	Networks   []string          `json:"networks"`
	Volumes    []VolumeBackup    `json:"volumes"`
	Config     map[string]string `json:"config"`
}

// ServiceBackup represents backup data for a service
type ServiceBackup struct {
	Name        string            `json:"name"`
	Image       string            `json:"image"`
	Environment map[string]string `json:"environment"`
	Labels      map[string]string `json:"labels"`
	Ports       []string          `json:"ports"`
	Volumes     []string          `json:"volumes"`
	Networks    []string          `json:"networks"`
}

// VolumeBackup represents backup data for a volume
type VolumeBackup struct {
	Name       string `json:"name"`
	Driver     string `json:"driver"`
	MountPoint string `json:"mount_point"`
	DataPath   string `json:"data_path"` // Path in backup archive
	SizeBytes  int64  `json:"size_bytes"`
}

// StorageConfig represents storage configuration for backups
type StorageConfig struct {
	Type        string     `json:"type"` // local, s3
	LocalPath   string     `json:"local_path,omitempty"`
	S3Config    *S3Config  `json:"s3_config,omitempty"`
}

// RestoreConfig holds configuration for restoring from a backup
type RestoreConfig struct {
	BackupID       string   `json:"backup_id"`
	Selective      bool     `json:"selective"`
	DeploymentIDs  []string `json:"deployment_ids,omitempty"`
	OverwriteExisting bool  `json:"overwrite_existing"`
	RestoreVolumes bool     `json:"restore_volumes"`
	TestRestore    bool     `json:"test_restore"`
}

// BackupMetadata contains metadata about a backup
type BackupMetadata struct {
	Version       string                 `json:"version"`
	CreatedAt     time.Time              `json:"created_at"`
	AppVersion    string                 `json:"app_version"`
	TotalSize     int64                  `json:"total_size"`
	DeploymentCount int                  `json:"deployment_count"`
	VolumeCount   int                    `json:"volume_count"`
	EncryptionKey string                 `json:"encryption_key,omitempty"`
	Checksum      string                 `json:"checksum"`
	Extra         map[string]interface{} `json:"extra,omitempty"`
}

// MarshalDeploymentIDs converts deployment IDs slice to JSON string for database storage
func (b *Backup) MarshalDeploymentIDs() (string, error) {
	if b.DeploymentIDs == nil {
		return "[]", nil
	}
	data, err := json.Marshal(b.DeploymentIDs)
	return string(data), err
}

// UnmarshalDeploymentIDs converts JSON string from database to deployment IDs slice
func (b *Backup) UnmarshalDeploymentIDs(data string) error {
	if data == "" {
		b.DeploymentIDs = []string{}
		return nil
	}
	return json.Unmarshal([]byte(data), &b.DeploymentIDs)
}

// IsCompleted returns true if backup is completed
func (b *Backup) IsCompleted() bool {
	return b.Status == BackupStatusCompleted
}

// IsFailed returns true if backup failed
func (b *Backup) IsFailed() bool {
	return b.Status == BackupStatusFailed
}

// GetFormattedSize returns human-readable size string
func (b *Backup) GetFormattedSize() string {
	return formatBytes(b.SizeBytes)
}

// GetDuration returns backup duration if completed
func (b *Backup) GetDuration() time.Duration {
	if b.CompletedAt == nil {
		return 0
	}
	return b.CompletedAt.Sub(b.CreatedAt)
}

// Validate validates backup configuration
func (bc *BackupConfig) Validate() error {
	if bc.Name == "" {
		return ErrBackupNameRequired
	}
	if len(bc.Deployments) == 0 {
		return ErrBackupNoDeployments
	}
	return nil
}

// GetDeploymentCount returns the number of deployments in backup
func (bc *BackupConfig) GetDeploymentCount() int {
	return len(bc.Deployments)
}

// GetTotalVolumeCount returns total number of volumes across all deployments
func (bc *BackupConfig) GetTotalVolumeCount() int {
	count := 0
	for _, deployment := range bc.Deployments {
		count += len(deployment.Volumes)
	}
	return count
}

// IsActive returns true if schedule is enabled
func (bs *BackupSchedule) IsActive() bool {
	return bs.Enabled
}

// IsOverdue returns true if schedule is overdue for execution
func (bs *BackupSchedule) IsOverdue() bool {
	if bs.NextRun == nil {
		return false
	}
	return time.Now().After(*bs.NextRun)
}

// UpdateNextRun updates the next run time based on cron expression
func (bs *BackupSchedule) UpdateNextRun() error {
	// This would use a cron parser to calculate next run time
	// Placeholder implementation
	if bs.NextRun == nil {
		nextRun := time.Now().Add(24 * time.Hour)
		bs.NextRun = &nextRun
	}
	return nil
}

// Validate validates restore configuration
func (rc *RestoreConfig) Validate() error {
	if rc.BackupID == "" {
		return ErrRestoreBackupRequired
	}
	if rc.Selective && len(rc.DeploymentIDs) == 0 {
		return ErrRestoreNoDeployments
	}
	return nil
}

// HasDeployment checks if a deployment ID is included in selective restore
func (rc *RestoreConfig) HasDeployment(deploymentID string) bool {
	if !rc.Selective {
		return true
	}
	for _, id := range rc.DeploymentIDs {
		if id == deploymentID {
			return true
		}
	}
	return false
}	