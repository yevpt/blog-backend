package cache

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/vpt/blog-backend/pkg/config"
)

// NewRedis 初始化 Redis 客户端，启动时 Ping 验证连通性，失败则阻断服务启动
func NewRedis(cfg *config.RedisConfig) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("Redis 连接失败 (%s): %w", cfg.Addr, err)
	}

	return client, nil
}
