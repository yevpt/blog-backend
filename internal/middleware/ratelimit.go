package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/vpt/blog-backend/pkg/response"
)

// RateLimitConfig 限流配置
type RateLimitConfig struct {
	Window      time.Duration // 计数窗口
	SoftLimit   int           // 超过此次数返回 429
	HardLimit   int           // 超过此次数封禁 IP
	BanDuration time.Duration // 封禁时长
}

// RateLimitStrict 适用于高风险接口（send-code、register）：5次软/20次硬/15min封禁
func RateLimitStrict(rdb *redis.Client) gin.HandlerFunc {
	return newRateLimiter(rdb, RateLimitConfig{
		Window:      60 * time.Second,
		SoftLimit:   5,
		HardLimit:   20,
		BanDuration: 15 * time.Minute,
	})
}

// RateLimitNormal 适用于普通敏感接口（login）：10次软/30次硬/15min封禁
func RateLimitNormal(rdb *redis.Client) gin.HandlerFunc {
	return newRateLimiter(rdb, RateLimitConfig{
		Window:      60 * time.Second,
		SoftLimit:   10,
		HardLimit:   30,
		BanDuration: 15 * time.Minute,
	})
}

func newRateLimiter(rdb *redis.Client, cfg RateLimitConfig) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := context.Background()
		ip := c.ClientIP()

		// 检查全局封禁
		banKey := fmt.Sprintf("ban:ip:%s", ip)
		banned, _ := rdb.Exists(ctx, banKey).Result()
		if banned > 0 {
			ttl, _ := rdb.TTL(ctx, banKey).Result()
			response.TooManyRequests(c, "IP 已被封禁，请稍后再试", int(ttl.Seconds()))
			c.Abort()
			return
		}

		// 按路由+IP 计数（c.FullPath() 自动派生 key，无需手动传名称）
		// 使用 Pipeline 原子执行 Incr+Expire，避免 Incr 成功而 Expire 未执行导致 key 永不过期
		routeKey := fmt.Sprintf("ratelimit:%s:%s", c.FullPath(), ip)
		pipe := rdb.Pipeline()
		incrCmd := pipe.Incr(ctx, routeKey)
		pipe.Expire(ctx, routeKey, cfg.Window)
		pipe.Exec(ctx)
		count := incrCmd.Val()

		if count > int64(cfg.HardLimit) {
			rdb.Set(ctx, banKey, 1, cfg.BanDuration)
			response.TooManyRequests(c, "请求过于频繁，IP 已被临时封禁", int(cfg.BanDuration.Seconds()))
			c.Abort()
			return
		}

		if count > int64(cfg.SoftLimit) {
			response.TooManyRequests(c, "请求过于频繁，请稍后再试", int(cfg.Window.Seconds()))
			c.Abort()
			return
		}

		c.Next()
	}
}
