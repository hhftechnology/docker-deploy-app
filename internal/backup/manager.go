package backup

import (
	"archive/tar"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/docker/docker/client"
	"docker-deploy-app/internal/models"
)

// Manager handles backup and restore operations
type Manager struct {
	db           *sql.DB
	dockerClient *client.Client
	storagePath  string
}

// NewManager creates a new backup manager
func NewManager(db *sql.DB, dockerClient *client.Client, storagePath string) *Manager {
	return &Manager{
		db:           db,
		dockerClient: dockerClient,
		storagePath:  storagePath,
	}
}

// CreateBackup creates a new backup
func (m *Manager) CreateBackup(config *models.BackupConfig) (*models.Backup, error) {
	backup := &models.Backup{
		ID:             generateBackupID(),
		Name:           config.Name,
		Type:           config.Type,
		Status:         models.BackupStatusCreating,
		IncludeVolumes: config.IncludeVolumes,
		Encrypted:      config.Encrypted,
		DeploymentIDs:  getDeploymentIDsFromConfig(config),
		CreatedAt:      time.Now(),
	}

	// Create backup directory
	backupDir := filepath.Join(m.storagePath, backup.ID)
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Save initial backup record
	if err := m.saveBackupRecord(backup); err != nil {
		return nil, fmt.Errorf("failed to save backup record: %w", err)
	}

	// Start backup process
	go m.performBackup(backup, config)

	return backup, nil
}

// RestoreBackup restores from a backup
func (m *Manager) RestoreBackup(config *models.RestoreConfig) error {
	backup, err := m.getBackup(config.BackupID)
	if err != nil {
		return fmt.Errorf("failed to get backup: %w", err)
	}

	if backup.Status != models.BackupStatusCompleted {
		return fmt.Errorf("backup is not completed")
	}

	// Start restore process
	go m.performRestore(backup, config)

	return nil
}

// ListBackups returns all backups
func (m *Manager) ListBackups() ([]*models.Backup, error) {
	query := `
		SELECT id, name, type, status, size_bytes, include_volumes, encrypted,
		       storage_path, deployment_ids, created_at, completed_at
		FROM backups ORDER BY created_at DESC`

	rows, err := m.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var backups []*models.Backup
	for rows.Next() {
		backup, err := m.scanBackup(rows)
		if err != nil {
			continue
		}
		backups = append(backups, backup)
	}

	return backups, nil
}

// GetBackup returns a specific backup
func (m *Manager) GetBackup(backupID string) (*models.Backup, error) {
	return m.getBackup(backupID)
}

// DeleteBackup removes a backup
func (m *Manager) DeleteBackup(backupID string) error {
	backup, err := m.getBackup(backupID)
	if err != nil {
		return err
	}

	// Remove backup file
	if backup.StoragePath != "" {
		os.Remove(backup.StoragePath)
	}

	// Remove backup directory
	backupDir := filepath.Join(m.storagePath, backupID)
	os.RemoveAll(backupDir)

	// Remove from database
	_, err = m.db.Exec("DELETE FROM backups WHERE id = $1", backupID)
	return err
}

// performBackup executes the backup process
func (m *Manager) performBackup(backup *models.Backup, config *models.BackupConfig) {
	defer m.updateBackupStatus(backup.ID, models.BackupStatusCompleted)

	backupDir := filepath.Join(m.storagePath, backup.ID)

	// Create deployments backup
	for _, deploymentID := range backup.DeploymentIDs {
		if err := m.backupDeployment(deploymentID, backupDir); err != nil {
			m.updateBackupStatus(backup.ID, models.BackupStatusFailed)
			return
		}
	}

	// Create metadata file
	metadata := &models.BackupMetadata{
		Version:         "1.0",
		CreatedAt:       backup.CreatedAt,
		AppVersion:      "1.0.0",
		DeploymentCount: len(backup.DeploymentIDs),
		VolumeCount:     0,
	}

	if err := m.saveMetadata(backupDir, metadata); err != nil {
		m.updateBackupStatus(backup.ID, models.BackupStatusFailed)
		return
	}

	// Create archive
	archivePath := filepath.Join(m.storagePath, backup.ID+".tar.gz")
	size, err := m.createArchive(backupDir, archivePath)
	if err != nil {
		m.updateBackupStatus(backup.ID, models.BackupStatusFailed)
		return
	}

	// Update backup record
	backup.StoragePath = archivePath
	backup.SizeBytes = size
	now := time.Now()
	backup.CompletedAt = &now

	m.updateBackupRecord(backup)

	// Clean up temporary directory
	os.RemoveAll(backupDir)
}

// performRestore executes the restore process
func (m *Manager) performRestore(backup *models.Backup, config *models.RestoreConfig) {
	restoreDir := filepath.Join(m.storagePath, "restore", backup.ID)
	defer os.RemoveAll(restoreDir)

	// Extract archive
	if err := m.extractArchive(backup.StoragePath, restoreDir); err != nil {
		return
	}

	// Restore deployments
	for _, deploymentID := range backup.DeploymentIDs {
		if config.Selective && !config.HasDeployment(deploymentID) {
			continue
		}

		if !config.TestRestore {
			m.restoreDeployment(deploymentID, restoreDir)
		}
	}
}

