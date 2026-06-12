package middleware_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/vpt/blog-backend/internal/middleware"
	jwtpkg "github.com/vpt/blog-backend/pkg/jwt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestLogger_WritesRequestContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, logs := observer.New(zapcore.InfoLevel)
	r := gin.New()
	r.Use(middleware.RequestID(), middleware.Logger(zap.New(core)))
	r.GET("/ping", func(c *gin.Context) {
		jwtpkg.SetClaims(c, &jwtpkg.Claims{UserId: 7})
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/ping?name=vpt", nil)
	req.Header.Set("X-Request-ID", "req-1")
	req.Header.Set("User-Agent", "test-agent")
	req.Header.Set("Referer", "https://example.com")
	r.ServeHTTP(w, req)

	entry := logs.All()[0]
	assert.Equal(t, zapcore.InfoLevel, entry.Level)
	assert.Equal(t, "请求", entry.Message)
	assert.Equal(t, "req-1", entry.ContextMap()["request_id"])
	assert.Equal(t, "/ping", entry.ContextMap()["path"])
	assert.Equal(t, "name=vpt", entry.ContextMap()["query"])
	assert.Equal(t, "test-agent", entry.ContextMap()["user_agent"])
	assert.Equal(t, "https://example.com", entry.ContextMap()["referer"])
	assert.Equal(t, int64(7), entry.ContextMap()["user_id"])
}

func TestLogger_UsesWarnForClientErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, logs := observer.New(zapcore.DebugLevel)
	r := gin.New()
	r.Use(middleware.RequestID(), middleware.Logger(zap.New(core)))
	r.GET("/missing", func(c *gin.Context) {
		c.Status(http.StatusNotFound)
	})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/missing", nil))

	assert.Equal(t, zapcore.WarnLevel, logs.All()[0].Level)
}

func TestLogger_UsesErrorForServerErrorsAndRecordsGinErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	core, logs := observer.New(zapcore.DebugLevel)
	r := gin.New()
	r.Use(middleware.RequestID(), middleware.Logger(zap.New(core)))
	r.GET("/boom", func(c *gin.Context) {
		_ = c.Error(errors.New("database unavailable"))
		c.Status(http.StatusInternalServerError)
	})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/boom", nil))

	entry := logs.All()[0]
	assert.Equal(t, zapcore.ErrorLevel, entry.Level)
	assert.Contains(t, entry.ContextMap()["errors"], "database unavailable")
}
