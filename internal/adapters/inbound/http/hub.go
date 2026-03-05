package httpapi

import (
	"context"
	"sync"
	"time"

	"github.com/flyluman/aelora/internal/application"
	"github.com/flyluman/aelora/internal/domain"
	"github.com/flyluman/aelora/internal/ports"
	"go.uber.org/zap"
)

type WebSocketHub struct {
	rooms      ports.RoomRepository
	bus        ports.EventBus
	notifyPush *application.NotifyOfflineMembersUseCase
	log        *zap.Logger

	mu    sync.RWMutex
	users map[string]map[*wsClient]struct{}
}

func NewWebSocketHub(
	rooms ports.RoomRepository,
	bus ports.EventBus,
	notifyPush *application.NotifyOfflineMembersUseCase,
	log *zap.Logger,
) *WebSocketHub {
	return &WebSocketHub{
		rooms:      rooms,
		bus:        bus,
		notifyPush: notifyPush,
		log:        log,
		users:      make(map[string]map[*wsClient]struct{}),
	}
}

func (h *WebSocketHub) Run(ctx context.Context) {
	stream, unsubscribe, err := h.bus.SubscribeMessageCreated(ctx, 256)
	if err != nil {
		h.log.Error("hub subscribe failed", zap.Error(err))
		return
	}
	defer unsubscribe()

	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-stream:
			if !ok {
				return
			}
			h.broadcastMessageCreated(ctx, msg)
		}
	}
}

func (h *WebSocketHub) Register(client *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, ok := h.users[client.userID]; !ok {
		h.users[client.userID] = make(map[*wsClient]struct{})
	}
	h.users[client.userID][client] = struct{}{}
}

func (h *WebSocketHub) Unregister(client *wsClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	clients, ok := h.users[client.userID]
	if !ok {
		return
	}
	delete(clients, client)
	if len(clients) == 0 {
		delete(h.users, client.userID)
	}
	client.close()
}

func (h *WebSocketHub) broadcastMessageCreated(ctx context.Context, msg domain.Message) {
	members, err := h.rooms.ListMembers(ctx, msg.RoomID)
	if err != nil {
		h.log.Error("list room members failed", zap.Error(err), zap.String("room_id", msg.RoomID))
		return
	}

	response := wsResponse{Type: "message_created", Message: msg}
	for _, member := range members {
		if member == msg.SenderID {
			continue
		}
		clients := h.clientsForUser(member)
		for _, client := range clients {
			if !client.enqueue(response) {
				h.log.Warn("dropping ws frame due to full outbound queue", zap.String("user_id", member))
				h.Unregister(client)
			}
		}
		if len(clients) > 0 || h.notifyPush == nil {
			continue
		}
		pushCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		if err := h.notifyPush.Execute(pushCtx, msg, member); err != nil {
			h.log.Warn("offline push notify failed", zap.String("user_id", member), zap.String("room_id", msg.RoomID), zap.Error(err))
		}
		cancel()
	}
}

func (h *WebSocketHub) clientsForUser(userID string) []*wsClient {
	h.mu.RLock()
	defer h.mu.RUnlock()
	set, ok := h.users[userID]
	if !ok {
		return nil
	}
	out := make([]*wsClient, 0, len(set))
	for client := range set {
		out = append(out, client)
	}
	return out
}
