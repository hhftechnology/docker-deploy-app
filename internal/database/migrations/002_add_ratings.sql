
-- Add rating columns to templates table
ALTER TABLE templates ADD COLUMN avg_rating REAL DEFAULT 0.0;
ALTER TABLE templates ADD COLUMN total_ratings INTEGER DEFAULT 0;

-- Template ratings table
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
    FOREIGN KEY (template_id) REFERENCES templates(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Review helpful votes table
CREATE TABLE IF NOT EXISTS review_helpful_votes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    template_id TEXT NOT NULL,
    review_user_id TEXT NOT NULL,
    voter_user_id TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(template_id, review_user_id, voter_user_id),
    FOREIGN KEY (template_id) REFERENCES templates(id) ON DELETE CASCADE,
    FOREIGN KEY (review_user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (voter_user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Indexes for ratings
CREATE INDEX IF NOT EXISTS idx_template_ratings_template ON template_ratings(template_id);
CREATE INDEX IF NOT EXISTS idx_template_ratings_rating ON template_ratings(rating DESC);
CREATE INDEX IF NOT EXISTS idx_template_ratings_user ON template_ratings(user_id);
CREATE INDEX IF NOT EXISTS idx_templates_rating ON templates(avg_rating DESC);
CREATE INDEX IF NOT EXISTS idx_review_votes_template ON review_helpful_votes(template_id);

-- Triggers to automatically update template rating statistics
CREATE TRIGGER IF NOT EXISTS update_template_rating_stats_insert
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

CREATE TRIGGER IF NOT EXISTS update_template_rating_stats_update
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

CREATE TRIGGER IF NOT EXISTS update_template_rating_stats_delete
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