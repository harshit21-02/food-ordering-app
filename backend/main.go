package main

import (
	"log"

	"github.com/harshit/food-ordering-app/internal/config"
	"github.com/harshit/food-ordering-app/internal/db"
	"github.com/harshit/food-ordering-app/internal/mailer"
	"github.com/harshit/food-ordering-app/internal/server"
)

func main() {
	cfg := config.Load()

	gormDB, err := db.Open(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}

	mail := mailer.New(mailer.Config{
		Host: cfg.SMTPHost,
		Port: cfg.SMTPPort,
		User: cfg.SMTPUser,
		Pass: cfg.SMTPPass,
		From: cfg.SMTPFrom,
	})
	if mail.Enabled() {
		log.Printf("mailer: SMTP enabled (host=%s)", cfg.SMTPHost)
	} else {
		log.Printf("mailer: SMTP not configured — OTP responses will include dev_otp")
	}

	r, err := server.New(cfg, gormDB, mail)
	if err != nil {
		log.Fatalf("server build: %v", err)
	}

	log.Printf("listening on %s", cfg.BackendAddr)
	if err := r.Run(cfg.BackendAddr); err != nil {
		log.Fatal(err)
	}
}
