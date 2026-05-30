package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// Recovery 捕获 handler 中的 panic，记录错误日志并返回 500，避免服务崩溃
func Recovery(log *zap.Logger) gin.HandlerFunc {
	return gin.RecoveryWithWriter(nil, func(c *gin.Context, err interface{}) {
		log.Error("panic 恢复",
			zap.Any("error", err),
			zap.String("path", c.Request.URL.Path),
		)
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "服务器内部错误",
		})
	})
}
