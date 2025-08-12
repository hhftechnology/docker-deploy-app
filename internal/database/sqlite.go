package database

import (
	"database/sql"
	"embed"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"docker-deploy-app/internal/config"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// DB wraps the sql.DB connection with additional functionality
type DB struct {
	*sql.DB
	config *config.Config
}

// NewConnection creates a new SQLite database connection
func NewConnection(cfg *config.Config) (*DB, error) {
	// Ensure data directory exists
	dbDir := filepath.Dir(cfg.Database.Path)
	if err := ensureDir(dbDir); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	// Open SQLite database
	sqlDB, err := sql.Open("sqlite3", cfg.Database.Path+"?_foreign_keys=on&_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(25)

	db := &DB{
		DB:     sqlDB,
		config: cfg,
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

// RunMigrations executes all pending database migrations
func (db *DB) RunMigrations() error {
	// Create migrations table if it doesn't exist
	if err := db.createMigrationsTable(); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get applied migrations
	appliedMigrations, err := db.getAppliedMigrations()
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Get available migrations
	availableMigrations, err := db.getAvailableMigrations()
	if err != nil {
		return fmt.Errorf("failed to get available migrations: %w", err)
	}

	// Execute pending migrations
	for _, migration := range availableMigrations {
		if !contains(appliedMigrations, migration.Name) {
			if err := db.executeMigration(migration); err != nil {
				return fmt.Errorf("failed to execute migration %s: %w", migration.Name, err)
			}
		}
	}

	return nil
}

// Migration represents a database migration
type Migration struct {
	Name     string
	Version  int
	SQL      string
	Filename string
}

// createMigrationsTable creates the schema_migrations table
func (db *DB) createMigrationsTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
	`
	_, err := db.Exec(query)
	return err
}

// getAppliedMigrations returns list of applied migration names
func (db *DB) getAppliedMigrations() ([]string, error) {
	rows, err := db.Query("SELECT name FROM schema_migrations ORDER BY version")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var migrations []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		migrations = append(migrations, name)
	}

	return migrations, rows.Err()
}

// getAvailableMigrations returns list of available migrations from embedded files
func (db *DB) getAvailableMigrations() ([]Migration, error) {
	entries, err := migrationFiles.ReadDir("migrations")
	if err != nil {
		return nil, err
	}

	var migrations []Migration
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			migration, err := db.parseMigrationFile(entry.Name())
			if err != nil {
				return nil, fmt.Errorf("failed to parse migration %s: %w", entry.Name(), err)
			}
			migrations = append(migrations, migration)
		}
	}

	// Sort migrations by version
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// parseMigrationFile parses a migration file and extracts metadata
func (db *DB) parseMigrationFile(filename string) (Migration, error) {
	// Read file content
	content, err := migrationFiles.ReadFile("migrations/" + filename)
	if err != nil {
		return Migration{}, err
	}

	// Extract version from filename (e.g., "001_initial_tables.sql" -> 1)
	parts := strings.Split(filename, "_")
	if len(parts) < 2 {
		return Migration{}, fmt.Errorf("invalid migration filename format: %s", filename)
	}

	version, err := strconv.Atoi(parts[0])
	if err != nil {
		return Migration{}, fmt.Errorf("invalid version in filename %s: %w", filename, err)
	}

	// Create readable name from filename
	nameParts := strings.Split(strings.TrimSuffix(filename, ".sql"), "_")[1:]
	name := strings.Join(nameParts, "_")

	return Migration{
		Name:     name,
		Version:  version,
		SQL:      string(content),
		Filename: filename,
	}, nil
}

// executeMigration executes a single migration
func (db *DB) executeMigration(migration Migration) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Execute migration SQL
	if _, err := tx.Exec(migration.SQL); err != nil {
		return fmt.Errorf("failed to execute migration SQL: %w", err)
	}

	// Record migration as applied
	_, err = tx.Exec(
		"INSERT INTO schema_migrations (version, name) VALUES (?, ?)",
		migration.Version, migration.Name,
	)
	if err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	return tx.Commit()
}

// GetAppliedMigrations returns the list of applied migrations for status checking
func (db *DB) GetAppliedMigrations() ([]Migration, error) {
	rows, err := db.Query(`
		SELECT version, name 
		FROM schema_migrations 
		ORDER BY version
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var migrations []Migration
	for rows.Next() {
		var migration Migration
		if err := rows.Scan(&migration.Version, &migration.Name); err != nil {
			return nil, err
		}
		migrations = append(migrations, migration)
	}

	return migrations, rows.Err()
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.DB.Close()
}

// Health checks database connectivity
func (db *DB) Health() error {
	return db.Ping()
}

// GetStats returns database statistics
func (db *DB) GetStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// Get table counts
	tables := []string{"templates", "deployments", "users", "sessions"}
	for _, table := range tables {
		var count int
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
		if err := db.QueryRow(query).Scan(&count); err != nil {
			// Table might not exist yet
			count = 0
		}
		stats[table] = count
	}

	// Get database file size (SQLite specific)
	var pageCount, pageSize int
	db.QueryRow("PRAGMA page_count").Scan(&pageCount)
	db.QueryRow("PRAGMA page_size").Scan(&pageSize)
	stats["size_bytes"] = pageCount * pageSize

	return stats, nil
}

// Backup creates a backup of the database
func (db *DB) Backup(backupPath string) error {
	// Ensure backup directory exists
	if err := ensureDir(filepath.Dir(backupPath)); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}

	// For SQLite, we can use the backup API or simple file copy
	// Using VACUUM INTO for a clean backup
	_, err := db.Exec("VACUUM INTO ?", backupPath)
	return err
}

// Helper functions

// ensureDir ensures a directory exists
func ensureDir(dir string) error {
	if dir == "" || dir == "." {
		return nil
	}
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return os.MkdirAll(dir, 0755)
			}
			return err
		}
		return nil
	})
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}