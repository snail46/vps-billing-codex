package rediscache

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	client *redis.Client
}

func Open(ctx context.Context, redisURL string) (*Client, error) {
	options, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse Redis URL: %w", err)
	}
	client := &Client{client: redis.NewClient(options)}
	if err := client.Ping(ctx); err != nil {
		return nil, errors.Join(fmt.Errorf("connect to Redis: %w", err), client.client.Close())
	}
	return client, nil
}

func (c *Client) Ping(ctx context.Context) error {
	return c.client.Ping(ctx).Err()
}

func (c *Client) Close() error {
	return c.client.Close()
}

func (c *Client) Raw() *redis.Client {
	return c.client
}
