package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Client struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, databaseURL string) (*Client, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL pool: %w", err)
	}
	client := &Client{pool: pool}
	if err := client.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	return client, nil
}

func (c *Client) Ping(ctx context.Context) error {
	return c.pool.Ping(ctx)
}

func (c *Client) Close() {
	c.pool.Close()
}

func (c *Client) Pool() *pgxpool.Pool {
	return c.pool
}
