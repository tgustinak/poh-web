// Package config loads application configuration from environment
// variables (and an optional .env file for local development).
package config

import (
	"os"

	"github.com/joho/godotenv"
)

// Config holds runtime configuration loaded from the environment.
type Config struct {
	Port          string
	DatabasePath  string
	AdminEmail    string
	AdminPassword string
}

// Load reads .env (if present) and returns Config with defaults.
func Load() Config {
	_ = godotenv.Load()

	return Config{
		Port:          getenv("PORT", ":8080"),
		DatabasePath:  getenv("DATABASE_PATH", "./data/app.db"),
		AdminEmail:    os.Getenv("ADMIN_EMAIL"),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),
	}
}

func getenv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}

	return fallback
}
