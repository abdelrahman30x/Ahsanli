package schema

import (
	"context"
	"os"
	"testing"
	"time"

	"ahsanly/go_platform/internal/catalog"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestIsolatedSchemaMigration(t *testing.T) {
	if os.Getenv("DEAL_GO_INTEGRATION") != "1" {
		t.Skip("requires isolated botafflite_go_test PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(catalog.DatabaseURL())
	if err != nil {
		t.Fatal("invalid database configuration")
	}
	cfg.ConnConfig.Database = "botafflite_go_test"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("isolated PostgreSQL pool unavailable")
	}
	defer pool.Close()
	var database string
	if err = pool.QueryRow(ctx, `SELECT current_database()`).Scan(&database); err != nil || database != "botafflite_go_test" {
		t.Fatal("refusing migration test outside isolated database")
	}
	pending, err := Apply(ctx, pool, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) > 0 {
		if _, err = Apply(ctx, pool, true); err != nil {
			t.Fatal(err)
		}
	}
	pending, err = Apply(ctx, pool, false)
	if err != nil || len(pending) != 0 {
		t.Fatalf("migration is not idempotent: %v, %v", pending, err)
	}
	var exists bool
	if err = pool.QueryRow(ctx, `SELECT to_regclass('public.go_admin_session') IS NOT NULL`).Scan(&exists); err != nil || !exists {
		t.Fatalf("session table missing: %v", err)
	}
}
