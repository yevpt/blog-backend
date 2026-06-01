package storage

import (
	"context"
	"net/url"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/pkg/config"
)

func newStorageTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	require.NoError(t, err)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() {
		rdb.Close()
		mr.Close()
	})
	return rdb, mr
}

// TestCachedObjectURLResolver_UsesCDNKeyAndCachesURL 验证 CDN 模式按原始对象 key 缓存签名 URL。
func TestCachedObjectURLResolver_UsesCDNKeyAndCachesURL(t *testing.T) {
	rdb, _ := newStorageTestRedis(t)
	signer, err := NewCDNSigner(&config.CDNConfig{
		Host:               "https://blog-oss.example.com",
		Secret:             "secret",
		SignQueryName:      "a",
		TimestampQueryName: "b",
	})
	require.NoError(t, err)
	current := time.Unix(1_700_000_000, 0)
	signer.impl.now = func() time.Time {
		current = current.Add(time.Second)
		return current
	}
	client := &Client{impl: &clientImpl{bucket: "blog", useCDN: true, cdnSigner: signer}}
	resolver := NewCachedObjectURLResolver(client, rdb, 6*24*time.Hour)

	objectName := "post/bg-images/202106/245eb60be3b9dadf181b6e98ae7482f6.jpg"
	firstURL, err := resolver.ObjectURL(context.Background(), objectName)
	require.NoError(t, err)
	secondURL, err := resolver.ObjectURL(context.Background(), "/"+objectName)
	require.NoError(t, err)

	assert.Equal(t, firstURL, secondURL)
	cachedURL, err := rdb.Get(context.Background(), "cdn:"+objectName).Result()
	require.NoError(t, err)
	assert.Equal(t, firstURL, cachedURL)
	assert.Positive(t, rdb.TTL(context.Background(), "cdn:"+objectName).Val())
}

// TestNewCachedGarage_ReturnsCachedResolver 验证对外构造函数会同时完成 Garage 初始化和 Redis 缓存包装。
func TestNewCachedGarage_ReturnsCachedResolver(t *testing.T) {
	rdb, _ := newStorageTestRedis(t)

	resolver, err := NewCachedGarage(&config.GarageConfig{
		Endpoint:        "https://garage.example.com",
		Bucket:          "blog",
		Region:          "garage",
		AccessKeyID:     "access-key",
		SecretAccessKey: "secret-key",
		CDN:             false,
	}, &config.CDNConfig{}, rdb)

	require.NoError(t, err)
	require.NotNil(t, resolver)
	require.NotNil(t, resolver.impl.client)
	assert.Equal(t, rdb, resolver.impl.rdb)
	assert.Equal(t, DefaultObjectURLCacheTTL, resolver.impl.ttl)
}

// TestCachedObjectURLResolver_UsesGarageKeyWhenCDNDisabled 验证 Garage 模式使用 garage 前缀缓存预签名 URL。
func TestCachedObjectURLResolver_UsesGarageKeyWhenCDNDisabled(t *testing.T) {
	rdb, _ := newStorageTestRedis(t)
	presigner := &fakePresigner{url: "https://garage.example.com/blog/images/cat.jpg?X-Amz-Signature=abc"}
	client := &Client{
		impl: &clientImpl{
			bucket:         "blog",
			presigner:      presigner,
			presignExpires: 15 * time.Minute,
		},
	}
	resolver := NewCachedObjectURLResolver(client, rdb, time.Hour)

	objectURL, err := resolver.ObjectURL(context.Background(), "/images/cat.jpg")
	require.NoError(t, err)

	cachedURL, err := rdb.Get(context.Background(), "garage:images/cat.jpg").Result()
	require.NoError(t, err)
	assert.Equal(t, objectURL, cachedURL)
	assert.Equal(t, 1, presigner.calls)
}

// TestCachedObjectURLResolver_ReturnsCachedURLBeforeSigning 验证 Redis 命中时不会重新请求底层签名。
func TestCachedObjectURLResolver_ReturnsCachedURLBeforeSigning(t *testing.T) {
	rdb, _ := newStorageTestRedis(t)
	presigner := &fakePresigner{url: "https://garage.example.com/blog/images/cat.jpg?X-Amz-Signature=new"}
	client := &Client{
		impl: &clientImpl{
			bucket:         "blog",
			presigner:      presigner,
			presignExpires: 15 * time.Minute,
		},
	}
	resolver := NewCachedObjectURLResolver(client, rdb, time.Hour)
	err := rdb.Set(context.Background(), "garage:images/cat.jpg", "https://cached.example.com/cat.jpg", time.Hour).Err()
	require.NoError(t, err)

	objectURL, err := resolver.ObjectURL(context.Background(), "images/cat.jpg")
	require.NoError(t, err)

	assert.Equal(t, "https://cached.example.com/cat.jpg", objectURL)
	assert.Zero(t, presigner.calls)
}

// TestCachedObjectURLResolver_BypassesCacheForEmptyObjectName 验证空对象名不会产生缓存 key。
func TestCachedObjectURLResolver_BypassesCacheForEmptyObjectName(t *testing.T) {
	rdb, _ := newStorageTestRedis(t)
	client := &Client{impl: &clientImpl{bucket: "blog"}}
	resolver := NewCachedObjectURLResolver(client, rdb, time.Hour)

	objectURL, err := resolver.ObjectURL(context.Background(), " ")
	require.NoError(t, err)

	assert.Empty(t, objectURL)
	keys, err := rdb.Keys(context.Background(), "*").Result()
	require.NoError(t, err)
	assert.Empty(t, keys)
}

// TestCachedObjectURLResolver_KeepsCachedURLStable 验证缓存 URL 保持可解析且不会因重新签名而变化。
func TestCachedObjectURLResolver_KeepsCachedURLStable(t *testing.T) {
	rdb, _ := newStorageTestRedis(t)
	signer, err := NewCDNSigner(&config.CDNConfig{
		Host:               "https://blog-oss.example.com",
		Secret:             "secret",
		SignQueryName:      "a",
		TimestampQueryName: "b",
	})
	require.NoError(t, err)
	signer.impl.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	client := &Client{impl: &clientImpl{bucket: "blog", useCDN: true, cdnSigner: signer}}
	resolver := NewCachedObjectURLResolver(client, rdb, time.Hour)

	objectURL, err := resolver.ObjectURL(context.Background(), "images/cat.jpg")
	require.NoError(t, err)

	parsed, err := url.Parse(objectURL)
	require.NoError(t, err)
	assert.Equal(t, "/blog/images/cat.jpg", parsed.Path)
	assert.NotEmpty(t, parsed.Query().Get("a"))
	assert.NotEmpty(t, parsed.Query().Get("b"))
}
