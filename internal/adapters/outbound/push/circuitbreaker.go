package push

import (
	"context"

	"github.com/flyluman/aelora/internal/domain"
	"github.com/flyluman/aelora/internal/ports"
	"github.com/sony/gobreaker"
)

type circuitBreakerNotifier struct {
	inner *HTTPNotifier
	cb    *gobreaker.CircuitBreaker
}

func newCircuitBreakerNotifier(inner *HTTPNotifier) *circuitBreakerNotifier {
	return &circuitBreakerNotifier{
		inner: inner,
		cb: gobreaker.NewCircuitBreaker(gobreaker.Settings{
			Name: "push",
		}),
	}
}

func (n *circuitBreakerNotifier) NotifyMessage(ctx context.Context, token ports.PushToken, message domain.Message) error {
	_, err := n.cb.Execute(func() (any, error) {
		return nil, n.inner.NotifyMessage(ctx, token, message)
	})
	return err
}
