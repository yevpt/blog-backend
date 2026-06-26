package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/vpt/blog-backend/pkg/jwt"
	"github.com/vpt/blog-backend/pkg/response"
	"github.com/vpt/blog-backend/pkg/roles"
)

// escalatingBanDurations 递进式封禁时长配置，严格模式（认证/资源接口）
// 第 1 次：10 分钟，第 2 次：30 分钟，第 3 次：2 小时，第 4 次+：24 小时
var escalatingBanDurations = []time.Duration{
	10 * time.Minute,
	30 * time.Minute,
	2 * time.Hour,
	24 * time.Hour,
}

// escalatingBanDurationsPublic 递进式封禁时长配置，公开接口温和模式
// 第 1 次：5 分钟，第 2 次：30 分钟，第 3 次：2 小时，第 4 次：24 小时，第 5 次：48 小时，第 6 次+：7 天
var escalatingBanDurationsPublic = []time.Duration{
	5 * time.Minute,
	30 * time.Minute,
	2 * time.Hour,
	24 * time.Hour,
	48 * time.Hour,
	7 * 24 * time.Hour,
}

// banCountWindow 计数器重置窗口，24 小时内的历史封禁会累计计数
const banCountWindow = 24 * time.Hour

// banCountWindowPublic 公开接口计数器窗口，7 天内的历史封禁会累计计数
const banCountWindowPublic = 7 * 24 * time.Hour

// RateLimitConfig 限流参数，软限返回 429，硬限触发递进式封禁（时长由 escalatingBanDurations(Public) 决定）
type RateLimitConfig struct {
	Window    time.Duration // 计数滑动窗口
	SoftLimit int           // 超过此次数触发 429，不封禁
	HardLimit int           // 超过此次数写入封禁标记
}

// RateLimitStrict 高风险接口限流（send-code、register），60s 内 5 次软限 / 20 次硬限
// 触发硬限后按 strict 档独立计数应用递进式封禁，不与其他档位互相影响
func RateLimitStrict(rdb *redis.Client) gin.HandlerFunc {
	return newIPRateLimiter(rdb, RateLimitConfig{
		Window:    60 * time.Second,
		SoftLimit: 5,
		HardLimit: 20,
	}, "strict")
}

// RateLimitNormal 普通敏感接口限流，60s 内 10 次软限 / 30 次硬限
// 触发硬限后按 normal 档独立计数应用递进式封禁，不与其他档位互相影响
func RateLimitNormal(rdb *redis.Client) gin.HandlerFunc {
	return newIPRateLimiter(rdb, RateLimitConfig{
		Window:    60 * time.Second,
		SoftLimit: 10,
		HardLimit: 30,
	}, "normal")
}

// RateLimitLoose 登录、OAuth 等认证接口，120s 内 30 次软限 / 100 次硬限
// 触发硬限后按 loose 档独立计数应用递进式封禁，不与其他档位互相影响
func RateLimitLoose(rdb *redis.Client) gin.HandlerFunc {
	return newIPRateLimiter(rdb, RateLimitConfig{
		Window:    120 * time.Second,
		SoftLimit: 30,
		HardLimit: 100,
	}, "loose")
}

// RateLimitPublic 公开接口兜底防护，300s 内 5000 次软限 / 20000 次硬限
// 触发硬限后按 public 档独立计数应用递进式封禁（7天窗口），不与认证类接口互相影响
func RateLimitPublic(rdb *redis.Client) gin.HandlerFunc {
	return newIPRateLimiterPublic(rdb, RateLimitConfig{
		Window:    300 * time.Second,
		SoftLimit: 5000,
		HardLimit: 20000,
	})
}

func newIPRateLimiterPublic(rdb *redis.Client, cfg RateLimitConfig) gin.HandlerFunc {
	return newPrincipalRateLimiterPublic(
		rdb,
		cfg,
		func(c *gin.Context) string { return "ip:" + c.ClientIP() },
		func(principal string) string { return "ban:public:" + principal },
	)
}

func newPrincipalRateLimiterPublic(
	rdb *redis.Client,
	cfg RateLimitConfig,
	principalFn func(*gin.Context) string,
	banKeyFn func(string) string,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal := principalFn(c)
		applyPrincipalRateLimitPublic(c, rdb, cfg, principal, banKeyFn(principal))
	}
}

