package main

import (
	"net/url"
	"testing"
)

func TestDatabaseURL_PrefersDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:p@h:5432/db?sslmode=disable")
	if got := databaseURL(); got != "postgres://u:p@h:5432/db?sslmode=disable" {
		t.Fatalf("expected DATABASE_URL to be used verbatim, got %q", got)
	}
}

// A Secrets Manager-issued password can contain URL-reserved characters; the assembled
// DSN must escape them so it parses back to exactly those credentials.
func TestDatabaseURL_EscapesReservedCharsInCredentials(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("PGUSER", "ledger_app")
	t.Setenv("PGPASSWORD", "p@ss/w:rd?#x")
	t.Setenv("PGHOST", "db.internal")
	t.Setenv("PGPORT", "5432")
	t.Setenv("PGDATABASE", "ledger")

	u, err := url.Parse(databaseURL())
	if err != nil {
		t.Fatalf("assembled DSN did not parse: %v", err)
	}
	if u.Hostname() != "db.internal" || u.Port() != "5432" {
		t.Fatalf("wrong host/port: %q", u.Host)
	}
	if u.Path != "/ledger" {
		t.Fatalf("wrong database path: %q", u.Path)
	}
	pass, _ := u.User.Password()
	if u.User.Username() != "ledger_app" || pass != "p@ss/w:rd?#x" {
		t.Fatalf("credentials did not round-trip: user=%q pass=%q", u.User.Username(), pass)
	}
	if u.Query().Get("sslmode") != "require" {
		t.Fatalf("expected sslmode=require, got %q", u.Query().Get("sslmode"))
	}
}
