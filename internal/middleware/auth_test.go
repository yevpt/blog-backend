package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
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
