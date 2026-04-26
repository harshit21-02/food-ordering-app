// seed — execute a SQL file against DATABASE_URL.
//
// Usage:
//   go run ./cmd/seed [path/to/file.sql]
//
// Default path is ../seeds/dev.sql (relative to backend/). Uses lib/pq's simple
// query protocol so multi-statement files work. Safe for any Postgres
// (local Docker or AWS RDS).
package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"
	"github.com/joho/godotenv"
)

func main() {
	flag.Parse()
	path := "../seeds/dev.sql"
	if flag.NArg() > 0 {
		path = flag.Arg(0)
	}

	_ = godotenv.Load("../.env")
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required (set in .env or environment)")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("read %s: %v", path, err)
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("db ping: %v", err)
	}

	if _, err := db.Exec(string(data)); err != nil {
		log.Fatalf("seed exec: %v", err)
	}
	fmt.Printf("✓ seeded from %s\n", path)
}
