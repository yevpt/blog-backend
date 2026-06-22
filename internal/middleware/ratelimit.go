package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
)

// RateLimitConfig 限流参数，软限返回 429，硬限封禁 IP
type RateLimitConfig struct {
	Window      time.Duration // 计数滑动窗口
	SoftLimit   int           // 超过此次数触发 429，不封禁
	HardLimit   int           // 超过此次数写入封禁标记
	BanDuration time.Duration // 封禁时长
}

// RateLimitStrict 高风险接口限流（send-code、register），60s 内 5 次软限 / 20 次硬限 / 封禁 15min
func RateLimitStrict(rdb *redis.Client) gin.HandlerFunc {
	return newIPRateLimiter(rdb, RateLimitConfig{
		Window:      60 * time.Second,
		SoftLimit:   5,
		HardLimit:   20,
		BanDuration: 15 * time.Minute,
	})
}

// RateLimitNormal 普通敏感接口限流（login），60s 内 10 次软限 / 30 次硬限 / 封禁 15min
func RateLimitNormal(rdb *redis.Client) gin.HandlerFunc {
	return newIPRateLimiter(rdb, RateLimitConfig{
		Window:      60 * time.Second,
		SoftLimit:   10,
		HardLimit:   30,
		BanDuration: 15 * time.Minute,
	})
}

// RateLimitMomentUpload 按登录用户限制碎语保存频率，降低恶意批量上传图片的资源消耗。
func RateLimitMomentUpload(rdb *redis.Client) gin.HandlerFunc {
	return newPrincipalRateLimiter(rdb, RateLimitConfig{
		Window:      60 * time.Second,
		SoftLimit:   5,
		HardLimit:   20,
		BanDuration: 15 * time.Minute,
	}, momentUploadRateLimitPrincipal, momentUploadRateLimitBanKey)
}

func newIPRateLimiter(rdb *redis.Client, cfg RateLimitConfig) gin.HandlerFunc {
	return newPrincipalRateLimiter(
		rdb,
		cfg,
		func(c *gin.Context) string { return "ip:" + c.ClientIP() },
		func(principal string) string { return "ban:" + principal },
	)
}

func newPrincipalRateLimiter(
	rdb *redis.Client,
	cfg RateLimitConfig,
	principalFn func(*gin.Context) string,
	banKeyFn func(string) string,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := context.Background()
		principal := principalFn(c)

		// 优先检查 IP 是否处于硬封禁状态，封禁期内直接拒绝，跳过后续计数操作
		banKey := banKeyFn(principal)
		banned, _ := rdb.Exists(ctx, banKey).Result()
		if banned > 0 {
			// 读取剩余封禁时间并写入 Retry-After header，告知客户端最早重试时机
			ttl, _ := rdb.TTL(ctx, banKey).Result()
			response.TooManyRequests(c, "IP 已被封禁，请稍后再试", int(ttl.Seconds()))
			c.Abort()
			return
		}

		// Pipeline 原子执行 Incr+Expire，避免 Incr 成功而 Expire 未执行导致 key 永不过期
		// key 包含 FullPath() 实现按路由独立计数，无需手动命名
		routeKey := fmt.Sprintf("ratelimit:%s:%s", c.FullPath(), principal)
		pipe := rdb.Pipeline()
		incrCmd := pipe.Incr(ctx, routeKey)
		pipe.Expire(ctx, routeKey, cfg.Window)
		pipe.Exec(ctx)
		count := incrCmd.Val()

		// 超过硬限：写入封禁标记并拒绝请求，后续请求直接走封禁逻辑，不再计数
		if count > int64(cfg.HardLimit) {
			rdb.Set(ctx, banKey, 1, cfg.BanDuration)
			response.TooManyRequests(c, "请求过于频繁，IP 已被临时封禁", int(cfg.BanDuration.Seconds()))
			c.Abort()
			return
		}

		// 超过软限：返回 429 但不封禁，给客户端减速信号
		if count > int64(cfg.SoftLimit) {
			response.TooManyRequests(c, "请求过于频繁，请稍后再试", int(cfg.Window.Seconds()))
			c.Abort()
			return
		}

		c.Next()
	}
}

func momentUploadRateLimitPrincipal(c *gin.Context) string {
	detail := GetUserDetail(c)
	if detail != nil && detail.ID > 0 {
		return fmt.Sprintf("user:%d", detail.ID)
	}
	claims := jwt.GetClaims(c)
	if claims != nil && claims.UserId > 0 {
		return fmt.Sprintf("user:%d", claims.UserId)
	}
	return "ip:" + c.ClientIP()
}

func momentUploadRateLimitBanKey(principal string) string {
	return "ban:moment-upload:" + principal
}
