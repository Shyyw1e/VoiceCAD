package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	App      AppConfig
	HTTP     HTTPConfig
	Log      LogConfig
	Postgres PostgresConfig
	Storage  StorageConfig
	ML       MLConfig
	CAD      CADConfig
	Telegram TelegramConfig
}

type AppConfig struct {
	Name string
	Env  string
}

type HTTPConfig struct {
	Addr            string
	ShutdownTimeout time.Duration
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
}

type LogConfig struct {
	Level  string
	Format string
}

type PostgresConfig struct {
	Host            string
	Port            int
	User            string
	Password        string
	Database        string
	SSLMode         string
	MaxOpenConns    int32
	MinIdleConns    int32
	ConnMaxLifetime time.Duration
}

type StorageConfig struct {
	LocalDir string
}

type MLConfig struct {
	TranscriberURL string
	ParserURL      string
}

type CADConfig struct {
	ExecutorURL string
}

type TelegramConfig struct {
	BotToken string
}

func Load() (Config, error) {
	loadDotEnv(".env")

	cfg := Config{
		App: AppConfig{
			Name: getEnv("APP_NAME", "voicecad-backend"),
			Env:  getEnv("APP_ENV", "development"),
		},
		HTTP: HTTPConfig{
			Addr:            getEnv("HTTP_ADDR", ":8080"),
			ShutdownTimeout: getEnvDuration("HTTP_SHUTDOWN_TIMEOUT", 10*time.Second),
			ReadTimeout:     getEnvDuration("HTTP_READ_TIMEOUT", 10*time.Second),
			WriteTimeout:    getEnvDuration("HTTP_WRITE_TIMEOUT", 15*time.Second),
			IdleTimeout:     getEnvDuration("HTTP_IDLE_TIMEOUT", 60*time.Second),
		},
		Log: LogConfig{
			Level:  getEnv("LOG_LEVEL", "info"),
			Format: getEnv("LOG_FORMAT", "text"),
		},
		Postgres: PostgresConfig{
			Host:            getEnv("POSTGRES_HOST", "localhost"),
			Port:            getEnvInt("POSTGRES_PORT", 5432),
			User:            getEnv("POSTGRES_USER", "postgres"),
			Password:        getEnv("POSTGRES_PASSWORD", "postgres"),
			Database:        getEnv("POSTGRES_DB", "voicecad"),
			SSLMode:         getEnv("POSTGRES_SSLMODE", "disable"),
			MaxOpenConns:    int32(getEnvInt("POSTGRES_MAX_OPEN_CONNS", 10)),
			MinIdleConns:    int32(getEnvInt("POSTGRES_MIN_IDLE_CONNS", 2)),
			ConnMaxLifetime: getEnvDuration("POSTGRES_CONN_MAX_LIFETIME", 30*time.Minute),
		},
		Storage: StorageConfig{
			LocalDir: getEnv("STORAGE_LOCAL_DIR", "data/storage"),
		},
		ML: MLConfig{
			TranscriberURL: getEnv("ML_TRANSCRIBER_URL", ""),
			ParserURL:      getEnv("ML_PARSER_URL", ""),
		},
		CAD: CADConfig{
			ExecutorURL: getEnv("CAD_EXECUTOR_URL", ""),
		},
		Telegram: TelegramConfig{
			BotToken: getEnv("TELEGRAM_BOT_TOKEN", ""),
		},
	}

	if cfg.Postgres.Port <= 0 {
		return Config{}, fmt.Errorf("postgres port must be positive")
	}

	return cfg, nil
}

func (c PostgresConfig) DSN() string {
	return c.DSNForDatabase(c.Database)
}

func (c PostgresConfig) DSNForDatabase(database string) string {
	dsn := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(c.User, c.Password),
		Host:   fmt.Sprintf("%s:%d", c.Host, c.Port),
		Path:   database,
	}

	query := dsn.Query()
	query.Set("sslmode", c.SSLMode)
	dsn.RawQuery = query.Encode()
	return dsn.String()
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}

	return fallback
}

func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" {
			continue
		}

		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
}

func getEnvInt(key string, fallback int) int {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return parsed
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}

	return parsed
}
