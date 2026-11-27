// Package config loads application configuration from environment
// variables (and an optional .env file for local development).
package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds runtime configuration loaded from the environment.
type Config struct {
	Port                        string
	DatabasePath                string
	UploadDir                   string
	AdminEmail                  string
	AdminPassword               string
	LogLevel                    string
	ForceShutdownAfterInSeconds string
	// BehindProxy must be true when the app runs behind a reverse proxy
	// terminating TLS. It enables X-Forwarded-Proto handling for
	// Secure cookies and logging.
	BehindProxy bool
	// SiteURL is the public base URL (scheme + host, no path) used for
	// canonical links, Open Graph tags and the sitemap. Required.
	SiteURL string
	// CookieSecure forces Secure on session/CSRF cookies. Use "always"
	// in production behind HTTPS, "auto" (default) follows the request.
	CookieSecure string
	// GAMeasurementID is the Google Analytics 4 measurement ID (G-XXXXXXXX).
	// Optional: when empty, analytics is disabled entirely. The tag is only
	// loaded in the visitor's browser after they accept analytics cookies.
	GAMeasurementID string
}

// Load reads .env (if present) and returns Config with defaults.
func Load() Config {
	_ = godotenv.Load()

	return Config{
		Port:                        getenv("PORT", ":8080"),
		DatabasePath:                getenv("DATABASE_PATH", "./data/app.db"),
		UploadDir:                   getenv("UPLOAD_DIR", "./data/uploads"),
		AdminEmail:                  os.Getenv("ADMIN_EMAIL"),
		AdminPassword:               os.Getenv("ADMIN_PASSWORD"),
		LogLevel:                    getenv("LOG_LEVEL", "info"),
		ForceShutdownAfterInSeconds: getenv("FORCE_SHUTDOWN_AFTER_IN_SECONDS", "10"),
		BehindProxy:                 getenv("BEHIND_PROXY", "false") == "true",
		CookieSecure:                getenv("COOKIE_SECURE", "auto"),
		SiteURL:                     os.Getenv("SITE_URL"),
		GAMeasurementID:             os.Getenv("GA_MEASUREMENT_ID"),
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

	if c.UploadDir == "" {
		return fmt.Errorf("UPLOAD_DIR must not be empty")
	}

	if _, err := ParseLogLevel(c.LogLevel); err != nil {
		return err
	}

	if c.CookieSecure != "" && c.CookieSecure != "auto" && c.CookieSecure != "always" && c.CookieSecure != "never" {
		return fmt.Errorf("invalid COOKIE_SECURE %q: expected auto, always or never", c.CookieSecure)
	}

	if u, err := url.Parse(c.SiteURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("invalid or missing SITE_URL %q: expected like https://www.example.com", c.SiteURL)
	}

	if c.GAMeasurementID != "" && !gaIDPattern.MatchString(c.GAMeasurementID) {
		return fmt.Errorf("invalid GA_MEASUREMENT_ID %q: expected like G-XXXXXXXXXX", c.GAMeasurementID)
	}

	return nil
}

var gaIDPattern = regexp.MustCompile(`^G-[A-Z0-9]{4,20}$`)

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