func applyPrincipalRateLimitPublic(c *gin.Context, rdb *redis.Client, cfg RateLimitConfig, principal string, banKey string) {
	ctx := context.Background()

	// 优先检查是否处于硬封禁状态
	banned, _ := rdb.Exists(ctx, banKey).Result()
	if banned > 0 {
		ttl, _ := rdb.TTL(ctx, banKey).Result()
		retryAfter := int(ttl.Seconds())
		msg := fmt.Sprintf("请求过于频繁，请在 %s 后重试", response.FormatRetryAfter(retryAfter))
		response.TooManyRequests(c, msg, retryAfter)
		c.Abort()
		return
	}

	// Pipeline 原子执行 Incr+Expire
	routeKey := fmt.Sprintf("ratelimit:%s:%s", c.FullPath(), principal)
	pipe := rdb.Pipeline()
	incrCmd := pipe.Incr(ctx, routeKey)
	pipe.Expire(ctx, routeKey, cfg.Window)
	pipe.Exec(ctx)
	count := incrCmd.Val()

	// 超过硬限：应用公开接口的递进式封禁
	if count > int64(cfg.HardLimit) {
		banCountKey := banKey + ":count"
		escalatedDuration := claimBanDuration(rdb, banKey, cfg.Window, func() time.Duration {
			return getEscalatedBanDurationPublic(rdb, banCountKey)
		})
		retryAfter := int(escalatedDuration.Seconds())
		msg := fmt.Sprintf("请求过于频繁，请在 %s 后重试", response.FormatRetryAfter(retryAfter))
		response.TooManyRequests(c, msg, retryAfter)
		c.Abort()
		return
	}

	// 超过软限：返回 429 但不封禁
	if count > int64(cfg.SoftLimit) {
		retryAfter := int(cfg.Window.Seconds())
		msg := fmt.Sprintf("请求过于频繁，请在 %s 后重试", response.FormatRetryAfter(retryAfter))
		response.TooManyRequests(c, msg, retryAfter)
		c.Abort()
		return
	}

	c.Next()
}

// claimBanDuration 在并发请求同时越过硬限时，只让第一个抢到 claim 标记的请求执行升级计数与落键封禁，
// 其余请求复用已落键的封禁时长，避免一次超限事件被并发请求重复计数导致封禁等级跳档
func claimBanDuration(rdb *redis.Client, banKey string, fallbackWindow time.Duration, escalate func() time.Duration) time.Duration {
	ctx := context.Background()
	claimed, _ := rdb.SetNX(ctx, banKey+":claim", 1, 2*time.Second).Result()
	if claimed {
		duration := escalate()
		rdb.Set(ctx, banKey, 1, duration)
		return duration
	}
	if ttl, err := rdb.TTL(ctx, banKey).Result(); err == nil && ttl > 0 {
		return ttl
	}
	return fallbackWindow
}

// RateLimitMomentUpload 按登录用户限制碎语保存频率，降低恶意批量上传图片的资源消耗。
func RateLimitMomentUpload(rdb *redis.Client) gin.HandlerFunc {
	return newPrincipalRateLimiter(rdb, RateLimitConfig{
		Window:    60 * time.Second,
		SoftLimit: 5,
		HardLimit: 20,
	}, momentUploadRateLimitPrincipal, momentUploadRateLimitBanKey)
}

// RateLimitTempUpload 按登录用户限制文章临时图片上传频率，管理员放宽阈值。
func RateLimitTempUpload(rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := RateLimitConfig{
			Window:    60 * time.Second,
			SoftLimit: 10,
			HardLimit: 40,
		}
		if detail := GetUserDetail(c); detail != nil && roles.HasPermission(detail.Roles, roles.AdminRole) {
			cfg = RateLimitConfig{
				Window:    60 * time.Second,
				SoftLimit: 60,
				HardLimit: 180,
			}
		}
		principal := tempUploadRateLimitPrincipal(c)
		applyPrincipalRateLimit(c, rdb, cfg, principal, tempUploadRateLimitBanKey(principal))
	}
}

// RateLimitAvatarUpload 按登录用户限制头像更换频率。
func RateLimitAvatarUpload(rdb *redis.Client) gin.HandlerFunc {
	return newPrincipalRateLimiter(rdb, RateLimitConfig{
		Window:    60 * time.Second,
		SoftLimit: 5,
		HardLimit: 20,
	}, avatarUploadRateLimitPrincipal, avatarUploadRateLimitBanKey)
}

