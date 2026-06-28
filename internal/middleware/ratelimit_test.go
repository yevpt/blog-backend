package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/vpt/blog-backend/internal/dto"
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
	banKey := "ban:strict:ip:10.0.0.3"
	exists, err := rdb.Exists(context.Background(), banKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, int64(1), exists, "封禁 key 应已写入 Redis")
}

func TestRateLimitStrict_BannedIPBlocked(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rdb, mr := newTestRedis(t)
	defer mr.Close()

	// 预先写入封禁 key（TTL=0 表示永不过期，仅测试用）
	rdb.Set(context.Background(), "ban:strict:ip:10.0.0.4", 1, 0)

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

func TestRateLimitStrictBan_DoesNotBlockPublicTier(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rdb, mr := newTestRedis(t)
	defer mr.Close()

	r := gin.New()
	r.GET("/auth/send-code", middleware.RateLimitStrict(rdb), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	r.GET("/categories", middleware.RateLimitPublic(rdb), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// 打满 /auth/send-code 触发 Strict 档硬限封禁
	for i := 0; i < 21; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/auth/send-code", nil)
		req.RemoteAddr = "10.0.0.50:9999"
		r.ServeHTTP(w, req)
	}

	// 同一 IP 访问公开接口不应被连带封禁（两档应各自独立计数与封禁）
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/categories", nil)
	req.RemoteAddr = "10.0.0.50:9999"
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code, "Strict 档封禁不应连带封禁 Public 档")
}

func TestRateLimitStrict_ConcurrentHardLimitBreachOnlyEscalatesOnce(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rdb, mr := newTestRedis(t)
	defer mr.Close()

	r := gin.New()
	r.GET("/auth/send-code", middleware.RateLimitStrict(rdb), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	// 模拟并发：多个请求几乎同时把 routeKey 计数推过硬限（20），其中 5 个请求的计数会越过硬限
	var wg sync.WaitGroup
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			req := httptest.NewRequest("GET", "/auth/send-code", nil)
			req.RemoteAddr = "10.0.0.60:9999"
			r.ServeHTTP(w, req)
		}()
	}
	wg.Wait()

	banCountKey := "ban:strict:ip:10.0.0.60:count"
	count, err := rdb.Get(context.Background(), banCountKey).Result()
	assert.NoError(t, err)
	assert.Equal(t, "1", count, "一次并发超限事件只应记一次升级计数，不应被并发请求重复计数导致跳档")
}

func TestRateLimitMomentUpload_BlocksByUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rdb, mr := newTestRedis(t)
	defer mr.Close()

	r := gin.New()
	r.POST("/moments", func(c *gin.Context) {
		middleware.SetUserDetail(c, &dto.UserDetailResp{ID: 7, Status: 1})
	}, middleware.RateLimitMomentUpload(rdb), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/moments", nil)
		req.RemoteAddr = "10.0.0.8:9999"
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/moments", nil)
	req.RemoteAddr = "10.0.0.9:9999"
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestRateLimitModerationWriteUsesConfiguredPerUserLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rdb, mr := newTestRedis(t)
	defer mr.Close()
	router := gin.New()
	router.POST("/comments", func(c *gin.Context) {
		middleware.SetUserDetail(c, &dto.UserDetailResp{ID: 7})
		c.Next()
	}, middleware.RateLimitModerationWrite(rdb, 2, "publish"), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	for i, want := range []int{http.StatusOK, http.StatusOK, http.StatusTooManyRequests} {
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest("POST", "/comments", nil))
		assert.Equal(t, want, recorder.Code, "request %d", i+1)
	}
}

func TestRateLimitTempUpload_NormalUserBlockedAfterSoftLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rdb, mr := newTestRedis(t)
	defer mr.Close()

	r := gin.New()
	r.POST("/uploads/temp", func(c *gin.Context) {
		middleware.SetUserDetail(c, &dto.UserDetailResp{ID: 18, Status: 1, Roles: []string{}})
	}, middleware.RateLimitTempUpload(rdb), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	for i := 0; i < 10; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/uploads/temp", nil)
		req.RemoteAddr = "10.0.0.18:9999"
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/uploads/temp", nil)
	req.RemoteAddr = "10.0.0.18:9999"
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusTooManyRequests, w.Code)
}

func TestRateLimitTempUpload_AdminStillAllowedAtEleventhRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rdb, mr := newTestRedis(t)
	defer mr.Close()

	r := gin.New()
	r.POST("/uploads/temp", func(c *gin.Context) {
		middleware.SetUserDetail(c, &dto.UserDetailResp{ID: 21, Status: 1, Roles: []string{"ROLE_ADMIN"}})
	}, middleware.RateLimitTempUpload(rdb), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	for i := 0; i < 11; i++ {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/uploads/temp", nil)
		req.RemoteAddr = "10.0.0.21:9999"
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code, "第 %d 次请求管理员应通过", i+1)
	}
}
