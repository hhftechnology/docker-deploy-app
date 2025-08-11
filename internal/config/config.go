package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds the application configuration
type Config struct {
	Server     ServerConfig     `yaml:"server"`
	Docker     DockerConfig     `yaml:"docker"`
	Newt       NewtConfig       `yaml:"newt"`
	Marketplace MarketplaceConfig `yaml:"marketplace"`
	Backup     BackupConfig     `yaml:"backup"`
	GitHub     GitHubConfig     `yaml:"github"`
	Database   DatabaseConfig   `yaml:"database"`
	Templates  TemplatesConfig  `yaml:"templates"`
	Logging    LoggingConfig    `yaml:"logging"`
	Security   SecurityConfig   `yaml:"security"`
}

type ServerConfig struct {
	Port int        `yaml:"port"`
	Host string     `yaml:"host"`
	CORS CORSConfig `yaml:"cors"`
}

type CORSConfig struct {
	Enabled bool     `yaml:"enabled"`
	Origins []string `yaml:"origins"`
}

type DockerConfig struct {
	Socket         string `yaml:"socket"`
	ComposeTimeout int    `yaml:"compose_timeout"`
	DefaultNetwork string `yaml:"default_network"`
}

type NewtConfig struct {
	Enabled      bool              `yaml:"enabled"`
	AutoInject   bool              `yaml:"auto_inject"`
	DefaultImage string            `yaml:"default_image"`
	Validation   ValidationConfig  `yaml:"validation"`
	DefaultConfig DefaultNewtConfig `yaml:"default_config"`
}

type ValidationConfig struct {
	Enforce           bool `yaml:"enforce"`
	RequireHealthCheck bool `yaml:"require_health_check"`
}

type DefaultNewtConfig struct {
	LogLevel     string `yaml:"log_level"`
	HealthFile   string `yaml:"health_file"`
	DockerSocket string `yaml:"docker_socket"`
}

type MarketplaceConfig struct {
	Enabled               bool     `yaml:"enabled"`
	MinRatingsForDisplay  int      `yaml:"min_ratings_for_display"`
	FeaturedTemplateCount int      `yaml:"featured_template_count"`
	Categories            []string `yaml:"categories"`
	AllowAnonymousRatings bool     `yaml:"allow_anonymous_ratings"`
	ReviewModeration      bool     `yaml:"review_moderation"`
}

type BackupConfig struct {
	Enabled    bool                `yaml:"enabled"`
	Storage    BackupStorageConfig `yaml:"storage"`
	Retention  RetentionConfig     `yaml:"retention"`
	Encryption EncryptionConfig    `yaml:"encryption"`
	Schedules  SchedulesConfig     `yaml:"schedules"`
}

type BackupStorageConfig struct {
	Type string    `yaml:"type"`
	Path string    `yaml:"path"`
	S3   S3Config  `yaml:"s3"`
}

type S3Config struct {
	Bucket    string `yaml:"bucket"`
	Region    string `yaml:"region"`
	AccessKey string `yaml:"access_key"`
	SecretKey string `yaml:"secret_key"`
}

type RetentionConfig struct {
	Daily   int `yaml:"daily"`
	Weekly  int `yaml:"weekly"`
	Monthly int `yaml:"monthly"`
}

type EncryptionConfig struct {
	Enabled    bool   `yaml:"enabled"`
	KeyStorage string `yaml:"key_storage"`
}

type SchedulesConfig struct {
	Daily  ScheduleConfig `yaml:"daily"`
	Weekly ScheduleConfig `yaml:"weekly"`
}

type ScheduleConfig struct {
	Enabled        bool   `yaml:"enabled"`
	Time           string `yaml:"time"`
	Day            string `yaml:"day,omitempty"`
	IncludeVolumes bool   `yaml:"include_volumes"`
}

type GitHubConfig struct {
	Token         string `yaml:"token"`
	WebhookSecret string `yaml:"webhook_secret"`
	SyncInterval  int    `yaml:"sync_interval"`
}

type DatabaseConfig struct {
	Type           string `yaml:"type"`
	Path           string `yaml:"path"`
	BackupEnabled  bool   `yaml:"backup_enabled"`
	BackupInterval int    `yaml:"backup_interval"`
}