// backupDeployment backs up a single deployment
func (m *Manager) backupDeployment(deploymentID, backupDir string) error {
	// Get deployment info
	var stackName, templateID, configJSON string
	err := m.db.QueryRow(`
		SELECT stack_name, template_id, config 
		FROM deployments WHERE id = $1`,
		deploymentID).Scan(&stackName, &templateID, &configJSON)

	if err != nil {
		return err
	}

	deploymentDir := filepath.Join(backupDir, "deployments", deploymentID)
	if err := os.MkdirAll(deploymentDir, 0755); err != nil {
		return err
	}

	// Save deployment info
	deploymentInfo := map[string]interface{}{
		"id":          deploymentID,
		"stack_name":  stackName,
		"template_id": templateID,
		"config":      configJSON,
	}

	return m.saveJSON(filepath.Join(deploymentDir, "deployment.json"), deploymentInfo)
}

// restoreDeployment restores a single deployment
func (m *Manager) restoreDeployment(deploymentID, restoreDir string) error {
	deploymentFile := filepath.Join(restoreDir, "deployments", deploymentID, "deployment.json")
	
	var deploymentInfo map[string]interface{}
	if err := m.loadJSON(deploymentFile, &deploymentInfo); err != nil {
		return err
	}

	// TODO: Implement deployment restoration
	// 1. Create new deployment record
	// 2. Restore docker-compose files
	// 3. Deploy stack

	return nil
}

// createArchive creates a compressed archive
func (m *Manager) createArchive(sourceDir, archivePath string) (int64, error) {
	file, err := os.Create(archivePath)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	err = filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, info.Name())
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		header.Name = relPath

		if err := tarWriter.WriteHeader(header); err != nil {
			return err
		}

		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()

			if _, err := io.Copy(tarWriter, file); err != nil {
				return err
			}
		}

		return nil
	})

	if err != nil {
		return 0, err
	}

	// Get file size
	stat, err := file.Stat()
	if err != nil {
		return 0, err
	}

	return stat.Size(), nil
}

// extractArchive extracts a compressed archive
func (m *Manager) extractArchive(archivePath, destDir string) error {
	file, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzipReader.Close()

	tarReader := tar.NewReader(gzipReader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		path := filepath.Join(destDir, header.Name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}

			file, err := os.Create(path)
			if err != nil {
				return err
			}

			if _, err := io.Copy(file, tarReader); err != nil {
				file.Close()
				return err
			}
			file.Close()
		}
	}

	return nil
}

// Helper functions
func (m *Manager) saveBackupRecord(backup *models.Backup) error {
	deploymentIDsJSON, _ := backup.MarshalDeploymentIDs()
	_, err := m.db.Exec(`
		INSERT INTO backups (id, name, type, status, size_bytes, include_volumes, 
		                     encrypted, storage_path, deployment_ids, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		backup.ID, backup.Name, backup.Type, backup.Status, backup.SizeBytes,
		backup.IncludeVolumes, backup.Encrypted, backup.StoragePath,
		deploymentIDsJSON, backup.CreatedAt)
	return err
}

func (m *Manager) updateBackupRecord(backup *models.Backup) error {
	deploymentIDsJSON, _ := backup.MarshalDeploymentIDs()
	_, err := m.db.Exec(`
		UPDATE backups SET status = $1, size_bytes = $2, storage_path = $3, 
		                   deployment_ids = $4, completed_at = $5
		WHERE id = $6`,
		backup.Status, backup.SizeBytes, backup.StoragePath,
		deploymentIDsJSON, backup.CompletedAt, backup.ID)
	return err
}

func (m *Manager) updateBackupStatus(backupID string, status models.BackupStatus) {
	completedAt := sql.NullTime{}
	if status == models.BackupStatusCompleted || status == models.BackupStatusFailed {
		completedAt = sql.NullTime{Time: time.Now(), Valid: true}
	}

	m.db.Exec("UPDATE backups SET status = $1, completed_at = $2 WHERE id = $3",
		status, completedAt, backupID)
}

func (m *Manager) getBackup(backupID string) (*models.Backup, error) {
	query := `
		SELECT id, name, type, status, size_bytes, include_volumes, encrypted,
		       storage_path, deployment_ids, created_at, completed_at
		FROM backups WHERE id = $1`

	row := m.db.QueryRow(query, backupID)
	return m.scanBackup(row)
}

func (m *Manager) scanBackup(scanner interface {
	Scan(dest ...interface{}) error
}) (*models.Backup, error) {
	var backup models.Backup
	var deploymentIDsJSON string
	var completedAt sql.NullTime

	err := scanner.Scan(
		&backup.ID, &backup.Name, &backup.Type, &backup.Status, &backup.SizeBytes,
		&backup.IncludeVolumes, &backup.Encrypted, &backup.StoragePath,
		&deploymentIDsJSON, &backup.CreatedAt, &completedAt)

	if err != nil {
		return nil, err
	}

	if completedAt.Valid {
		backup.CompletedAt = &completedAt.Time
	}

	backup.UnmarshalDeploymentIDs(deploymentIDsJSON)
	return &backup, nil
}

func (m *Manager) saveJSON(path string, data interface{}) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return json.NewEncoder(file).Encode(data)
}

func (m *Manager) loadJSON(path string, data interface{}) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	return json.NewDecoder(file).Decode(data)
}

func (m *Manager) saveMetadata(backupDir string, metadata *models.BackupMetadata) error {
	return m.saveJSON(filepath.Join(backupDir, "metadata.json"), metadata)
}

func generateBackupID() string {
	return fmt.Sprintf("backup_%d", time.Now().Unix())
}

func getDeploymentIDsFromConfig(config *models.BackupConfig) []string {
	var deploymentIDs []string
	for _, deployment := range config.Deployments {
		deploymentIDs = append(deploymentIDs, deployment.ID)
	}
	return deploymentIDs
}