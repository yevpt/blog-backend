package middleware_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/middleware"
	"github.com/vpt/blog-backend/pkg/jwt"
)

func newJWTManager() *jwt.Manager {
	return jwt.NewManager("test-secret", 2, 168)
}

func TestAuth_MissingToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/", middleware.Auth(newJWTManager(), nil), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuth_ValidAccessToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := newJWTManager()
	token, _ := m.GenerateAccess(1)

	r := gin.New()
	r.GET("/", middleware.Auth(m, nil), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuth_RefreshTokenRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := newJWTManager()
	token, _ := m.GenerateRefresh(1)

	r := gin.New()
	r.GET("/", middleware.Auth(m, nil), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestOptionalAuth_AllowsAnonymous(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/", middleware.OptionalAuth(newJWTManager()), func(c *gin.Context) {
		assert.Nil(t, jwt.GetClaims(c))
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestOptionalAuth_AttachesClaimsWhenTokenPresent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := newJWTManager()
	token, _ := m.GenerateAccess(9)

	r := gin.New()
	r.GET("/", middleware.OptionalAuth(m), func(c *gin.Context) {
		claims := jwt.GetClaims(c)
		assert.NotNil(t, claims)
		assert.Equal(t, int64(9), claims.UserId)
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestOptionalAuth_RejectsBadTokenWhenPresent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/", middleware.OptionalAuth(newJWTManager()), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer bad.token")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestOptionalAuth_RejectsRefreshTokenWhenPresent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := newJWTManager()
	token, _ := m.GenerateRefresh(1)

	r := gin.New()
	r.GET("/", middleware.OptionalAuth(m), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

// --- userCache 集成测试 ---

// mockUserCache 是 service.UserCacheService 的测试 stub
type mockUserCache struct {
	profile *dto.UserDetailResp
	err     error
}

func (m *mockUserCache) Get(_ context.Context, _ int64) (*dto.UserDetailResp, error) {
	return m.profile, m.err
}
func (m *mockUserCache) Set(_ context.Context, _ int64, _ *dto.UserDetailResp) error {
	return nil
}
func (m *mockUserCache) Invalidate(_ context.Context, _ int64) error { return nil }

func TestAuth_WithCache_UserDetailInContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := newJWTManager()
	token, _ := m.GenerateAccess(1)

	profile := &dto.UserDetailResp{ID: 1, Username: "alice", Status: 1, Roles: []string{"ROLE_NORMAL"}}
	cache := &mockUserCache{profile: profile}

	r := gin.New()
	r.GET("/", middleware.Auth(m, cache), func(c *gin.Context) {
		detail := middleware.GetUserDetail(c)
		assert.NotNil(t, detail)
		assert.Equal(t, "alice", detail.Username)
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestAuth_WithCache_DisabledUser_Returns401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := newJWTManager()
	token, _ := m.GenerateAccess(2)

	// Status != 1 表示被禁用
	profile := &dto.UserDetailResp{ID: 2, Username: "banned", Status: 0}
	cache := &mockUserCache{profile: profile}

	r := gin.New()
	r.GET("/", middleware.Auth(m, cache), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestAuth_WithCache_CacheError_Returns401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := newJWTManager()
	token, _ := m.GenerateAccess(3)

	cache := &mockUserCache{err: errors.New("redis unavailable")}

	r := gin.New()
	r.GET("/", middleware.Auth(m, cache), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}
