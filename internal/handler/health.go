package handler

import (
	"context"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/vpt/blog-backend/pkg/response"
	"gorm.io/gorm"
)

// HealthHandler 处理健康检查相关请求
type HealthHandler struct {
	db    *gorm.DB
	redis *redis.Client
}

// NewHealthHandler 创建健康检查 handler
func NewHealthHandler(db *gorm.DB, redis *redis.Client) *HealthHandler {
	return &HealthHandler{db: db, redis: redis}
}

// Check 检查 DB 和 Redis 连通状态，供运维和开发调试使用
// GET /health
func (h *HealthHandler) Check(c *gin.Context) {
	status := gin.H{
		"status": "ok",
		"db":     checkDB(h.db),
		"redis":  checkRedis(h.redis),
	}
	response.Success(c, status)
}

func checkDB(db *gorm.DB) string {
	sqlDB, err := db.DB()
	if err != nil {
		return "error: " + err.Error()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		return "error: " + err.Error()
	}
	return "ok"
}

func checkRedis(client *redis.Client) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return "error: " + err.Error()
	}
	return "ok"
}
