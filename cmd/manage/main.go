// Command manage gồm các tác vụ quản trị chạy ngoài server:
//
//	go run ./cmd/manage migrate                       # chạy migration (dùng khi DB_AUTO_MIGRATE=false)
//	go run ./cmd/manage set-role -email a@x.com admin # cấp quyền admin (hoặc "user" để thu hồi)
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"demo_echo/internal/config"
	"demo_echo/internal/database"
	"demo_echo/internal/models"

	"gorm.io/gorm"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("invalid config: %v", err)
	}
	db, err := database.Connect(cfg)
	if err != nil {
		log.Fatalf("failed to connect database: %v", err)
	}

	switch os.Args[1] {
	case "migrate":
		if err := database.Migrate(db); err != nil {
			log.Fatalf("migrate: %v", err)
		}
		log.Println("migrate: done")
	case "set-role":
		fs := flag.NewFlagSet("set-role", flag.ExitOnError)
		email := fs.String("email", "", "email của user")
		_ = fs.Parse(os.Args[2:])
		if *email == "" || fs.NArg() != 1 {
			usage()
		}
		if err := setRole(db, *email, fs.Arg(0)); err != nil {
			log.Fatalf("set-role: %v", err)
		}
	default:
		usage()
	}
}

func setRole(db *gorm.DB, email, role string) error {
	if role != models.RoleAdmin && role != models.RoleUser {
		return fmt.Errorf("role must be %q or %q", models.RoleAdmin, models.RoleUser)
	}
	email = strings.ToLower(strings.TrimSpace(email))
	res := db.Model(&models.User{}).Where("email = ?", email).Update("role", role)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		var count int64
		db.Model(&models.User{}).Where("email = ?", email).Count(&count)
		if count == 0 {
			return errors.New("no user with email " + email)
		}
	}
	log.Printf("set-role: %s is now %s", email, role)
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, `usage:
  manage migrate
  manage set-role -email <email> admin|user`)
	os.Exit(2)
}
