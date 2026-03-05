package valkey

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/flyluman/aelora/internal/domain"
	"github.com/flyluman/aelora/internal/ports"
	"github.com/valkey-io/valkey-go"
)

type StreamRepository struct {
	client         valkey.Client
	eventStreamKey string
	roomMaxLen     int64
	eventMaxLen    int64
}

func NewStreamRepository(client valkey.Client, eventStreamKey string, roomMaxLen, eventMaxLen int64) *StreamRepository {
	if eventStreamKey == "" {
		eventStreamKey = "stream:events:message_created"
	}
	if roomMaxLen <= 0 {
		roomMaxLen = 20000
	}
	if eventMaxLen <= 0 {
		eventMaxLen = 50000
	}
	return &StreamRepository{
		client:         client,
		eventStreamKey: eventStreamKey,
		roomMaxLen:     roomMaxLen,
		eventMaxLen:    eventMaxLen,
	}
}

func (r *StreamRepository) AppendMessage(ctx context.Context, roomID string, msg domain.Message) (string, error) {
	payload, err := json.Marshal(msg)
	if err != nil {
		return "", err
	}
	resp := r.client.Do(
		ctx,
		r.client.B().
			Xadd().
			Key(StreamKey(roomID)).
			Maxlen().
			Almost().
			Threshold(strconv.FormatInt(r.roomMaxLen, 10)).
			Id("*").
			FieldValue().
			FieldValue("message", string(payload)).
			Build(),
	)
	if err := resp.Error(); err != nil {
		return "", err
	}
	return resp.ToString()
}

func (r *StreamRepository) ReadFrom(ctx context.Context, roomID, lastAck string, limit int) ([]ports.StreamMessage, error) {
	if limit <= 0 {
		limit = 100
	}
	if lastAck == "" {
		lastAck = "0"
	}

	resp := r.client.Do(ctx, r.client.B().Xread().Count(int64(limit)).Streams().Key(StreamKey(roomID)).Id(lastAck).Build())
	if err := resp.Error(); err != nil {
		if valkey.IsValkeyNil(err) {
			return nil, nil
		}
		return nil, err
	}

	data, err := resp.AsXRead()
	if err != nil {
		if valkey.IsValkeyNil(err) {
			return nil, nil
		}
		return nil, err
	}

	items := data[StreamKey(roomID)]
	out := make([]ports.StreamMessage, 0, len(items))
	for _, item := range items {
		raw, ok := item.FieldValues["message"]
		if !ok {
			continue
		}
		var msg domain.Message
		if err := json.Unmarshal([]byte(raw), &msg); err != nil {
			continue
		}
		out = append(out, ports.StreamMessage{StreamID: item.ID, RoomID: roomID, Message: msg})
	}
	return out, nil
}

func (r *StreamRepository) PublishMessageCreated(ctx context.Context, msg domain.Message) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return r.client.Do(
		ctx,
		r.client.B().
			Xadd().
			Key(r.eventStreamKey).
			Maxlen().
			Almost().
			Threshold(strconv.FormatInt(r.eventMaxLen, 10)).
			Id("*").
			FieldValue().
			FieldValue("message", string(payload)).
			Build(),
	).Error()
}

func (r *StreamRepository) SubscribeMessageCreated(ctx context.Context, buffer int) (<-chan domain.Message, func(), error) {
	if buffer <= 0 {
		buffer = 128
	}
	out := make(chan domain.Message, buffer)

	subCtx, cancel := context.WithCancel(ctx)
	go func() {
		defer close(out)
		lastID := "$"
		for {
			select {
			case <-subCtx.Done():
				return
			default:
			}

			resp := r.client.Do(subCtx, r.client.B().Xread().Count(100).Block((2 * time.Second).Milliseconds()).Streams().Key(r.eventStreamKey).Id(lastID).Build())
			if err := resp.Error(); err != nil {
				if valkey.IsValkeyNil(err) || errors.Is(err, context.Canceled) {
					continue
				}
				continue
			}

			data, err := resp.AsXRead()
			if err != nil {
				if valkey.IsValkeyNil(err) {
					continue
				}
				continue
			}
			items := data[r.eventStreamKey]
			for _, item := range items {
				lastID = item.ID
				raw, ok := item.FieldValues["message"]
				if !ok {
					continue
				}
				var msg domain.Message
				if err := json.Unmarshal([]byte(raw), &msg); err != nil {
					continue
				}
				select {
				case out <- msg:
				case <-subCtx.Done():
					return
				}
			}
		}
	}()

	unsub := func() { cancel() }
	return out, unsub, nil
}

func StreamKey(roomID string) string {
	return "stream:room:" + roomID
}
