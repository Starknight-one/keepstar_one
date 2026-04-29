package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Client struct {
	pool *pgxpool.Pool
}

func NewClient(ctx context.Context, databaseURL string) (*Client, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}

	// Tuned for Neon serverless autosuspend (default 5 min). MinConns=0 + short
	// idle timeout lets the pool collapse to zero on quiet periods so Neon can
	// suspend the compute. HealthCheckPeriod is intentionally long so the pool
	// doesn't ping idle connections back into the warm path.
	config.MaxConns = 10
	config.MinConns = 0
	config.MaxConnLifetime = time.Hour
	config.MaxConnIdleTime = time.Minute
	config.HealthCheckPeriod = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return &Client{pool: pool}, nil
}

func (c *Client) Pool() *pgxpool.Pool { return c.pool }
func (c *Client) Ping(ctx context.Context) error { return c.pool.Ping(ctx) }
func (c *Client) Close() { c.pool.Close() }
