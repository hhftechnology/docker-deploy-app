-- Backup schedules table
CREATE TABLE IF NOT EXISTS backup_schedules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    cron_expression TEXT NOT NULL,
    include_volumes BOOLEAN DEFAULT 0,
    encrypt BOOLEAN DEFAULT 1,
    enabled BOOLEAN DEFAULT 1,
    last_run DATETIME,
    next_run DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Schedule execution logs
CREATE TABLE IF NOT EXISTS schedule_executions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    schedule_id INTEGER NOT NULL,
    backup_id TEXT,
    status TEXT CHECK(status IN ('started', 'completed', 'failed')),
    started_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME,
    error_message TEXT,
    FOREIGN KEY (schedule_id) REFERENCES backup_schedules(id) ON DELETE CASCADE,
    FOREIGN KEY (backup_id) REFERENCES backups(id) ON DELETE SET NULL
);

-- User preferences table
CREATE TABLE IF NOT EXISTS user_preferences (
    user_id TEXT PRIMARY KEY,
    theme TEXT DEFAULT 'auto',
    language TEXT DEFAULT 'en',
    notifications_email BOOLEAN DEFAULT 1,
    notifications_web BOOLEAN DEFAULT 1,
    default_view TEXT DEFAULT 'marketplace',
    backup_notifications BOOLEAN DEFAULT 1,
    deployment_notifications BOOLEAN DEFAULT 1,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- System settings table for global configuration
CREATE TABLE IF NOT EXISTS system_settings (
    key TEXT PRIMARY KEY,
    value TEXT,
    description TEXT,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_by TEXT,
    FOREIGN KEY (updated_by) REFERENCES users(id)
);

-- Notification queue table
CREATE TABLE IF NOT EXISTS notification_queue (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id TEXT,
    type TEXT CHECK(type IN ('email', 'web', 'webhook')),
    subject TEXT,
    message TEXT,
    data TEXT, -- JSON data
    status TEXT CHECK(status IN ('pending', 'sent', 'failed')) DEFAULT 'pending',
    attempts INTEGER DEFAULT 0,
    max_attempts INTEGER DEFAULT 3,
    scheduled_for DATETIME DEFAULT CURRENT_TIMESTAMP,
    sent_at DATETIME,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Indexes for schedules and notifications
CREATE INDEX IF NOT EXISTS idx_backup_schedules_enabled ON backup_schedules(enabled);
CREATE INDEX IF NOT EXISTS idx_backup_schedules_next_run ON backup_schedules(next_run);
CREATE INDEX IF NOT EXISTS idx_schedule_executions_schedule ON schedule_executions(schedule_id);
CREATE INDEX IF NOT EXISTS idx_schedule_executions_status ON schedule_executions(status);
CREATE INDEX IF NOT EXISTS idx_notification_queue_status ON notification_queue(status);
CREATE INDEX IF NOT EXISTS idx_notification_queue_scheduled ON notification_queue(scheduled_for);
CREATE INDEX IF NOT EXISTS idx_notification_queue_user ON notification_queue(user_id);

-- Trigger to update schedule updated_at timestamp
CREATE TRIGGER IF NOT EXISTS update_schedule_timestamp
AFTER UPDATE ON backup_schedules
BEGIN
    UPDATE backup_schedules SET updated_at = CURRENT_TIMESTAMP WHERE id = NEW.id;
END;

-- Insert default system settings
INSERT OR IGNORE INTO system_settings (key, value, description) VALUES
('app_version', '1.0.0', 'Application version'),
('maintenance_mode', 'false', 'Maintenance mode flag'),
('max_concurrent_deployments', '5', 'Maximum concurrent deployments'),
('max_concurrent_backups', '2', 'Maximum concurrent backups'),
('backup_retention_days', '30', 'Default backup retention in days'),
('log_retention_days', '7', 'Log retention in days'),
('websocket_heartbeat_interval', '30', 'WebSocket heartbeat interval in seconds'),
('github_sync_interval', '3600', 'GitHub sync interval in seconds'),
('newt_validation_timeout', '30', 'Newt validation timeout in seconds'),
('rate_limit_per_minute', '60', 'API rate limit per minute per IP');

-- Insert default user preferences for anonymous user
INSERT OR IGNORE INTO user_preferences (user_id, theme, language, default_view) 
VALUES ('anonymous', 'auto', 'en', 'marketplace');