func avatarUploadRateLimitPrincipal(c *gin.Context) string {
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

func avatarUploadRateLimitBanKey(principal string) string {
	return "ban:avatar-upload:" + principal
}

// newIPRateLimiter 按 IP 限流，tier 用于隔离不同档位的封禁 key，避免某个档位触发硬限后
// 连带封禁共用同一 IP 的其他档位接口（如 Strict 档封禁不应影响 Loose 档登录接口）
func newIPRateLimiter(rdb *redis.Client, cfg RateLimitConfig, tier string) gin.HandlerFunc {
	return newPrincipalRateLimiter(
		rdb,
		cfg,
		func(c *gin.Context) string { return "ip:" + c.ClientIP() },
		func(principal string) string { return "ban:" + tier + ":" + principal },
	)
}

func newPrincipalRateLimiter(
	rdb *redis.Client,
	cfg RateLimitConfig,
	principalFn func(*gin.Context) string,
	banKeyFn func(string) string,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		principal := principalFn(c)
		applyPrincipalRateLimit(c, rdb, cfg, principal, banKeyFn(principal))
	}
}

// getEscalatedBanDuration 根据 24 小时内的历史封禁次数，返回本次应用的递进式封禁时长（认证/资源接口）
// Incr 出错时退化为最低档时长，不参与计数升级，避免 Redis 抖动导致负索引 panic 或计数永不过期
func getEscalatedBanDuration(rdb *redis.Client, banCountKey string) time.Duration {
	count, err := incrWithExpire(rdb, banCountKey, banCountWindow)
	if err != nil {
		return escalatingBanDurations[0]
	}
	return escalatingBanDurations[clampIdx(count, len(escalatingBanDurations))]
}

// getEscalatedBanDurationPublic 根据 7 天内的历史封禁次数，返回本次应用的递进式封禁时长（公开接口）
// Incr 出错时退化为最低档时长，不参与计数升级，避免 Redis 抖动导致负索引 panic 或计数永不过期
func getEscalatedBanDurationPublic(rdb *redis.Client, banCountKey string) time.Duration {
	count, err := incrWithExpire(rdb, banCountKey, banCountWindowPublic)
	if err != nil {
		return escalatingBanDurationsPublic[0]
	}
	return escalatingBanDurationsPublic[clampIdx(count, len(escalatingBanDurationsPublic))]
}

// incrWithExpire 用 Pipeline 原子执行 Incr+Expire，避免 Incr 成功而 Expire 未执行导致计数 key 永不过期
func incrWithExpire(rdb *redis.Client, key string, window time.Duration) (int64, error) {
	ctx := context.Background()
	pipe := rdb.Pipeline()
	incrCmd := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, window)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, err
	}
	return incrCmd.Val(), nil
}

// clampIdx 把从 1 开始的次数换算成数组下标，并夹在 [0, size-1] 范围内
func clampIdx(count int64, size int) int {
	idx := int(count) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= size {
		idx = size - 1
	}
	return idx
}

func applyPrincipalRateLimit(c *gin.Context, rdb *redis.Client, cfg RateLimitConfig, principal string, banKey string) {
	ctx := context.Background()

	// 优先检查是否处于硬封禁状态，封禁期内直接拒绝，跳过后续计数操作
	banned, _ := rdb.Exists(ctx, banKey).Result()
	if banned > 0 {
		// 读取剩余封禁时间并写入 Retry-After header，告知客户端最早重试时机
		ttl, _ := rdb.TTL(ctx, banKey).Result()
		retryAfter := int(ttl.Seconds())
		msg := fmt.Sprintf("IP 已被封禁，请在 %s 后重试", response.FormatRetryAfter(retryAfter))
		response.TooManyRequests(c, msg, retryAfter)
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

	// 超过硬限：查询历史封禁次数，应用递进式封禁时长
	if count > int64(cfg.HardLimit) {
		banCountKey := banKey + ":count"
		escalatedDuration := claimBanDuration(rdb, banKey, cfg.Window, func() time.Duration {
			return getEscalatedBanDuration(rdb, banCountKey)
		})
		retryAfter := int(escalatedDuration.Seconds())
		msg := fmt.Sprintf("请求过于频繁，IP 已被临时封禁，请在 %s 后重试", response.FormatRetryAfter(retryAfter))
		response.TooManyRequests(c, msg, retryAfter)
		c.Abort()
		return
	}

	// 超过软限：返回 429 但不封禁，给客户端减速信号
	if count > int64(cfg.SoftLimit) {
		retryAfter := int(cfg.Window.Seconds())
		msg := fmt.Sprintf("请求过于频繁，请在 %s 后重试", response.FormatRetryAfter(retryAfter))
		response.TooManyRequests(c, msg, retryAfter)
		c.Abort()
		return
	}

	c.Next()
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

func tempUploadRateLimitPrincipal(c *gin.Context) string {
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

func tempUploadRateLimitBanKey(principal string) string {
	return "ban:temp-upload:" + principal
}
