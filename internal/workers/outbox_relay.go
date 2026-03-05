package workers

import (
	"context"
	"os"
	"time"

	"github.com/flyluman/aelora/internal/ports"
	"go.uber.org/zap"
)

type MessageOutboxRelay struct {
	outbox ports.MessageOutboxRepository
	stream ports.StreamRepository
	bus    ports.EventBus
	log    *zap.Logger
	id     string
}

func NewMessageOutboxRelay(outbox ports.MessageOutboxRepository, stream ports.StreamRepository, bus ports.EventBus, log *zap.Logger) *MessageOutboxRelay {
	host, _ := os.Hostname()
	if host == "" {
		host = "unknown"
	}
	return &MessageOutboxRelay{outbox: outbox, stream: stream, bus: bus, log: log, id: host}
}

func (w *MessageOutboxRelay) Run(ctx context.Context, interval time.Duration, batchSize int) {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	if batchSize <= 0 {
		batchSize = 200
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pending, err := w.listPending(ctx, batchSize)
			if err != nil {
				w.log.Error("outbox list pending failed", zap.Error(err))
				continue
			}
			for _, item := range pending {
				msg := item.Message
				if !item.StreamAppended {
					if _, err := w.stream.AppendMessage(ctx, msg.RoomID, msg); err != nil {
						w.log.Warn("outbox stream append failed", zap.Error(err), zap.String("message_id", msg.ID))
						w.releaseClaim(ctx, msg.ID)
						continue
					}
					if err := w.outbox.MarkStreamAppended(ctx, msg.ID); err != nil {
						w.log.Warn("outbox mark stream appended failed", zap.Error(err), zap.String("message_id", msg.ID))
						w.releaseClaim(ctx, msg.ID)
						continue
					}
				}
				if err := w.bus.PublishMessageCreated(ctx, msg); err != nil {
					w.log.Warn("outbox publish failed", zap.Error(err), zap.String("message_id", msg.ID))
					w.releaseClaim(ctx, msg.ID)
					continue
				}
				if err := w.outbox.MarkDispatched(ctx, msg.ID); err != nil {
					w.log.Warn("outbox mark dispatched failed", zap.Error(err), zap.String("message_id", msg.ID))
				}
			}
		}
	}
}

func (w *MessageOutboxRelay) listPending(ctx context.Context, batchSize int) ([]ports.OutboxMessage, error) {
	return w.outbox.ClaimPending(ctx, w.id, batchSize, 30*time.Second)
}

func (w *MessageOutboxRelay) releaseClaim(ctx context.Context, messageID string) {
	if err := w.outbox.ReleaseClaim(ctx, w.id, messageID); err != nil {
		w.log.Debug("outbox release claim failed", zap.Error(err), zap.String("message_id", messageID))
	}
}
