package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Logger 使用 Zap 记录每次 HTTP 请求的基本信息（方法、路径、状态码、耗时）
func Logger(log *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 在请求进入 handler 前记录开始时间，用于后续计算耗时
		start := time.Now()
		path := c.Request.URL.Path

		// 执行后续中间件和 handler
		c.Next()

		// handler 执行完毕后，写入本次请求的结构化日志
		log.Info("请求",
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.String("ip", c.ClientIP()),
		)
	}
}
