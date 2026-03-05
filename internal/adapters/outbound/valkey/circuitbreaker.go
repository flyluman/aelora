package valkey

import (
	"context"

	"github.com/sony/gobreaker"
)

type circuitBreakerClient struct {
	inner *Client
	cb    *gobreaker.CircuitBreaker
}

func newCircuitBreakerClient(inner *Client) *circuitBreakerClient {
	return &circuitBreakerClient{
		inner: inner,
		cb: gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name: "valkey",
		}),
	}
}

func (c *circuitBreakerClient) Ping(ctx context.Context) error {
	_, err := c.cb.Execute(func() (any, error) {
		return nil, c.inner.Ping(ctx)
	})
	return err
}
