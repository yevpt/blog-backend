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

// RateLimitNormal 普通敏感接口限流，60s 内 10 次软限 / 30 次硬限 / 封禁 15min
func RateLimitNormal(rdb *redis.Client) gin.HandlerFunc {
	return newIPRateLimiter(rdb, RateLimitConfig{
		Window:      60 * time.Second,
		SoftLimit:   10,
		HardLimit:   30,
		BanDuration: 15 * time.Minute,
	})
}

// RateLimitLoose 登录、OAuth 等认证接口，120s 内 30 次软限 / 100 次硬限 / 封禁 10min
func RateLimitLoose(rdb *redis.Client) gin.HandlerFunc {
	return newIPRateLimiter(rdb, RateLimitConfig{
		Window:      120 * time.Second,
		SoftLimit:   30,
		HardLimit:   100,
		BanDuration: 10 * time.Minute,
	})
}

// RateLimitPublic 公开接口兜底防护，300s 内 5000 次软限 / 20000 次硬限
// 触发硬限时应用递进式封禁（7天窗口）
func RateLimitPublic(rdb *redis.Client) gin.HandlerFunc {
	return newIPRateLimiterPublic(rdb, RateLimitConfig{
		Window:      300 * time.Second,
		SoftLimit:   5000,
		HardLimit:   20000,
		BanDuration: 5 * time.Minute,
	})
}

func newIPRateLimiterPublic(rdb *redis.Client, cfg RateLimitConfig) gin.HandlerFunc {
	return newPrincipalRateLimiterPublic(
		rdb,
		cfg,
		func(c *gin.Context) string { return "ip:" + c.ClientIP() },
		func(principal string) string { return "ban:" + principal },
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
		escalatedDuration := getEscalatedBanDurationPublic(rdb, banCountKey)
		rdb.Set(ctx, banKey, 1, escalatedDuration)
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

// RateLimitMomentUpload 按登录用户限制碎语保存频率，降低恶意批量上传图片的资源消耗。
func RateLimitMomentUpload(rdb *redis.Client) gin.HandlerFunc {
	return newPrincipalRateLimiter(rdb, RateLimitConfig{
		Window:      60 * time.Second,
		SoftLimit:   5,
		HardLimit:   20,
		BanDuration: 15 * time.Minute,
	}, momentUploadRateLimitPrincipal, momentUploadRateLimitBanKey)
}

// RateLimitTempUpload 按登录用户限制文章临时图片上传频率，管理员放宽阈值。
func RateLimitTempUpload(rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		cfg := RateLimitConfig{
			Window:      60 * time.Second,
			SoftLimit:   10,
			HardLimit:   40,
			BanDuration: 15 * time.Minute,
		}
		if detail := GetUserDetail(c); detail != nil && roles.HasPermission(detail.Roles, roles.AdminRole) {
			cfg = RateLimitConfig{
				Window:      60 * time.Second,
				SoftLimit:   60,
				HardLimit:   180,
				BanDuration: 15 * time.Minute,
			}
		}
		principal := tempUploadRateLimitPrincipal(c)
		applyPrincipalRateLimit(c, rdb, cfg, principal, tempUploadRateLimitBanKey(principal))
	}
}

// RateLimitAvatarUpload 按登录用户限制头像更换频率。
func RateLimitAvatarUpload(rdb *redis.Client) gin.HandlerFunc {
	return newPrincipalRateLimiter(rdb, RateLimitConfig{
		Window:      60 * time.Second,
		SoftLimit:   5,
		HardLimit:   20,
		BanDuration: 15 * time.Minute,
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
		principal := principalFn(c)
		applyPrincipalRateLimit(c, rdb, cfg, principal, banKeyFn(principal))
	}
}

// getEscalatedBanDuration 根据 24 小时内的历史封禁次数，返回本次应用的递进式封禁时长（认证/资源接口）
func getEscalatedBanDuration(rdb *redis.Client, banCountKey string) time.Duration {
	ctx := context.Background()
	count, _ := rdb.Incr(ctx, banCountKey).Result()
	rdb.Expire(ctx, banCountKey, banCountWindow)

	idx := int(count) - 1
	if idx >= len(escalatingBanDurations) {
		idx = len(escalatingBanDurations) - 1
	}
	return escalatingBanDurations[idx]
}

// getEscalatedBanDurationPublic 根据 7 天内的历史封禁次数，返回本次应用的递进式封禁时长（公开接口）
func getEscalatedBanDurationPublic(rdb *redis.Client, banCountKey string) time.Duration {
	ctx := context.Background()
	count, _ := rdb.Incr(ctx, banCountKey).Result()
	rdb.Expire(ctx, banCountKey, banCountWindowPublic)

	idx := int(count) - 1
	if idx >= len(escalatingBanDurationsPublic) {
		idx = len(escalatingBanDurationsPublic) - 1
	}
	return escalatingBanDurationsPublic[idx]
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
		escalatedDuration := getEscalatedBanDuration(rdb, banCountKey)
		rdb.Set(ctx, banKey, 1, escalatedDuration)
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
