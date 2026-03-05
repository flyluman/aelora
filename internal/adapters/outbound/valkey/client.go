package valkey

import (
	"context"
	"fmt"

	"github.com/valkey-io/valkey-go"
)

type Client struct {
	valkey.Client
}

func NewClient(addr string, username, password string, db int) (*Client, error) {
	opts := valkey.ClientOption{
		InitAddress: []string{addr},
		SelectDB:    db,
	}
	if username != "" {
		opts.Username = username
	}
	if password != "" {
		opts.Password = password
	}
	client, err := valkey.NewClient(opts)
	if err != nil {
		return nil, fmt.Errorf("valkey connect: %w", err)
	}
	return &Client{client}, nil
}

func (c *Client) Ping(ctx context.Context) error {
	return c.Do(ctx, c.B().Ping().Build()).Error()
}

func (c *Client) Close() {
	c.Client.Close()
}
