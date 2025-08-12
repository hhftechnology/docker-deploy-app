package backup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"docker-deploy-app/internal/models"
)

// Storage interface for backup storage backends
type Storage interface {
	Store(backupID string, reader io.Reader) (string, error)
	Retrieve(path string) (io.ReadCloser, error)
	Delete(path string) error
	Exists(path string) (bool, error)
	Size(path string) (int64, error)
}

// LocalStorage implements local file system storage
type LocalStorage struct {
	basePath string
}

// NewLocalStorage creates a new local storage instance
func NewLocalStorage(basePath string) *LocalStorage {
	return &LocalStorage{
		basePath: basePath,
	}
}

// Store stores a backup to local filesystem
func (ls *LocalStorage) Store(backupID string, reader io.Reader) (string, error) {
	// Ensure base directory exists
	if err := os.MkdirAll(ls.basePath, 0755); err != nil {
		return "", fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Create backup file path
	filename := fmt.Sprintf("%s.tar.gz", backupID)
	filePath := filepath.Join(ls.basePath, filename)

	// Create the file
	file, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create backup file: %w", err)
	}
	defer file.Close()

	// Copy data to file
	_, err = io.Copy(file, reader)
	if err != nil {
		os.Remove(filePath) // Clean up on error
		return "", fmt.Errorf("failed to write backup data: %w", err)
	}

	return filePath, nil
}

// Retrieve retrieves a backup from local filesystem
func (ls *LocalStorage) Retrieve(path string) (io.ReadCloser, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open backup file: %w", err)
	}
	return file, nil
}

// Delete removes a backup from local filesystem
func (ls *LocalStorage) Delete(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete backup file: %w", err)
	}
	return nil
}

// Exists checks if a backup exists
func (ls *LocalStorage) Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Size returns the size of a backup file
func (ls *LocalStorage) Size(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("failed to get file info: %w", err)
	}
	return info.Size(), nil
}

// S3Storage implements S3-compatible storage (placeholder)
type S3Storage struct {
	bucket    string
	region    string
	accessKey string
	secretKey string
}

// NewS3Storage creates a new S3 storage instance
func NewS3Storage(config *models.S3Config) *S3Storage {
	return &S3Storage{
		bucket:    config.Bucket,
		region:    config.Region,
		accessKey: config.AccessKey,
		secretKey: config.SecretKey,
	}
}

// Store stores a backup to S3 (placeholder implementation)
func (s3 *S3Storage) Store(backupID string, reader io.Reader) (string, error) {
	// TODO: Implement S3 upload using AWS SDK
	return "", fmt.Errorf("S3 storage not implemented")
}

// Retrieve retrieves a backup from S3 (placeholder implementation)
func (s3 *S3Storage) Retrieve(path string) (io.ReadCloser, error) {
	// TODO: Implement S3 download using AWS SDK
	return nil, fmt.Errorf("S3 storage not implemented")
}

// Delete removes a backup from S3 (placeholder implementation)
func (s3 *S3Storage) Delete(path string) error {
	// TODO: Implement S3 delete using AWS SDK
	return fmt.Errorf("S3 storage not implemented")
}

// Exists checks if a backup exists in S3 (placeholder implementation)
func (s3 *S3Storage) Exists(path string) (bool, error) {
	// TODO: Implement S3 head object using AWS SDK
	return false, fmt.Errorf("S3 storage not implemented")
}

// Size returns the size of a backup file in S3 (placeholder implementation)
func (s3 *S3Storage) Size(path string) (int64, error) {
	// TODO: Implement S3 head object using AWS SDK
	return 0, fmt.Errorf("S3 storage not implemented")
}

// StorageManager manages different storage backends
type StorageManager struct {
	local Storage
	s3    Storage
	config *models.StorageConfig
}

// NewStorageManager creates a new storage manager
func NewStorageManager(config *models.StorageConfig) *StorageManager {
	sm := &StorageManager{
		config: config,
	}

	// Initialize local storage
	if config.LocalPath != "" {
		sm.local = NewLocalStorage(config.LocalPath)
	}

	// Initialize S3 storage
	if config.S3Config != nil {
		sm.s3 = NewS3Storage(config.S3Config)
	}

	return sm
}

// GetStorage returns the appropriate storage backend
func (sm *StorageManager) GetStorage() Storage {
	switch sm.config.Type {
	case "s3":
		if sm.s3 != nil {
			return sm.s3
		}
		fallthrough // Fall back to local if S3 not configured
	case "local":
		fallthrough
	default:
		return sm.local
	}
}

// Store stores a backup using the configured storage backend
func (sm *StorageManager) Store(backupID string, reader io.Reader) (string, error) {
	storage := sm.GetStorage()
	if storage == nil {
		return "", fmt.Errorf("no storage backend configured")
	}
	return storage.Store(backupID, reader)
}

// Retrieve retrieves a backup using the configured storage backend
func (sm *StorageManager) Retrieve(path string) (io.ReadCloser, error) {
	storage := sm.GetStorage()
	if storage == nil {
		return nil, fmt.Errorf("no storage backend configured")
	}
	return storage.Retrieve(path)
}

// Delete removes a backup using the configured storage backend
func (sm *StorageManager) Delete(path string) error {
	storage := sm.GetStorage()
	if storage == nil {
		return fmt.Errorf("no storage backend configured")
	}
	return storage.Delete(path)
}

// Exists checks if a backup exists using the configured storage backend
func (sm *StorageManager) Exists(path string) (bool, error) {
	storage := sm.GetStorage()
	if storage == nil {
		return false, fmt.Errorf("no storage backend configured")
	}
	return storage.Exists(path)
}

// Size returns the size of a backup using the configured storage backend
func (sm *StorageManager) Size(path string) (int64, error) {
	storage := sm.GetStorage()
	if storage == nil {
		return 0, fmt.Errorf("no storage backend configured")
	}
	return storage.Size(path)
}

// ListBackups lists all backups in storage
func (sm *StorageManager) ListBackups() ([]string, error) {
	// For local storage, list files in directory
	if sm.config.Type == "local" && sm.config.LocalPath != "" {
		return sm.listLocalBackups()
	}
	
	// For S3, would list objects in bucket
	return []string{}, fmt.Errorf("list not implemented for storage type: %s", sm.config.Type)
}

// listLocalBackups lists backup files in local storage
func (sm *StorageManager) listLocalBackups() ([]string, error) {
	entries, err := os.ReadDir(sm.config.LocalPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read storage directory: %w", err)
	}

	var backups []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".gz" {
			backups = append(backups, entry.Name())
		}
	}

	return backups, nil
}

// CleanupOrphanedFiles removes backup files that don't have database records
func (sm *StorageManager) CleanupOrphanedFiles(validBackupIDs []string) error {
	files, err := sm.ListBackups()
	if err != nil {
		return err
	}

	validIDs := make(map[string]bool)
	for _, id := range validBackupIDs {
		validIDs[fmt.Sprintf("%s.tar.gz", id)] = true
	}

	for _, file := range files {
		if !validIDs[file] {
			path := filepath.Join(sm.config.LocalPath, file)
			if err := sm.Delete(path); err != nil {
				fmt.Printf("Failed to delete orphaned file %s: %v\n", file, err)
			}
		}
	}

	return nil
}