type TemplatesConfig struct {
	RepoURL               string   `yaml:"repo_url"`
	Branch                string   `yaml:"branch"`
	CacheDuration         int      `yaml:"cache_duration"`
	AutoVerifyPublishers  []string `yaml:"auto_verify_publishers"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
	Output string `yaml:"output"`
}

type SecurityConfig struct {
	AuthEnabled    bool           `yaml:"auth_enabled"`
	APIKey         string         `yaml:"api_key"`
	SessionTimeout int            `yaml:"session_timeout"`
	EncryptSecrets bool           `yaml:"encrypt_secrets"`
	RateLimiting   RateLimitConfig `yaml:"rate_limiting"`
}

type RateLimitConfig struct {
	Enabled            bool `yaml:"enabled"`
	RequestsPerMinute  int  `yaml:"requests_per_minute"`
}

// Load loads configuration from environment variables with defaults
func Load() (*Config, error) {
	config := &Config{
		Server: ServerConfig{
			Port: getEnvInt("SERVER_PORT", 8080),
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			CORS: CORSConfig{
				Enabled: getEnvBool("CORS_ENABLED", true),
				Origins: getEnvSlice("CORS_ORIGINS", []string{"*"}),
			},
		},
		Docker: DockerConfig{
			Socket:         getEnv("DOCKER_SOCKET", "/var/run/docker.sock"),
			ComposeTimeout: getEnvInt("DOCKER_COMPOSE_TIMEOUT", 300),
			DefaultNetwork: getEnv("DOCKER_DEFAULT_NETWORK", "app_network"),
		},
		Newt: NewtConfig{
			Enabled:      getEnvBool("NEWT_ENABLED", true),
			AutoInject:   getEnvBool("NEWT_AUTO_INJECT", true),
			DefaultImage: getEnv("NEWT_DEFAULT_IMAGE", "fosrl/newt:latest"),
			Validation: ValidationConfig{
				Enforce:           getEnvBool("NEWT_VALIDATION_ENFORCE", true),
				RequireHealthCheck: getEnvBool("NEWT_REQUIRE_HEALTH_CHECK", true),
			},
			DefaultConfig: DefaultNewtConfig{
				LogLevel:     getEnv("NEWT_LOG_LEVEL", "INFO"),
				HealthFile:   getEnv("NEWT_HEALTH_FILE", "/tmp/healthy"),
				DockerSocket: getEnv("NEWT_DOCKER_SOCKET", "/var/run/docker.sock"),
			},
		},
		Marketplace: MarketplaceConfig{
			Enabled:               getEnvBool("MARKETPLACE_ENABLED", true),
			MinRatingsForDisplay:  getEnvInt("MARKETPLACE_MIN_RATINGS", 5),
			FeaturedTemplateCount: getEnvInt("MARKETPLACE_FEATURED_COUNT", 10),
			Categories: getEnvSlice("MARKETPLACE_CATEGORIES", []string{
				"web", "database", "monitoring", "networking", "development", "ai-ml", "security", "analytics",
			}),
			AllowAnonymousRatings: getEnvBool("MARKETPLACE_ALLOW_ANONYMOUS_RATINGS", false),
			ReviewModeration:      getEnvBool("MARKETPLACE_REVIEW_MODERATION", true),
		},
		Backup: BackupConfig{
			Enabled: getEnvBool("BACKUP_ENABLED", true),
			Storage: BackupStorageConfig{
				Type: getEnv("BACKUP_STORAGE_TYPE", "local"),
				Path: getEnv("BACKUP_STORAGE_PATH", "./backups"),
				S3: S3Config{
					Bucket:    getEnv("S3_BACKUP_BUCKET", ""),
					Region:    getEnv("S3_REGION", ""),
					AccessKey: getEnv("S3_ACCESS_KEY", ""),
					SecretKey: getEnv("S3_SECRET_KEY", ""),
				},
			},
			Retention: RetentionConfig{
				Daily:   getEnvInt("BACKUP_RETENTION_DAILY", 7),
				Weekly:  getEnvInt("BACKUP_RETENTION_WEEKLY", 4),
				Monthly: getEnvInt("BACKUP_RETENTION_MONTHLY", 12),
			},
			Encryption: EncryptionConfig{
				Enabled:    getEnvBool("BACKUP_ENCRYPTION_ENABLED", true),
				KeyStorage: getEnv("BACKUP_KEY_STORAGE", "local"),
			},
			Schedules: SchedulesConfig{
				Daily: ScheduleConfig{
					Enabled:        getEnvBool("BACKUP_DAILY_ENABLED", true),
					Time:           getEnv("BACKUP_DAILY_TIME", "02:00"),
					IncludeVolumes: getEnvBool("BACKUP_DAILY_INCLUDE_VOLUMES", false),
				},
				Weekly: ScheduleConfig{
					Enabled:        getEnvBool("BACKUP_WEEKLY_ENABLED", true),
					Day:            getEnv("BACKUP_WEEKLY_DAY", "sunday"),
					Time:           getEnv("BACKUP_WEEKLY_TIME", "03:00"),
					IncludeVolumes: getEnvBool("BACKUP_WEEKLY_INCLUDE_VOLUMES", true),
				},
			},
		},
		GitHub: GitHubConfig{
			Token:         getEnv("GITHUB_TOKEN", ""),
			WebhookSecret: getEnv("GITHUB_WEBHOOK_SECRET", ""),
			SyncInterval:  getEnvInt("GITHUB_SYNC_INTERVAL", 3600),
		},
		Database: DatabaseConfig{
			Type:           getEnv("DATABASE_TYPE", "sqlite"),
			Path:           getEnv("DATABASE_PATH", "./data/app.db"),
			BackupEnabled:  getEnvBool("DATABASE_BACKUP_ENABLED", true),
			BackupInterval: getEnvInt("DATABASE_BACKUP_INTERVAL", 3600),
		},
		Templates: TemplatesConfig{
			RepoURL:       getEnv("TEMPLATES_REPO_URL", "https://github.com/yourusername/docker-templates"),
			Branch:        getEnv("TEMPLATES_BRANCH", "main"),
			CacheDuration: getEnvInt("TEMPLATES_CACHE_DURATION", 3600),
			AutoVerifyPublishers: getEnvSlice("TEMPLATES_AUTO_VERIFY_PUBLISHERS", []string{
				"docker", "bitnami", "linuxserver",
			}),
		},
		Logging: LoggingConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", "json"),
			Output: getEnv("LOG_OUTPUT", "stdout"),
		},
		Security: SecurityConfig{
			AuthEnabled:    getEnvBool("AUTH_ENABLED", false),
			APIKey:         getEnv("API_KEY", ""),
			SessionTimeout: getEnvInt("SESSION_TIMEOUT", 86400),
			EncryptSecrets: getEnvBool("ENCRYPT_SECRETS", true),
			RateLimiting: RateLimitConfig{
				Enabled:           getEnvBool("RATE_LIMITING_ENABLED", true),
				RequestsPerMinute: getEnvInt("RATE_LIMITING_RPM", 60),
			},
		},
	}

	return config, nil
}

// Helper functions for environment variable parsing
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

func getEnvSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		return strings.Split(value, ",")
	}
	return defaultValue
}