package postgres_test

import (
	"context"
	"os"
	"testing"
	"time"

	"keepstar/internal/adapters/postgres"
)

// sharedClient is a single connection pool reused by all tests in this package.
// Initialized once in TestMain, avoids ~3-5s TLS handshake per test.
var sharedClient *postgres.Client

func TestMain(m *testing.M) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		// No DB — run anyway, individual tests will t.Skip()
		os.Exit(m.Run())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var err error
	sharedClient, err = postgres.NewClient(ctx, dbURL)
	if err != nil {
		panic("shared_test: connect failed: " + err.Error())
	}

	_ = sharedClient.RunMigrations(ctx)
	_ = sharedClient.RunStateMigrations(ctx)
	_ = sharedClient.RunCatalogMigrations(ctx)

	code := m.Run()
	sharedClient.Close()
	os.Exit(code)
}

// getSharedClient returns the shared client, skipping the test if DB is not available.
func getSharedClient(t *testing.T) *postgres.Client {
	t.Helper()
	if sharedClient == nil {
		t.Skip("DATABASE_URL not set, skipping integration test")
	}
	return sharedClient
}
