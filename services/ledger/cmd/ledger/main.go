package main

import (
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/nobleman97/bank-of-the-people/services/ledger/internal/httpapi"
	"github.com/nobleman97/bank-of-the-people/services/ledger/internal/store"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(runHealthcheck())
	}

	dbURL := databaseURL()
	port := envOr("PORT", "8080")

	if err := store.RunMigrations(dbURL); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	router := httpapi.NewRouter(store.New(db))

	log.Printf("ledger listening on :%s", port)
	if err := http.ListenAndServe(":"+port, router); err != nil {
		log.Fatalf("serve: %v", err)
	}
}

// databaseURL prefers a full DATABASE_URL (used by local dev and tests) and falls
// back to assembling one from the discrete PG* env vars the ECS task definition
// injects (PGHOST/PGPORT/PGDATABASE as plain env, PGUSER/PGPASSWORD from the
// dedicated ledger_app Secrets Manager secret — ADR-0014, never the RDS master user).
func databaseURL() string {
	if u := os.Getenv("DATABASE_URL"); u != "" {
		return u
	}
	// Build the DSN with net/url so a PGUSER/PGPASSWORD containing URL-reserved
	// characters (the Secrets Manager-issued credential can be any generated string) is
	// percent-escaped rather than producing a malformed connection string.
	dsn := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(os.Getenv("PGUSER"), os.Getenv("PGPASSWORD")),
		Host:     net.JoinHostPort(os.Getenv("PGHOST"), os.Getenv("PGPORT")),
		Path:     "/" + os.Getenv("PGDATABASE"),
		RawQuery: "sslmode=require",
	}
	return dsn.String()
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// runHealthcheck backs the ECS container HEALTHCHECK. The distroless runtime image
// has no shell/curl, so the binary itself makes the request.
func runHealthcheck() int {
	port := envOr("PORT", "8080")
	resp, err := http.Get(fmt.Sprintf("http://localhost:%s/healthz", port))
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}
