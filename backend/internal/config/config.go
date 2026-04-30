package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL   string
	BackendAddr   string
	JWTSecret     string
	OTPTTLSeconds int
	JWTTTLDays    int

	// Env: "production" disables dev_otp leakage and tightens defaults.
	// Anything else (or unset) is treated as development.
	Env string

	// CORSOrigins is the comma-separated allow-list of frontend origins.
	CORSOrigins []string

	// Cashfree (optional). If ClientID is empty, payment sessions are disabled.
	CashfreeClientID  string
	CashfreeSecretKey string
	CashfreeEnv       string // "sandbox" | "production"

	// SMTP (optional). If SMTPHost is empty, OTP emails aren't sent.
	// In dev, OTP is returned in the API response (`dev_otp`); in production
	// it is never returned regardless of mailer state.
	SMTPHost string
	SMTPPort int
	SMTPUser string
	SMTPPass string
	SMTPFrom string
}

// IsProduction reports whether ENV=production.
func (c *Config) IsProduction() bool {
	return strings.EqualFold(c.Env, "production")
}

func Load() *Config {
	if err := godotenv.Load("../.env"); err != nil {
		log.Println("no .env at ../.env, relying on environment")
	}

	// Render and most PaaS providers set PORT. If present, use it. Otherwise
	// fall back to BACKEND_ADDR (full ":8081"-style string), then default.
	addr := os.Getenv("BACKEND_ADDR")
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port
	}
	if addr == "" {
		addr = ":8081"
	}

	corsRaw := envDefault("CORS_ORIGINS", "http://localhost:5173")
	corsOrigins := splitAndTrim(corsRaw)

	return &Config{
		DatabaseURL:   mustEnv("DATABASE_URL"),
		BackendAddr:   addr,
		JWTSecret:     mustEnv("JWT_SECRET"),
		OTPTTLSeconds: envInt("OTP_TTL_SECONDS", 300),
		JWTTTLDays:    envInt("JWT_TTL_DAYS", 30),
		Env:           envDefault("ENV", "development"),
		CORSOrigins:   corsOrigins,
		CashfreeClientID:  os.Getenv("CASHFREE_CLIENT_ID"),
		CashfreeSecretKey: os.Getenv("CASHFREE_SECRET_KEY"),
		CashfreeEnv:       envDefault("CASHFREE_ENV", "sandbox"),
		SMTPHost:      os.Getenv("SMTP_HOST"),
		SMTPPort:      envInt("SMTP_PORT", 587),
		SMTPUser:      os.Getenv("SMTP_USER"),
		SMTPPass:      os.Getenv("SMTP_PASS"),
		SMTPFrom:      os.Getenv("SMTP_FROM"),
	}
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("env %s is required", k)
	}
	return v
}

func envDefault(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
