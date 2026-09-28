package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/tgustinak/pohkapela.sk/internal/admin"
	"github.com/tgustinak/pohkapela.sk/internal/app"
	"github.com/tgustinak/pohkapela.sk/internal/auth"
	"github.com/tgustinak/pohkapela.sk/internal/config"
	"github.com/tgustinak/pohkapela.sk/internal/database"
	"github.com/tgustinak/pohkapela.sk/internal/db"
	"github.com/tgustinak/pohkapela.sk/internal/logbuffer"
	"github.com/tgustinak/pohkapela.sk/internal/server"
)

func main() {
	cfg := config.Load()

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid configuration: %v\n", err)
		os.Exit(1)
	}

	level, _ := config.ParseLogLevel(cfg.LogLevel)
	logs := logbuffer.New(logbuffer.DefaultMaxEntries)
	slog.SetDefault(slog.New(logbuffer.TeeHandler(os.Stderr, &slog.HandlerOptions{Level: level}, logs)))

	ctx := context.Background()

	sqlDB, err := database.Open(ctx, cfg.DatabasePath)
	if err != nil {
		slog.Error("open database", "error", err)
		os.Exit(1)
	}
	defer sqlDB.Close()

	queries := db.New(sqlDB)
	authSvc := auth.NewService(queries)

	if healed, err := authSvc.BackfillSessionCsrf(ctx); err != nil {
		slog.Error("backfill session csrf", "error", err)
		os.Exit(1)
	} else if healed > 0 {
		slog.Info("healed sessions missing csrf token", "count", healed)
	}

	seedAdmin(ctx, authSvc, cfg)

	shutdownTimeout, err := strconv.Atoi(cfg.ForceShutdownAfterInSeconds)
	if err != nil {
		slog.Error("unable to convert force shutdown to number", "error", err)
	}

	srv := server.NewServer(server.NewServerOptions{
		Port:                    cfg.Port,
		GracefulShutdownTimeout: time.Duration(shutdownTimeout),
	})

	srv.Mount("/", app.NewMux(app.Deps{
		Auth:              authSvc,
		Queries:           queries,
		UploadDir:         cfg.UploadDir,
		AuthHandler:       admin.NewAuthHandler(authSvc),
		LogsHandler:       admin.NewLogsHandler(logs),
		ConcertsHandler:   admin.NewConcertsHandler(queries),
		VideosHandler:     admin.NewVideosHandler(queries),
		AboutHandler:      admin.NewAboutHandler(queries),
		SocialHandler:     admin.NewSocialHandler(queries),
		OrganizersHandler: admin.NewOrganizersHandler(queries, cfg.UploadDir),
		ContactHandler:    admin.NewContactHandler(queries, cfg.UploadDir),
	}))

	srv.Start()
}

// seedAdmin creates the initial admin account when ADMIN_EMAIL and
// ADMIN_PASSWORD are set and no users exist yet.
func seedAdmin(ctx context.Context, authSvc *auth.Service, cfg config.Config) {
	if !cfg.NeedsAdminSeed() {
		slog.Warn("ADMIN_EMAIL or ADMIN_PASSWORD not set, skipping admin seed (see .env.example)")

		return
	}

	created, err := authSvc.EnsureAdminUser(ctx, cfg.AdminEmail, cfg.AdminPassword)
	if err != nil {
		slog.Error("seed admin user", "error", err)
		os.Exit(1)
	}

	if created {
		slog.Info("seeded admin user", "email", cfg.AdminEmail)
	}
}
