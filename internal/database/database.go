package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// Init initializes the SQLite database
func Init(dbPath string) (*sql.DB, error) {
	// Create directory if it doesn't exist
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open database connection
	db, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=10000&_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

// Migrate runs all database migrations
func Migrate(db *sql.DB) error {
	migrations := []string{
		createTemplatesTable,
		createDeploymentsTable,
		createNewtConfigsTable,
		createDeploymentLogsTable,
		createTemplateRatingsTable,
		createReviewHelpfulVotesTable,
		createBackupsTable,
		createBackupSchedulesTable,
		createIndexes,
		createTriggers,
	}

	for i, migration := range migrations {
		if err := executeMigration(db, migration, i+1); err != nil {
			return fmt.Errorf("migration %d failed: %w", i+1, err)
		}
	}

	return nil
}

func executeMigration(db *sql.DB, migration string, version int) error {
	// Check if migration already applied
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'").Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		// Create schema_migrations table
		_, err := db.Exec(`
			CREATE TABLE schema_migrations (
				version INTEGER PRIMARY KEY,
				applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
			)
		`)
		if err != nil {
			return err
		}
	}

	// Check if this migration was already applied
	err = db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = ?", version).Scan(&count)
	if err != nil {
		return err
	}

	if count > 0 {
		return nil // Migration already applied
	}

	// Execute migration
	_, err = db.Exec(migration)
	if err != nil {
		return err
	}

	// Record migration
	_, err = db.Exec("INSERT INTO schema_migrations (version) VALUES (?)", version)
	return err
}

const createTemplatesTable = `
CREATE TABLE IF NOT EXISTS templates (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    icon TEXT,
    category TEXT,
    tags TEXT, -- JSON array stored as text
    repo_url TEXT NOT NULL,
    branch TEXT DEFAULT 'main',
    path TEXT DEFAULT '/',
    version TEXT,
    variables TEXT, -- JSON array of environment variables
    requires_newt BOOLEAN DEFAULT 1,
    newt_config TEXT, -- JSON configuration for newt
    publisher_id TEXT,
    is_verified BOOLEAN DEFAULT 0,
    download_count INTEGER DEFAULT 0,
    avg_rating REAL DEFAULT 0.0,
    total_ratings INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);`

const createDeploymentsTable = `
CREATE TABLE IF NOT EXISTS deployments (
    id TEXT PRIMARY KEY,
    template_id TEXT NOT NULL,
    stack_name TEXT NOT NULL UNIQUE,
    status TEXT CHECK(status IN ('pending', 'deploying', 'running', 'stopped', 'failed')),
    config TEXT, -- JSON map of configuration
    newt_injected BOOLEAN DEFAULT 0,
    tunnel_url TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (template_id) REFERENCES templates(id)
);`

const createNewtConfigsTable = `
CREATE TABLE IF NOT EXISTS newt_configs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    endpoint TEXT NOT NULL,
    newt_id TEXT NOT NULL,
    newt_secret TEXT NOT NULL,
    is_active BOOLEAN DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);`

const createDeploymentLogsTable = `
CREATE TABLE IF NOT EXISTS deployment_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    deployment_id TEXT NOT NULL,
    log_level TEXT CHECK(log_level IN ('info', 'warning', 'error')),
    message TEXT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (deployment_id) REFERENCES deployments(id)
);`

const createTemplateRatingsTable = `
CREATE TABLE IF NOT EXISTS template_ratings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    template_id TEXT NOT NULL,
    user_id TEXT NOT NULL,
    rating INTEGER CHECK(rating >= 1 AND rating <= 5),
    review TEXT,
    helpful_count INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(template_id, user_id),
    FOREIGN KEY (template_id) REFERENCES templates(id)
);`

const createReviewHelpfulVotesTable = `
CREATE TABLE IF NOT EXISTS review_helpful_votes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    template_id TEXT NOT NULL,
    review_user_id TEXT NOT NULL,
    voter_user_id TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(template_id, review_user_id, voter_user_id)
);`

