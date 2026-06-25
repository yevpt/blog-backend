package analytics_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	hdl "github.com/vpt/blog-backend/internal/handler/analytics"
	svc "github.com/vpt/blog-backend/internal/service/analytics"
)

type captureSvc struct {
	got    svc.RawEvent
	called bool
}

func (c *captureSvc) Handle(_ context.Context, raw svc.RawEvent) error {
	c.got = raw
	c.called = true
	return nil
}

func TestCollectHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cs := &captureSvc{}
	h := hdl.NewCollectHandler(cs, []string{"https://www.yevpt.com"})

	r := gin.New()
	r.POST("/collect", func(c *gin.Context) { c.Set("visitor_id", "v1") }, h.Collect)

	body := `{"event_type":"page_view","path":"/a","session_id":"s1"}`
	req := httptest.NewRequest(http.MethodPost, "/collect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://www.yevpt.com")
	req.Header.Set("User-Agent", "Chrome")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	require.True(t, cs.called)
	assert.Equal(t, "v1", cs.got.VisitorID)
	assert.Equal(t, "page_view", cs.got.EventType)
	assert.True(t, cs.got.OriginAllowed)
}

func TestCollectHandlerBadOriginMarksSuspect(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cs := &captureSvc{}
	h := hdl.NewCollectHandler(cs, []string{"https://www.yevpt.com"})
	r := gin.New()
	r.POST("/collect", func(c *gin.Context) { c.Set("visitor_id", "v1") }, h.Collect)

	body := `{"event_type":"page_view","path":"/a","session_id":"s1"}`
	req := httptest.NewRequest(http.MethodPost, "/collect", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.com")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	require.True(t, cs.called)
	assert.Equal(t, "https://evil.com", cs.got.Origin)
	assert.False(t, cs.got.OriginAllowed)
}

func TestCollectHandlerBadBodyReturns204(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cs := &captureSvc{}
	h := hdl.NewCollectHandler(cs, nil)
	r := gin.New()
	r.POST("/collect", func(c *gin.Context) { c.Set("visitor_id", "v1") }, h.Collect)

	req := httptest.NewRequest(http.MethodPost, "/collect", strings.NewReader(`{bad`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.False(t, cs.called)
}
