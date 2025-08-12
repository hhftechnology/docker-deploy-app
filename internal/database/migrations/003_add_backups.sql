-- Backups table
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
);

-- Backup metadata table for additional information
CREATE TABLE IF NOT EXISTS backup_metadata (
    backup_id TEXT PRIMARY KEY,
    version TEXT,
    app_version TEXT,
    total_size INTEGER,
    deployment_count INTEGER,
    volume_count INTEGER,
    encryption_key TEXT,
    checksum TEXT,
    extra_data TEXT, -- JSON for additional metadata
    FOREIGN KEY (backup_id) REFERENCES backups(id) ON DELETE CASCADE
);

-- Backup files table to track individual files in backups
CREATE TABLE IF NOT EXISTS backup_files (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    backup_id TEXT NOT NULL,
    file_path TEXT NOT NULL,
    file_type TEXT, -- compose, volume, config, metadata
    original_path TEXT,
    size_bytes INTEGER,
    checksum TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (backup_id) REFERENCES backups(id) ON DELETE CASCADE
);

-- Restore operations table to track restore history
CREATE TABLE IF NOT EXISTS restore_operations (
    id TEXT PRIMARY KEY,
    backup_id TEXT NOT NULL,
    status TEXT CHECK(status IN ('pending', 'running', 'completed', 'failed')),
    selective BOOLEAN DEFAULT 0,
    deployment_ids TEXT, -- JSON array for selective restore
    overwrite_existing BOOLEAN DEFAULT 0,
    restore_volumes BOOLEAN DEFAULT 0,
    test_restore BOOLEAN DEFAULT 0,
    error_message TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME,
    FOREIGN KEY (backup_id) REFERENCES backups(id)
);

-- Restore logs table
CREATE TABLE IF NOT EXISTS restore_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    restore_id TEXT NOT NULL,
    log_level TEXT CHECK(log_level IN ('info', 'warning', 'error')),
    message TEXT,
    timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (restore_id) REFERENCES restore_operations(id) ON DELETE CASCADE
);

-- Indexes for backup tables
CREATE INDEX IF NOT EXISTS idx_backups_type ON backups(type);
CREATE INDEX IF NOT EXISTS idx_backups_status ON backups(status);
CREATE INDEX IF NOT EXISTS idx_backups_created ON backups(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_backup_files_backup ON backup_files(backup_id);
CREATE INDEX IF NOT EXISTS idx_backup_files_type ON backup_files(file_type);
CREATE INDEX IF NOT EXISTS idx_restore_operations_backup ON restore_operations(backup_id);
CREATE INDEX IF NOT EXISTS idx_restore_operations_status ON restore_operations(status);
CREATE INDEX IF NOT EXISTS idx_restore_logs_restore ON restore_logs(restore_id);

-- Trigger to clean up backup files when backup is deleted
CREATE TRIGGER IF NOT EXISTS cleanup_backup_files
AFTER DELETE ON backups
BEGIN
    DELETE FROM backup_files WHERE backup_id = OLD.id;
    DELETE FROM backup_metadata WHERE backup_id = OLD.id;
END;