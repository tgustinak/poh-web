package config

import (
	"testing"
)

func TestValidate(t *testing.T) {
	valid := Config{Port: ":8080", DatabasePath: "./data/app.db", UploadDir: "./data/uploads", LogLevel: "info", ForceShutdownAfterInSeconds: "10"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("expected valid config, got %v", err)
	}

	for name, mutate := range map[string]func(*Config){
		"empty port":       func(c *Config) { c.Port = "" },
		"bad port":         func(c *Config) { c.Port = ":abc" },
		"port zero":        func(c *Config) { c.Port = ":0" },
		"port too big":     func(c *Config) { c.Port = ":99999" },
		"empty db path":    func(c *Config) { c.DatabasePath = "" },
		"empty upload dir": func(c *Config) { c.UploadDir = "" },
		"bad log level":    func(c *Config) { c.LogLevel = "verbose" },
		"bad shutdown":     func(c *Config) { c.ForceShutdownAfterInSeconds = "soon" },
	} {
		cfg := valid
		mutate(&cfg)
		if err := cfg.Validate(); err == nil {
			t.Fatalf("%s: expected validation error", name)
		}
	}
}

func TestNeedsAdminSeed(t *testing.T) {
	full := Config{AdminEmail: "a@b.c", AdminPassword: "secret"}
	if !full.NeedsAdminSeed() {
		t.Fatal("expected seed needed with both credentials")
	}

	partial := Config{AdminEmail: "a@b.c"}
	if partial.NeedsAdminSeed() {
		t.Fatal("expected no seed without password")
	}
}
