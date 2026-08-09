package database

import (
	"context"
	"os"
	"testing"
)

// TestSchemaConstraints is an opt-in integration test. Point DATABASE_URL at a
// disposable PostgreSQL database; migrations are applied before the assertions.
func TestSchemaConstraints(t *testing.T) {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := MigrateUp(ctx, pool); err != nil {
		t.Fatal(err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var familyID string
	if err := tx.QueryRow(ctx, `INSERT INTO families(name, timezone) VALUES('Test', 'Europe/Berlin') RETURNING id`).Scan(&familyID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO children(family_id, nickname, avatar, color) VALUES($1, 'Sam', 'fox', '#336699')`, familyID); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO children(family_id, nickname, avatar, color) VALUES($1, 'sam', 'bear', '#663399')`, familyID); err == nil {
		t.Fatal("expected active nickname uniqueness violation")
	}
}
