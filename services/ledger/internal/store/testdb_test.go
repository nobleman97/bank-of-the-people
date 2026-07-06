package store_test

import (
	"context"
	"database/sql"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/nobleman97/bank-of-the-people/services/ledger/internal/store"
)

// newTestDB starts a real Postgres container, runs the embedded migrations against it,
// and returns an open *sql.DB. The container is terminated when the test completes.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()

	// BasicWaitStrategies waits for the "ready to accept connections" log line
	// (twice — Postgres restarts once after initdb) plus a listening-port check, so
	// the container is genuinely accepting connections before we migrate. Without it,
	// postgres.Run only waits for the port to listen, which races the pre-restart startup.
	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("ledger"),
		postgres.WithUsername("ledger"),
		postgres.WithPassword("ledger"),
		postgres.BasicWaitStrategies(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	require.NoError(t, store.RunMigrations(connStr))

	db, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	return db
}
