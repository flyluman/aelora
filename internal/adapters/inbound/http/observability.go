package httpapi

import (
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/flyluman/aelora/internal/platform/config"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func withTraceID() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			traceID := req.Header.Get(echo.HeaderXRequestID)
			if traceID == "" {
				traceID = req.Header.Get("X-Trace-ID")
				if traceID == "" {
					traceID := c.Response().Header().Get(echo.HeaderXRequestID)
					c.Set("trace_id", traceID)
				}
			}
			c.Set("trace_id", traceID)
			return next(c)
		}
	}
}

func withHTTPLogging(logger *zap.Logger, cfg config.Config) echo.MiddlewareFunc {
	policy := newRequestLogPolicy(cfg)

	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			start := time.Now()
			err := next(c)
			if err != nil {
				c.Error(err)
			}

			req := c.Request()
			res := c.Response()
			status := res.Status

			fields := []zap.Field{
				zap.String("method", req.Method),
				zap.String("path", req.URL.Path),
				zap.String("route", requestRoute(c)),
				zap.String("remote_addr", req.RemoteAddr),
				zap.Int("status", status),
				zap.Duration("duration", time.Since(start)),
				zap.Int64("response_bytes", res.Size),
			}

			if traceID, ok := c.Get("trace_id").(string); ok && traceID != "" {
				fields = append(fields, zap.String("trace_id", traceID))
			}
			if userAgent := req.UserAgent(); userAgent != "" {
				fields = append(fields, zap.String("user_agent", userAgent))
			}
			if req.ContentLength > 0 {
				fields = append(fields, zap.Int64("content_length", req.ContentLength))
			}
			if userID := identityFromContext(c); userID != "" {
				fields = append(fields, zap.String("user_id", userID))
			}
			if deviceID := strings.TrimSpace(req.Header.Get("X-Device-ID")); deviceID != "" {
				fields = append(fields, zap.String("device_id", deviceID))
			}
			if queryKeys := requestQueryKeys(req); len(queryKeys) > 0 {
				fields = append(fields, zap.Strings("query_keys", queryKeys))
			}

			decision := policy.decide(c, status)
			if !decision.log {
				return err
			}
			switch decision.level {
			case zapcore.ErrorLevel:
				logger.Error("http request completed", fields...)
			case zapcore.WarnLevel:
				logger.Warn("http request completed", fields...)
			case zapcore.DebugLevel:
				logger.Debug("http request completed", fields...)
			default:
				logger.Info("http request completed", fields...)
			}

			return err
		}
	}
}

func requestRoute(c echo.Context) string {
	path := c.Path()
	if path != "" {
		return path
	}
	return c.Request().URL.Path
}

func requestQueryKeys(r *http.Request) []string {
	if r == nil || r.URL == nil {
		return nil
	}
	query := r.URL.Query()
	if len(query) == 0 {
		return nil
	}
	keys := make([]string, 0, len(query))
	for key := range query {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
