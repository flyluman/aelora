package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/flyluman/aelora/internal/application"
	"github.com/flyluman/aelora/internal/domain"
	"github.com/flyluman/aelora/internal/platform/auth"
	"github.com/flyluman/aelora/internal/ports"
	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

var (
	wsWriteWait  = 10 * time.Second
	wsPongWait   = 60 * time.Second
	wsPingPeriod = (wsPongWait * 9) / 10
	wsPresenceTT = 10 * time.Second
)

type WebSocketHandler struct {
	send        *application.SendMessageUseCase
	syncUC      *application.SyncMessagesUseCase
	ackUC       *application.AckMessageUseCase
	presence    ports.PresenceRepository
	hub         *WebSocketHub
	log         *zap.Logger
	authEnabled bool
	authn       auth.Validator
	readLim     int64
	upgrader    websocket.Upgrader
}

type wsRequest struct {
	Type        string `json:"type"`
	RoomID      string `json:"room_id"`
	Content     string `json:"content"`
	ClientMsgID string `json:"client_msg_id"`
	ReplyToID   string `json:"reply_to_message_id"`
	StreamID    string `json:"stream_id"`
	LastAck     string `json:"last_ack"`
	Limit       int    `json:"limit"`
}

type wsResponse struct {
	Type    string `json:"type"`
	Message any    `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

type wsClient struct {
	userID   string
	deviceID string
	conn     *websocket.Conn
	log      *zap.Logger

	send      chan wsResponse
	closeOnce sync.Once
	mu        sync.RWMutex
	closed    bool
}

func newWSClient(userID, deviceID string, conn *websocket.Conn, log *zap.Logger) *wsClient {
	return &wsClient{userID: userID, deviceID: deviceID, conn: conn, log: log, send: make(chan wsResponse, 128)}
}

func (c *wsClient) enqueue(msg wsResponse) bool {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return false
	}
	select {
	case c.send <- msg:
		c.mu.RUnlock()
		return true
	default:
		c.mu.RUnlock()
		return false
	}
}

func (c *wsClient) writePump() {
	pingTicker := time.NewTicker(wsPingPeriod)
	defer pingTicker.Stop()

	for {
		select {
		case msg, ok := <-c.send:
			if !ok {
				_ = c.conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := c.conn.WriteJSON(msg); err != nil {
				c.log.Warn("ws write failed", zap.String("user_id", c.userID), zap.String("device_id", c.deviceID), zap.Error(err))
				c.close()
				return
			}
		case <-pingTicker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
			if err := c.conn.WriteControl(websocket.PingMessage, []byte(strconv.FormatInt(time.Now().Unix(), 10)), time.Now().Add(wsWriteWait)); err != nil {
				c.log.Warn("ws ping failed", zap.String("user_id", c.userID), zap.String("device_id", c.deviceID), zap.Error(err))
				c.close()
				return
			}
		}
	}
}

func (c *wsClient) close() {
	c.closeOnce.Do(func() {
		c.mu.Lock()
		c.closed = true
		close(c.send)
		c.mu.Unlock()
		if c.conn != nil {
			_ = c.conn.Close()
		}
	})
}

func NewWebSocketHandler(
	send *application.SendMessageUseCase,
	syncUC *application.SyncMessagesUseCase,
	ackUC *application.AckMessageUseCase,
	presence ports.PresenceRepository,
	hub *WebSocketHub,
	log *zap.Logger,
	authEnabled bool,
	authn auth.Validator,
	wsReadLimit int64,
	wsCheckOrigin bool,
) *WebSocketHandler {
	h := &WebSocketHandler{
		send:        send,
		syncUC:      syncUC,
		ackUC:       ackUC,
		presence:    presence,
		hub:         hub,
		log:         log,
		authEnabled: authEnabled,
		authn:       authn,
		readLim:     wsReadLimit,
	}
	h.upgrader = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			if !wsCheckOrigin {
				return true
			}
			origin := r.Header.Get("Origin")
			host := "http://" + r.Host
			return origin == host || origin == "https://"+r.Host
		},
	}
	return h
}

func (h *WebSocketHandler) Handle(c echo.Context) error {
	r := c.Request()
	w := c.Response().Writer

	identity, ok := h.resolveIdentity(r)
	if !ok {
		h.log.Warn("audit.auth_failed",
			zap.String("auth_transport", "websocket"),
			zap.String("remote", r.RemoteAddr),
		)
		return c.String(http.StatusUnauthorized, "unauthorized")
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Error("ws upgrade failed", zap.Error(err))
		return nil
	}
	conn.SetReadLimit(h.readLim)
	_ = conn.SetReadDeadline(time.Now().Add(wsPongWait))
	conn.SetPongHandler(func(_ string) error {
		return conn.SetReadDeadline(time.Now().Add(wsPongWait))
	})

	client := newWSClient(identity.UserID, identity.DeviceID, conn, h.log)
	h.hub.Register(client)
	defer h.hub.Unregister(client)
	if err := h.presence.SetOnline(r.Context(), identity.UserID, identity.DeviceID); err != nil {
		h.log.Warn("ws set online failed", zap.String("user_id", identity.UserID), zap.String("device_id", identity.DeviceID), zap.Error(err))
	}
	stopPresenceRefresh := make(chan struct{})
	defer close(stopPresenceRefresh)
	go h.refreshPresence(stopPresenceRefresh, identity.UserID, identity.DeviceID)
	defer func() {
		if err := h.presence.SetOffline(context.Background(), identity.UserID, identity.DeviceID); err != nil {
			h.log.Warn("ws set offline failed", zap.String("user_id", identity.UserID), zap.String("device_id", identity.DeviceID), zap.Error(err))
		}
	}()

	go client.writePump()
	h.log.Info("ws connected", zap.String("user_id", identity.UserID), zap.String("device_id", identity.DeviceID), zap.String("remote", r.RemoteAddr))

	for {
		var req wsRequest
		if err := conn.ReadJSON(&req); err != nil {
			fields := []zap.Field{zap.String("user_id", identity.UserID), zap.String("device_id", identity.DeviceID), zap.Error(err)}
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
				h.log.Warn("ws disconnected unexpectedly", fields...)
			} else {
				h.log.Info("ws disconnected", fields...)
			}
			return nil
		}

		switch req.Type {
		case "send_message":
			h.handleSendMessage(r.Context(), client, identity.UserID, req)
		case "ACK":
			h.handleAck(r.Context(), client, identity.UserID, identity.DeviceID, req)
		case "SYNC":
			h.handleSync(r.Context(), client, identity.UserID, identity.DeviceID, req)
		default:
			_ = client.enqueue(wsResponse{Type: "error", Error: "unsupported type"})
		}
	}
}

func (h *WebSocketHandler) refreshPresence(stop <-chan struct{}, userID, deviceID string) {
	ticker := time.NewTicker(wsPresenceTT)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			if err := h.presence.SetOnline(ctx, userID, deviceID); err != nil {
				h.log.Warn("ws presence refresh failed", zap.String("user_id", userID), zap.String("device_id", deviceID), zap.Error(err))
			}
			cancel()
		}
	}
}

func (h *WebSocketHandler) resolveIdentity(r *http.Request) (auth.Identity, bool) {
	if h.authEnabled && h.authn != nil {
		identity, err := h.authn.ValidateToken(r.Context(), r.Header.Get("Authorization"), r.Header.Get("X-Device-ID"))
		if err != nil {
			return auth.Identity{}, false
		}
		return identity, true
	}

	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		userID = r.Header.Get("X-User-ID")
	}
	if userID == "" {
		return auth.Identity{}, false
	}
	deviceID := r.URL.Query().Get("device_id")
	if deviceID == "" {
		deviceID = "unknown"
	}
	return auth.Identity{UserID: userID, DeviceID: deviceID}, true
}

func (h *WebSocketHandler) handleSendMessage(ctx context.Context, client *wsClient, userID string, req wsRequest) {
	msg, err := h.send.Execute(ctx, application.SendMessageCommand{
		RoomID:      req.RoomID,
		SenderID:    userID,
		Content:     req.Content,
		ClientMsgID: req.ClientMsgID,
		ReplyToID:   req.ReplyToID,
	})
	if err != nil {
		fields := []zap.Field{
			zap.String("user_id", userID),
			zap.String("room_id", req.RoomID),
			zap.String("client_msg_id", req.ClientMsgID),
		}
		if req.ReplyToID != "" {
			fields = append(fields, zap.String("reply_to_message_id", req.ReplyToID))
		}
		h.logWSError("ws send message failed", err, fields...)

		switch {
		case errors.Is(err, application.ErrDuplicateMessage):
			_ = client.enqueue(wsResponse{Type: "error", Error: "duplicate client_msg_id"})
		case errors.Is(err, application.ErrRateLimited):
			_ = client.enqueue(wsResponse{Type: "error", Error: "rate limit exceeded"})
		case errors.Is(err, application.ErrRoomAccessDenied):
			_ = client.enqueue(wsResponse{Type: "error", Error: "room access denied"})
		case errors.Is(err, application.ErrReplyTarget):
			_ = client.enqueue(wsResponse{Type: "error", Error: "reply target not found"})
		case errors.Is(err, domain.ErrInvalidMessage):
			_ = client.enqueue(wsResponse{Type: "error", Error: "invalid message payload"})
		default:
			_ = client.enqueue(wsResponse{Type: "error", Error: "internal error"})
		}
		return
	}
	_ = client.enqueue(wsResponse{
		Type: "message_accepted",
		Message: map[string]any{
			"message_id":          msg.ID,
			"room_id":             msg.RoomID,
			"sender_id":           msg.SenderID,
			"client_msg_id":       msg.ClientMsgID,
			"reply_to_message_id": msg.ReplyToID,
			"created_at":          msg.CreatedAt,
		},
	})
}

func (h *WebSocketHandler) handleAck(ctx context.Context, client *wsClient, userID, deviceID string, req wsRequest) {
	if err := h.ackUC.Execute(ctx, application.AckMessageCommand{RoomID: req.RoomID, UserID: userID, DeviceID: deviceID, StreamID: req.StreamID}); err != nil {
		h.logWSError("ws ack failed", err,
			zap.String("user_id", userID),
			zap.String("device_id", deviceID),
			zap.String("room_id", req.RoomID),
			zap.String("stream_id", req.StreamID),
		)
		if errors.Is(err, application.ErrInvalidAck) {
			_ = client.enqueue(wsResponse{Type: "error", Error: "invalid ack payload"})
			return
		}
		_ = client.enqueue(wsResponse{Type: "error", Error: "ack failed"})
		return
	}
	_ = client.enqueue(wsResponse{Type: "ack_saved", Message: map[string]string{"stream_id": req.StreamID}})
}

func (h *WebSocketHandler) handleSync(ctx context.Context, client *wsClient, userID, deviceID string, req wsRequest) {
	result, err := h.syncUC.Execute(ctx, application.SyncMessagesQuery{RoomID: req.RoomID, UserID: userID, DeviceID: deviceID, LastAck: req.LastAck, Limit: req.Limit})
	if err != nil {
		h.logWSError("ws sync failed", err,
			zap.String("user_id", userID),
			zap.String("device_id", deviceID),
			zap.String("room_id", req.RoomID),
			zap.String("last_ack", req.LastAck),
			zap.Int("limit", req.Limit),
		)
		_ = client.enqueue(wsResponse{Type: "error", Error: "sync failed"})
		return
	}
	_ = client.enqueue(wsResponse{Type: "sync_result", Message: map[string]any{
		"room_id":  req.RoomID,
		"messages": result.Messages,
		"replies":  result.Replies,
		"count":    len(result.Messages),
	}})
}

func (h *WebSocketHandler) logWSError(message string, err error, fields ...zap.Field) {
	fields = append(fields, zap.Error(err))
	if isWSClientError(err) {
		h.log.Warn(message, fields...)
		return
	}
	h.log.Error(message, fields...)
}

func isWSClientError(err error) bool {
	switch {
	case errors.Is(err, application.ErrDuplicateMessage):
		return true
	case errors.Is(err, application.ErrRateLimited):
		return true
	case errors.Is(err, application.ErrRoomAccessDenied):
		return true
	case errors.Is(err, application.ErrReplyTarget):
		return true
	case errors.Is(err, application.ErrInvalidAck):
		return true
	case errors.Is(err, domain.ErrInvalidMessage):
		return true
	case errors.Is(err, domain.ErrInvalidRoom):
		return true
	default:
		return false
	}
}
