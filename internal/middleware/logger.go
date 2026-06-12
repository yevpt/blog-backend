package middleware

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"go.uber.org/zap"
)

// Logger 使用 Zap 记录每次 HTTP 请求的结构化信息，便于线上检索和请求链路排查。
func Logger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 在请求进入 handler 前记录开始时间，用于后续计算耗时
		start := time.Now()
		path := c.Request.URL.Path

		// 执行后续中间件和 handler
		c.Next()

		// handler 执行完毕后，写入本次请求的结构化日志
		fields := []zap.Field{
			zap.String("type", "request"),
			zap.String("request_id", GetRequestID(c)),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("query", c.Request.URL.RawQuery),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.String("ip", c.ClientIP()),
			zap.String("user_agent", c.Request.UserAgent()),
			zap.String("referer", c.Request.Referer()),
		}

		if claims := jwtpkg.GetClaims(c); claims != nil && claims.UserId > 0 {
			fields = append(fields, zap.Int64("user_id", claims.UserId))
		}
		if len(c.Errors) > 0 {
			fields = append(fields, zap.String("errors", c.Errors.String()))
		}

		switch status := c.Writer.Status(); {
		case status >= http.StatusInternalServerError:
			log.Error("请求", fields...)
		case status >= http.StatusBadRequest:
			log.Warn("请求", fields...)
		default:
			log.Info("请求", fields...)
		}
	}
}
