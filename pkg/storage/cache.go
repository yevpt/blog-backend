package storage

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	objectURLCachePrefixCDN    = "cdn"
	objectURLCachePrefixGarage = "garage"
)

// cachedObjectURLResolverImpl 保存对象访问 URL 缓存所需的依赖和策略。
type cachedObjectURLResolverImpl struct {
	client *Client       // 底层对象存储客户端
	rdb    *redis.Client // Redis 客户端
	ttl    time.Duration // 缓存有效期
}

// newCachedObjectURLResolver 创建对象 URL 缓存解析器。
func newCachedObjectURLResolver(client *Client, rdb *redis.Client, ttl time.Duration) *CachedObjectURLResolver {
	// 构造轻量包装器，真正的空值和降级处理放在调用路径中统一完成。
	return &CachedObjectURLResolver{impl: &cachedObjectURLResolverImpl{
		client: client,
		rdb:    rdb,
		ttl:    ttl,
	}}
}

// objectURL 优先从 Redis 读取稳定 URL，未命中时再调用底层客户端生成。
func (r *CachedObjectURLResolver) objectURL(ctx context.Context, objectName string) (string, error) {
	// 先确认底层客户端存在，否则无法生成任何对象访问 URL。
	if r == nil || r.impl == nil || r.impl.client == nil {
		return "", errors.New("对象访问 URL 缓存解析器未初始化")
	}

	// 先清理对象 key，保证缓存 key 和底层签名都使用同一种对象名。
	objectName = normalizeObjectName(objectName)
	if objectName == "" {
		return "", nil
	}

	// 缓存不可用时直接走底层客户端，保持对象 URL 解析能力可用。
	if r.cacheDisabled() {
		return r.impl.client.ObjectURL(ctx, objectName)
	}

	// 先查 Redis，命中后直接返回，避免重复生成带新时间戳的 URL。
	cacheKey := r.cacheKey(objectName)
	cachedURL, err := r.impl.rdb.Get(ctx, cacheKey).Result()
	if err == nil {
		return cachedURL, nil
	}

	// Redis 未命中以外的读取错误不阻断访问，继续生成新 URL。
	if err != nil && !errors.Is(err, redis.Nil) {
		return r.impl.client.ObjectURL(ctx, objectName)
	}

	// 未命中时生成真实访问 URL，错误继续向上返回给调用方。
	objectURL, err := r.impl.client.ObjectURL(ctx, objectName)
	if err != nil {
		return "", err
	}

	// 空 URL 没有缓存价值，直接返回。
	if objectURL == "" {
		return "", nil
	}

	// 写缓存失败不影响本次返回，下一次请求仍可重新生成。
	_ = r.impl.rdb.Set(ctx, cacheKey, objectURL, r.impl.ttl).Err()

	return objectURL, nil
}

// cacheDisabled 判断缓存依赖是否足够工作。
func (r *CachedObjectURLResolver) cacheDisabled() bool {
	// 包装器、底层客户端、Redis 或 TTL 缺失时都直接降级到底层解析。
	return r.impl.rdb == nil || r.impl.ttl <= 0
}

// cacheKey 根据当前 URL 生成策略拼出 Redis key。
func (r *CachedObjectURLResolver) cacheKey(objectName string) string {
	// 开启 CDN 时使用 cdn 前缀，否则使用 garage 前缀，便于两种 URL 策略互不污染。
	if r.impl.client.impl != nil && r.impl.client.impl.useCDN {
		return objectURLCachePrefixCDN + ":" + objectName
	}

	return objectURLCachePrefixGarage + ":" + objectName
}
