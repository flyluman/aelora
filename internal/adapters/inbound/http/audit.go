package httpapi

import (
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func (h *Handler) auditInfo(c echo.Context, event string, fields ...zap.Field) {
	h.logAudit(c, zapcore.InfoLevel, event, fields...)
}

func (h *Handler) auditWarn(c echo.Context, event string, fields ...zap.Field) {
	h.logAudit(c, zapcore.WarnLevel, event, fields...)
}

func (h *Handler) logAudit(c echo.Context, level zapcore.Level, event string, fields ...zap.Field) {
	req := c.Request()
	base := []zap.Field{
		zap.String("method", req.Method),
		zap.String("path", req.URL.Path),
		zap.String("route", requestRoute(c)),
	}
	if traceID, ok := c.Get("trace_id").(string); ok && traceID != "" {
		base = append(base, zap.String("trace_id", traceID))
	}
	base = append(base, zap.String("audit_event", event))
	base = append(base, fields...)

	logger := h.logger()
	switch level {
	case zapcore.WarnLevel:
		logger.Warn(event, base...)
	default:
		logger.Info(event, base...)
	}
}
