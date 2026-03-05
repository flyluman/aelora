package postgres

import (
	"github.com/sony/gobreaker"
)

type circuitBreakerPool struct {
	*Pool
	cb *gobreaker.CircuitBreaker
}

func newCircuitBreakerPool(inner *Pool) *circuitBreakerPool {
	return &circuitBreakerPool{
		Pool: inner,
		cb: gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name: "postgres",
		}),
	}
}
