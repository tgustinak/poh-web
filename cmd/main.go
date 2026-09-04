package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/tgustinak/pohkapela.sk/internal/admin"
	"github.com/tgustinak/pohkapela.sk/internal/app"
	"github.com/tgustinak/pohkapela.sk/internal/auth"
	"github.com/tgustinak/pohkapela.sk/internal/config"
	"github.com/tgustinak/pohkapela.sk/internal/database"
	"github.com/tgustinak/pohkapela.sk/internal/db"
	"github.com/tgustinak/pohkapela.sk/internal/server"
)

const FORCE_SHUTDOWN_AFTER_IN_SECONDS = 10

func main() {
	cfg := config.Load()
	ctx := context.Background()

	sqlDB, err := database.Open(ctx, cfg.DatabasePath)
	if err != nil {
		log.Fatal(err)
	}
	defer sqlDB.Close()

	queries := db.New(sqlDB)
	authSvc := auth.NewService(queries)

	seedAdmin(ctx, authSvc, cfg)

	srv := server.NewServer(server.NewServerOptions{
		Port:                    cfg.Port,
		GracefulShutdownTimeout: FORCE_SHUTDOWN_AFTER_IN_SECONDS,
	})

	srv.Mount("/", app.NewMux(app.Deps{
		Auth:         authSvc,
		AdminHandler: admin.NewHandler(authSvc),
	}))

	srv.Start()
}

// seedAdmin creates the initial admin account when ADMIN_EMAIL and
// ADMIN_PASSWORD are set and no users exist yet.
func seedAdmin(ctx context.Context, authSvc *auth.Service, cfg config.Config) {
	if cfg.AdminEmail == "" || cfg.AdminPassword == "" {
		return
	}

	created, err := authSvc.EnsureAdminUser(ctx, cfg.AdminEmail, cfg.AdminPassword)
	if err != nil {
		log.Fatal(err)
	}

	if created {
		fmt.Fprintf(os.Stderr, "Seeded admin user %s\n", cfg.AdminEmail)
	}
}
