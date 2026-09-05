// Package config loads application configuration from environment
// variables (and an optional .env file for local development).
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds runtime configuration loaded from the environment.
type Config struct {
	Port                        string
	DatabasePath                string
	AdminEmail                  string
	AdminPassword               string
	LogLevel                    string
	ForceShutdownAfterInSeconds string
}

// Load reads .env (if present) and returns Config with defaults.
func Load() Config {
	_ = godotenv.Load()

	return Config{
		Port:                        getenv("PORT", ":8080"),
		DatabasePath:                getenv("DATABASE_PATH", "./data/app.db"),
		AdminEmail:                  os.Getenv("ADMIN_EMAIL"),
		AdminPassword:               os.Getenv("ADMIN_PASSWORD"),
		LogLevel:                    getenv("LOG_LEVEL", "info"),
		ForceShutdownAfterInSeconds: getenv("FORCE_SHUTDOWN_AFTER_IN_SECONDS", "10"),
	}
}

// Validate checks that the configuration is usable.
// Missing admin credentials are not an error (seeding is skipped),
// callers should warn via NeedsAdminSeed.
func (c Config) Validate() error {
	if c.Port == "" {
		return fmt.Errorf("PORT must not be empty")
	}

	if port, ok := strings.CutPrefix(c.Port, ":"); ok {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("invalid PORT %q: expected like :8080", c.Port)
		}
	}

	_, err := strconv.Atoi(c.ForceShutdownAfterInSeconds)
	if err != nil {
		return fmt.Errorf("invalid FORCE_SHUTDOWN_AFTER_IN_SECONDS %q: expected like 10", c.ForceShutdownAfterInSeconds)
	}

	if c.DatabasePath == "" {
		return fmt.Errorf("DATABASE_PATH must not be empty")
	}

	if _, err := ParseLogLevel(c.LogLevel); err != nil {
		return err
	}

	return nil
}

// NeedsAdminSeed reports whether initial admin credentials are configured.
// When false, no admin user is seeded and /admin login cannot succeed
// until one is created via `make seed`.
func (c Config) NeedsAdminSeed() bool {
	return c.AdminEmail != "" && c.AdminPassword != ""
}

// ParseLogLevel parses a LOG_LEVEL value (debug, info, warn, error).
func ParseLogLevel(s string) (slog.Level, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(s)); err != nil {
		return slog.LevelInfo, fmt.Errorf("invalid LOG_LEVEL %q: expected debug, info, warn or error", s)
	}

	return level, nil
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}
