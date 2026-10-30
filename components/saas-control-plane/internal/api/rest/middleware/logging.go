package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RequestLogger logs each HTTP request with timing info. It deliberately does
// not log raw headers or query strings because either can carry credentials.
func RequestLogger(logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		fields := []zap.Field{
			zap.Int("status", status),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.Bool("query_present", c.Request.URL.RawQuery != ""),
			zap.Duration("latency", latency),
			zap.String("ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
		}

		for _, key := range []string{"user_id", "tenant_id", "satellite_id", "command_id", "command_action", "command_status", "command_count"} {
			if value, exists := c.Get(key); exists {
				fields = append(fields, zap.Any(key, value))
			}
		}

		if status >= 500 {
			logger.Error("request", fields...)
		} else if status >= 400 {
			logger.Warn("request", fields...)
		} else {
			logger.Info("request", fields...)
		}
	}
}
