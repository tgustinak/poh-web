// Command seed creates the initial admin user.
// It requires ADMIN_EMAIL and ADMIN_PASSWORD environment variables
// (see .env.example). Existing databases are left untouched.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/tgustinak/pohkapela.sk/internal/auth"
	"github.com/tgustinak/pohkapela.sk/internal/config"
	"github.com/tgustinak/pohkapela.sk/internal/database"
	"github.com/tgustinak/pohkapela.sk/internal/db"
)

func main() {
	cfg := config.Load()

	if cfg.AdminEmail == "" || cfg.AdminPassword == "" {
		log.Fatal("ADMIN_EMAIL and ADMIN_PASSWORD must be set")
	}

	ctx := context.Background()

	sqlDB, err := database.Open(ctx, cfg.DatabasePath)
	if err != nil {
		log.Fatal(err)
	}
	defer sqlDB.Close()

	authSvc := auth.NewService(db.New(sqlDB))

	created, err := authSvc.EnsureAdminUser(ctx, cfg.AdminEmail, cfg.AdminPassword)
	if err != nil {
		log.Fatal(err)
	}

	if created {
		fmt.Printf("Created admin user %s\n", cfg.AdminEmail)

		return
	}

	fmt.Printf("Admin user already exists, nothing to do (wanted %s)\n", cfg.AdminEmail)
	os.Exit(0)
}
