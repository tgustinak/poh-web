// Command seed creates the initial admin user.
// It requires ADMIN_EMAIL and ADMIN_PASSWORD environment variables
// (see .env.example). Existing databases are left untouched.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/tgustinak/pohkapela.sk/internal/auth"
	"github.com/tgustinak/pohkapela.sk/internal/config"
	"github.com/tgustinak/pohkapela.sk/internal/database"
	"github.com/tgustinak/pohkapela.sk/internal/db"
)

func main() {
	cfg := config.Load()

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid configuration: %v\n", err)
		os.Exit(1)
	}

	if !cfg.NeedsAdminSeed() {
		slog.Error("ADMIN_EMAIL and ADMIN_PASSWORD must be set")
		os.Exit(1)
	}

	ctx := context.Background()

	sqlDB, err := database.Open(ctx, cfg.DatabasePath)
	if err != nil {
		slog.Error("open database", "error", err)
		os.Exit(1)
	}
	defer sqlDB.Close()

	authSvc := auth.NewService(db.New(sqlDB))

	created, err := authSvc.EnsureAdminUser(ctx, cfg.AdminEmail, cfg.AdminPassword)
	if err != nil {
		slog.Error("seed admin user", "error", err)
		os.Exit(1)
	}

	if created {
		slog.Info("created admin user", "email", cfg.AdminEmail)

		return
	}

	slog.Info("admin user already exists, nothing to do", "email", cfg.AdminEmail)
}
