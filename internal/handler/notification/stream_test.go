package notification_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/vpt/blog-backend/internal/dto"
	notificationhandler "github.com/vpt/blog-backend/internal/handler/notification"
	"github.com/vpt/blog-backend/internal/middleware"
	notificationservice "github.com/vpt/blog-backend/internal/service/notification"
)

func newStreamRouter(hub *notificationservice.SSEHub) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := notificationhandler.NewNotificationHandler(&stubInboxService{}, hub)
	r.GET("/notifications/stream", func(c *gin.Context) {
		middleware.SetUserDetail(c, &dto.UserDetailResp{ID: 7, Username: "vpt", Status: 1})
		h.Stream(c)
	})
	return r
}

// Stream 应设置正确的 SSE 响应头并在 context 取消后退出。
func TestStreamHandler_SetsHeadersAndExitsOnCancel(t *testing.T) {
	hub := notificationservice.NewSSEHub()
	r := newStreamRouter(hub)

	w := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/notifications/stream", nil).WithContext(ctx)

	done := make(chan struct{})
	go func() {
		r.ServeHTTP(w, req)
		close(done)
	}()

	// 等待连接建立后取消，避免与 handler 并发读 recorder。
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Stream 未在 context 取消后退出")
	}

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	assert.Equal(t, "no-cache", w.Header().Get("Cache-Control"))
	assert.Empty(t, w.Header().Get("Connection"))
}

// hub 为 nil（未启用 SSE）时返回 500。
func TestStreamHandler_NoHubReturnsServerError(t *testing.T) {
	r := newStreamRouter(nil)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/notifications/stream", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
