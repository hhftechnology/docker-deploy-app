package backup

import (
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/robfig/cron/v3"
	"docker-deploy-app/internal/models"
)

// Scheduler manages scheduled backups
type Scheduler struct {
	db      *sql.DB
	manager *Manager
	cron    *cron.Cron
	jobs    map[int]cron.EntryID
}

// NewScheduler creates a new backup scheduler
func NewScheduler(db *sql.DB, manager *Manager) *Scheduler {
	return &Scheduler{
		db:      db,
		manager: manager,
		cron:    cron.New(),
		jobs:    make(map[int]cron.EntryID),
	}
}

// Start starts the scheduler
func (s *Scheduler) Start() error {
	// Load existing schedules
	if err := s.loadSchedules(); err != nil {
		return err
	}

	s.cron.Start()
	log.Println("Backup scheduler started")
	return nil
}

// Stop stops the scheduler
func (s *Scheduler) Stop() {
	s.cron.Stop()
	log.Println("Backup scheduler stopped")
}

// AddSchedule adds a new backup schedule
func (s *Scheduler) AddSchedule(schedule *models.BackupSchedule) error {
	// Calculate next run time
	if err := schedule.UpdateNextRun(); err != nil {
		return err
	}

	// Save to database
	result, err := s.db.Exec(`
		INSERT INTO backup_schedules (name, cron_expression, include_volumes, encrypt, enabled, next_run, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		schedule.Name, schedule.CronExpression, schedule.IncludeVolumes,
		schedule.Encrypt, schedule.Enabled, schedule.NextRun, schedule.CreatedAt)

	if err != nil {
		return err
	}

	// Get the ID
	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	schedule.ID = int(id)

	// Add to cron if enabled
	if schedule.Enabled {
		return s.addCronJob(schedule)
	}

	return nil
}

// UpdateSchedule updates an existing schedule
func (s *Scheduler) UpdateSchedule(schedule *models.BackupSchedule) error {
	// Remove existing cron job
	if entryID, exists := s.jobs[schedule.ID]; exists {
		s.cron.Remove(entryID)
		delete(s.jobs, schedule.ID)
	}

	// Update next run time
	if err := schedule.UpdateNextRun(); err != nil {
		return err
	}

	// Update database
	_, err := s.db.Exec(`
		UPDATE backup_schedules 
		SET name = $1, cron_expression = $2, include_volumes = $3, encrypt = $4, 
		    enabled = $5, next_run = $6
		WHERE id = $7`,
		schedule.Name, schedule.CronExpression, schedule.IncludeVolumes,
		schedule.Encrypt, schedule.Enabled, schedule.NextRun, schedule.ID)

	if err != nil {
		return err
	}

	// Add to cron if enabled
	if schedule.Enabled {
		return s.addCronJob(schedule)
	}

	return nil
}

// RemoveSchedule removes a backup schedule
func (s *Scheduler) RemoveSchedule(scheduleID int) error {
	// Remove cron job
	if entryID, exists := s.jobs[scheduleID]; exists {
		s.cron.Remove(entryID)
		delete(s.jobs, scheduleID)
	}

	// Remove from database
	_, err := s.db.Exec("DELETE FROM backup_schedules WHERE id = $1", scheduleID)
	return err
}

// GetSchedules returns all backup schedules
func (s *Scheduler) GetSchedules() ([]*models.BackupSchedule, error) {
	query := `
		SELECT id, name, cron_expression, include_volumes, encrypt, enabled,
		       last_run, next_run, created_at
		FROM backup_schedules ORDER BY created_at DESC`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []*models.BackupSchedule
	for rows.Next() {
		schedule, err := s.scanSchedule(rows)
		if err != nil {
			continue
		}
		schedules = append(schedules, schedule)
	}

	return schedules, nil
}

// loadSchedules loads all schedules from database and adds them to cron
func (s *Scheduler) loadSchedules() error {
	schedules, err := s.GetSchedules()
	if err != nil {
		return err
	}

	for _, schedule := range schedules {
		if schedule.Enabled {
			if err := s.addCronJob(schedule); err != nil {
				log.Printf("Failed to add cron job for schedule %d: %v", schedule.ID, err)
			}
		}
	}

	return nil
}

// addCronJob adds a schedule to the cron scheduler
func (s *Scheduler) addCronJob(schedule *models.BackupSchedule) error {
	entryID, err := s.cron.AddFunc(schedule.CronExpression, func() {
		s.executeScheduledBackup(schedule)
	})

	if err != nil {
		return err
	}

	s.jobs[schedule.ID] = entryID
	return nil
}

// executeScheduledBackup executes a scheduled backup
func (s *Scheduler) executeScheduledBackup(schedule *models.BackupSchedule) {
	log.Printf("Executing scheduled backup: %s", schedule.Name)

	// Get all active deployments
	deploymentIDs, err := s.getActiveDeployments()
	if err != nil {
		log.Printf("Failed to get active deployments: %v", err)
		return
	}

	if len(deploymentIDs) == 0 {
		log.Printf("No active deployments to backup")
		return
	}

	// Create backup config
	config := &models.BackupConfig{
		Name:           fmt.Sprintf("%s_%s", schedule.Name, time.Now().Format("20060102_150405")),
		Type:           models.BackupTypeScheduled,
		IncludeVolumes: schedule.IncludeVolumes,
		Encrypted:      schedule.Encrypt,
		Deployments:    s.createDeploymentBackups(deploymentIDs),
	}

	// Create backup
	backup, err := s.manager.CreateBackup(config)
	if err != nil {
		log.Printf("Failed to create scheduled backup: %v", err)
		return
	}

	// Update schedule last run time
	now := time.Now()
	schedule.LastRun = &now
	schedule.UpdateNextRun()

	s.db.Exec(`
		UPDATE backup_schedules 
		SET last_run = $1, next_run = $2 
		WHERE id = $3`,
		schedule.LastRun, schedule.NextRun, schedule.ID)

	log.Printf("Scheduled backup created: %s (ID: %s)", backup.Name, backup.ID)
}

// getActiveDeployments returns all active deployment IDs
func (s *Scheduler) getActiveDeployments() ([]string, error) {
	query := "SELECT id FROM deployments WHERE status = 'running'"
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deploymentIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		deploymentIDs = append(deploymentIDs, id)
	}

	return deploymentIDs, nil
}

// createDeploymentBackups creates deployment backup entries
func (s *Scheduler) createDeploymentBackups(deploymentIDs []string) []models.DeploymentBackup {
	var deployments []models.DeploymentBackup

	for _, id := range deploymentIDs {
		var stackName string
		s.db.QueryRow("SELECT stack_name FROM deployments WHERE id = $1", id).Scan(&stackName)

		deployment := models.DeploymentBackup{
			ID:        id,
			StackName: stackName,
			Services:  []models.ServiceBackup{},
			Networks:  []string{},
			Volumes:   []models.VolumeBackup{},
			Config:    make(map[string]string),
		}

		deployments = append(deployments, deployment)
	}

	return deployments
}

// scanSchedule scans a backup schedule from database row
func (s *Scheduler) scanSchedule(scanner interface {
	Scan(dest ...interface{}) error
}) (*models.BackupSchedule, error) {
	var schedule models.BackupSchedule
	var lastRun, nextRun sql.NullTime

	err := scanner.Scan(
		&schedule.ID, &schedule.Name, &schedule.CronExpression,
		&schedule.IncludeVolumes, &schedule.Encrypt, &schedule.Enabled,
		&lastRun, &nextRun, &schedule.CreatedAt)

	if err != nil {
		return nil, err
	}

	if lastRun.Valid {
		schedule.LastRun = &lastRun.Time
	}
	if nextRun.Valid {
		schedule.NextRun = &nextRun.Time
	}

	return &schedule, nil
}

// CleanupOldBackups removes old backups based on retention policy
func (s *Scheduler) CleanupOldBackups(retention *models.RetentionConfig) error {
	// Daily backups retention
	cutoffDaily := time.Now().AddDate(0, 0, -retention.Daily)
	_, err := s.db.Exec(`
		DELETE FROM backups 
		WHERE type = 'scheduled' 
		AND created_at < $1 
		AND strftime('%H', created_at) != '00'`, // Keep daily backups taken at midnight
		cutoffDaily)

	if err != nil {
		return err
	}

	// Weekly backups retention
	cutoffWeekly := time.Now().AddDate(0, 0, -retention.Weekly*7)
	_, err = s.db.Exec(`
		DELETE FROM backups 
		WHERE type = 'scheduled' 
		AND created_at < $1 
		AND strftime('%w', created_at) != '0'`, // Keep weekly backups taken on Sunday
		cutoffWeekly)

	if err != nil {
		return err
	}

	// Monthly backups retention
	cutoffMonthly := time.Now().AddDate(0, -retention.Monthly, 0)
	_, err = s.db.Exec(`
		DELETE FROM backups 
		WHERE type = 'scheduled' 
		AND created_at < $1 
		AND strftime('%d', created_at) != '01'`, // Keep monthly backups taken on 1st
		cutoffMonthly)

	return err
}