package notification

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/vpt/blog-backend/pkg/response"
)

// sseHeartbeat SSE 心跳间隔，定期发送注释行保持连接与探测断线。
const sseHeartbeat = 25 * time.Second

// Stream 建立站内通知 SSE 长连接。
// @Summary 站内通知实时推送（SSE）
// @Description 登录用户建立 SSE 长连接，dispatcher 写入收件箱后推送事件类型；断线后由列表接口补齐历史。
// @Tags 通知
// @Produce text/event-stream
// @Success 200 {string} string "SSE 事件流；每条事件 data 为事件类型"
// @Failure 401 {object} response.Response "未登录或 token 已过期"
// @Failure 500 {object} response.Response "服务器内部错误"
// @Router /notifications/stream [get]
func (h *NotificationHandler) Stream(c *gin.Context) {
	userID, ok := requiredUserID(c)
	if !ok {
		return
	}
	if h.hub == nil {
		response.ServerError(c)
		return
	}

	// 设置 SSE 响应头，禁用缓冲与缓存。
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("X-Accel-Buffering", "no")

	// 注册在线连接，函数退出时注销并关闭通道。
	sub := h.hub.Subscribe(userID)
	defer h.hub.Unsubscribe(userID, sub)

	// 先发一条注释行，让客户端尽快确认连接建立。
	c.Writer.Write([]byte(": connected\n\n"))
	c.Writer.Flush()

	heartbeat := time.NewTicker(sseHeartbeat)
	defer heartbeat.Stop()

	ctx := c.Request.Context()
	for {
		select {
		case <-ctx.Done():
			// 客户端断开或请求取消，结束连接。
			return
		case eventType, alive := <-sub.Events:
			if !alive {
				return
			}
			fmt.Fprintf(c.Writer, "event: notification\ndata: %s\n\n", eventType)
			c.Writer.Flush()
		case <-heartbeat.C:
			c.Writer.Write([]byte(": ping\n\n"))
			c.Writer.Flush()
		}
	}
}
