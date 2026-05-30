package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/vpt/blog-backend/internal/middleware"
)

func newTestRedis(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	return rdb, mr
}

func TestRateLimitStrict_AllowsUnderLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rdb, mr := newTestRedis(t)
	defer mr.Close()

	r := gin.New()
	r.GET("/auth/send-code", middleware.RateLimitStrict(rdb), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/auth/send-code", nil)
		req.RemoteAddr = "10.0.0.1:9999"
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, "第 %d 次请求应通过", i+1)
	}
}

func TestRateLimitStrict_BlocksAtSoftLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rdb, mr := newTestRedis(t)
	defer mr.Close()

	r := gin.New()
	r.GET("/auth/send-code", middleware.RateLimitStrict(rdb), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/auth/send-code", nil)
		req.RemoteAddr = "10.0.0.2:9999"
		r.ServeHTTP(w, req)
	}

	// 第 6 次应触发 429
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/auth/send-code", nil)
	req.RemoteAddr = "10.0.0.2:9999"
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
	assert.NotEmpty(t, w.Header().Get("Retry-After"))
}

func TestRateLimitStrict_BansAtHardLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rdb, mr := newTestRedis(t)
	defer mr.Close()

	r := gin.New()
	r.GET("/auth/send-code", middleware.RateLimitStrict(rdb), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// 打满到超过 hardLimit=20
	for i := 0; i < 21; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/auth/send-code", nil)
		req.RemoteAddr = "10.0.0.3:9999"
		r.ServeHTTP(w, req)
	}

	// 验证封禁 key 存在
	banKey := "ban:ip:10.0.0.3"
	exists, err := rdb.Exists(context.Background(), banKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, int64(1), exists, "封禁 key 应已写入 Redis")
}

func TestRateLimitStrict_BannedIPBlocked(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rdb, mr := newTestRedis(t)
	defer mr.Close()

	// 预先写入封禁 key（TTL=0 表示永不过期，仅测试用）
	rdb.Set(context.Background(), "ban:ip:10.0.0.4", 1, 0)

	r := gin.New()
	r.GET("/auth/send-code", middleware.RateLimitStrict(rdb), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/auth/send-code", nil)
	req.RemoteAddr = "10.0.0.4:9999"
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}
