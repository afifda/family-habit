package health

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Checker interface{ Check(context.Context) error }

type DatabaseChecker struct {
	pool    *pgxpool.Pool
	timeout time.Duration
}

func NewDatabaseChecker(pool *pgxpool.Pool, timeout time.Duration) *DatabaseChecker {
	return &DatabaseChecker{pool: pool, timeout: timeout}
}

func (c *DatabaseChecker) Check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	if err := c.pool.Ping(ctx); err != nil {
		return fmt.Errorf("database unavailable: %w", err)
	}
	var exists bool
	if err := c.pool.QueryRow(ctx, `SELECT to_regclass('public.schema_migrations') IS NOT NULL`).Scan(&exists); err != nil {
		return fmt.Errorf("check schema: %w", err)
	}
	if !exists {
		return fmt.Errorf("database schema is not migrated")
	}
	return nil
}