const createBackupsTable = `
CREATE TABLE IF NOT EXISTS backups (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT CHECK(type IN ('manual', 'scheduled', 'auto')),
    status TEXT CHECK(status IN ('creating', 'completed', 'failed')),
    size_bytes INTEGER DEFAULT 0,
    include_volumes BOOLEAN DEFAULT 0,
    encrypted BOOLEAN DEFAULT 0,
    storage_path TEXT,
    deployment_ids TEXT, -- JSON array of deployment IDs
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME
);`

const createBackupSchedulesTable = `
CREATE TABLE IF NOT EXISTS backup_schedules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    cron_expression TEXT NOT NULL,
    include_volumes BOOLEAN DEFAULT 0,
    encrypt BOOLEAN DEFAULT 1,
    enabled BOOLEAN DEFAULT 1,
    last_run DATETIME,
    next_run DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);`

const createIndexes = `
CREATE INDEX IF NOT EXISTS idx_templates_category ON templates(category);
CREATE INDEX IF NOT EXISTS idx_templates_verified ON templates(is_verified);
CREATE INDEX IF NOT EXISTS idx_templates_rating ON templates(avg_rating DESC);
CREATE INDEX IF NOT EXISTS idx_deployments_status ON deployments(status);
CREATE INDEX IF NOT EXISTS idx_deployments_template ON deployments(template_id);
CREATE INDEX IF NOT EXISTS idx_deployment_logs_deployment_id ON deployment_logs(deployment_id);
CREATE INDEX IF NOT EXISTS idx_template_ratings_template ON template_ratings(template_id);
CREATE INDEX IF NOT EXISTS idx_template_ratings_rating ON template_ratings(rating DESC);
CREATE INDEX IF NOT EXISTS idx_backups_type ON backups(type);
CREATE INDEX IF NOT EXISTS idx_backups_status ON backups(status);
`

const createTriggers = `
-- Trigger to update template rating statistics when a rating is added/updated
CREATE TRIGGER IF NOT EXISTS update_template_rating_stats
AFTER INSERT ON template_ratings
BEGIN
    UPDATE templates SET
        avg_rating = (
            SELECT AVG(CAST(rating AS REAL))
            FROM template_ratings
            WHERE template_id = NEW.template_id
        ),
        total_ratings = (
            SELECT COUNT(*)
            FROM template_ratings
            WHERE template_id = NEW.template_id
        ),
        updated_at = CURRENT_TIMESTAMP
    WHERE id = NEW.template_id;
END;

-- Trigger to update template rating statistics when a rating is updated
CREATE TRIGGER IF NOT EXISTS update_template_rating_stats_on_update
AFTER UPDATE ON template_ratings
BEGIN
    UPDATE templates SET
        avg_rating = (
            SELECT AVG(CAST(rating AS REAL))
            FROM template_ratings
            WHERE template_id = NEW.template_id
        ),
        total_ratings = (
            SELECT COUNT(*)
            FROM template_ratings
            WHERE template_id = NEW.template_id
        ),
        updated_at = CURRENT_TIMESTAMP
    WHERE id = NEW.template_id;
END;

-- Trigger to update template rating statistics when a rating is deleted
CREATE TRIGGER IF NOT EXISTS update_template_rating_stats_on_delete
AFTER DELETE ON template_ratings
BEGIN
    UPDATE templates SET
        avg_rating = COALESCE((
            SELECT AVG(CAST(rating AS REAL))
            FROM template_ratings
            WHERE template_id = OLD.template_id
        ), 0.0),
        total_ratings = (
            SELECT COUNT(*)
            FROM template_ratings
            WHERE template_id = OLD.template_id
        ),
        updated_at = CURRENT_TIMESTAMP
    WHERE id = OLD.template_id;
END;

-- Trigger to increment download count when deployment is created
CREATE TRIGGER IF NOT EXISTS increment_download_count
AFTER INSERT ON deployments
BEGIN
    UPDATE templates SET
        download_count = download_count + 1,
        updated_at = CURRENT_TIMESTAMP
    WHERE id = NEW.template_id;
END;
`