package cache

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/vpt/blog-backend/pkg/config"
)

// NewRedis 初始化 Redis 客户端并验证连通性
func NewRedis(cfg *config.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// 启动时 ping 一次，确认 Redis 可达
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("Redis 连接失败 (%s): %w", cfg.Addr, err)
	}

	return client, nil
}
