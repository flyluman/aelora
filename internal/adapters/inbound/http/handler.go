package httpapi

import (
	"context"
	"net/http"

	"github.com/flyluman/aelora/internal/app"
	"github.com/flyluman/aelora/internal/platform/health"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"
)

const identityKey = "user_id"

type Handler struct {
	app    *app.App
	hub    *WebSocketHub
	ws     *WebSocketHandler
	health *health.Service
}

func NewHandler(a *app.App, hub *WebSocketHub, ws *WebSocketHandler, health *health.Service) *Handler {
	return &Handler{
		app:    a,
		hub:    hub,
		ws:     ws,
		health: health,
	}
}

func (h *Handler) RunRealtime(ctx context.Context) {
	go h.hub.Run(ctx)
}

func (h *Handler) RegisterRoutes(e *echo.Echo) {
	e.HideBanner = true

	e.Use(middleware.Recover())
	e.Use(middleware.RequestID())
	e.Use(withTraceID())
	e.Use(withHTTPLogging(h.logger(), h.app.Cfg))

	e.GET("/livez", h.live)
	e.GET("/readyz", h.ready)
	e.GET("/healthz", h.ready)

	v1 := e.Group("/v1")
	v1.Use(h.withAuth)

	v1.POST("/rooms", h.createRoom)
	v1.POST("/rooms/:room_id/members/:user_id", h.joinRoom)
	v1.POST("/rooms/:room_id/messages", h.sendMessage)
	v1.GET("/rooms/:room_id/messages", h.listMessages)
	v1.PUT("/users/me/devices/:device_id/push-token", h.registerPushTokenHandler)
	v1.DELETE("/users/me/devices/:device_id/push-token", h.deletePushTokenHandler)

	e.GET("/v1/ws", h.ws.Handle)
}

func (h *Handler) withAuth(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if !h.app.Cfg.AuthEnabled || h.app.Auth == nil {
			userID := c.Request().Header.Get("X-User-ID")
			if userID == "" {
				userID = c.QueryParam("user_id")
			}
			c.Set(identityKey, userID)
			return next(c)
		}

		identity, err := h.app.Auth.ValidateToken(
			c.Request().Context(),
			c.Request().Header.Get("Authorization"),
			c.Request().Header.Get("X-Device-ID"),
		)
		if err != nil {
			return writeError(c, http.StatusUnauthorized, "unauthorized", "unauthorized")
		}
		c.Set(identityKey, identity.UserID)
		return next(c)
	}
}

func (h *Handler) logger() *zap.Logger {
	if h != nil && h.app != nil && h.app.Log != nil {
		return h.app.Log
	}
	return zap.NewNop()
}
