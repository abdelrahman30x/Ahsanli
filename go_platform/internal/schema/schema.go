package schema

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

const migrationLock int64 = 8274132

// Apply runs versioned Go schema migrations under one PostgreSQL advisory lock.
// With apply=false it reports pending migrations without changing the database.
func Apply(ctx context.Context, db *pgxpool.Pool, apply bool) ([]string, error) {
	entries, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(entries)
	tx, err := db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, migrationLock); err != nil {
		return nil, err
	}
	var exists bool
	err = tx.QueryRow(ctx, `SELECT to_regclass('public.go_schema_migrations') IS NOT NULL`).Scan(&exists)
	if err != nil {
		return nil, err
	}
	applied := map[string]bool{}
	if exists {
		rows, e := tx.Query(ctx, `SELECT name FROM go_schema_migrations`)
		if e != nil {
			return nil, e
		}
		for rows.Next() {
			var name string
			if e = rows.Scan(&name); e != nil {
				rows.Close()
				return nil, e
			}
			applied[name] = true
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
	}
	pending := []string{}
	for _, file := range entries {
		name := strings.TrimPrefix(file, "migrations/")
		if !applied[name] {
			pending = append(pending, name)
		}
	}
	if !apply || len(pending) == 0 {
		return pending, nil
	}
	if !exists {
		if _, err = tx.Exec(ctx, `CREATE TABLE go_schema_migrations(name text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
			return nil, err
		}
	}
	for _, name := range pending {
		contents, e := migrations.ReadFile("migrations/" + name)
		if e != nil {
			return nil, e
		}
		if _, e = tx.Exec(ctx, string(contents)); e != nil {
			return nil, fmt.Errorf("migration %s: %w", name, e)
		}
		if _, e = tx.Exec(ctx, `INSERT INTO go_schema_migrations(name) VALUES($1)`, name); e != nil {
			return nil, e
		}
	}
	return pending, tx.Commit(ctx)
}
