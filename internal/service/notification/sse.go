package notification

import (
	"context"
	"sync"
)

// sseChannelBuffer 单个订阅者的缓冲长度；满时丢弃消息，SSE 只做在线提示不保证可靠。
const sseChannelBuffer = 16

// SSESubscriber 一个在线连接的订阅者，Events 输出待推送的消息体。
type SSESubscriber struct {
	Events chan string
}

// SSEHub 单实例内存 SSE 广播中心，按 user_id 维护在线连接。
// 多实例下的一致性后续可接 Redis Pub/Sub，此处先落单实例。
type SSEHub struct {
	mu   sync.RWMutex
	subs map[uint]map[*SSESubscriber]struct{}
}

// NewSSEHub 创建内存 SSE 广播中心。
func NewSSEHub() *SSEHub {
	return &SSEHub{subs: make(map[uint]map[*SSESubscriber]struct{})}
}

// Subscribe 为某用户注册一个在线连接并返回订阅者。
func (h *SSEHub) Subscribe(userID uint) *SSESubscriber {
	sub := &SSESubscriber{Events: make(chan string, sseChannelBuffer)}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.subs[userID] == nil {
		h.subs[userID] = make(map[*SSESubscriber]struct{})
	}
	h.subs[userID][sub] = struct{}{}
	return sub
}

// Unsubscribe 移除某用户的指定连接并关闭其通道。
func (h *SSEHub) Unsubscribe(userID uint, sub *SSESubscriber) {
	h.mu.Lock()
	defer h.mu.Unlock()
	conns, ok := h.subs[userID]
	if !ok {
		return
	}
	if _, exists := conns[sub]; exists {
		delete(conns, sub)
		close(sub.Events)
	}
	if len(conns) == 0 {
		delete(h.subs, userID)
	}
}

// NotifyInbox 在收件箱写入后向该用户的在线连接推送事件类型。
// 实现 dispatcher 的 InboxNotifier 接口；通道满时丢弃，断线由列表接口补齐。
func (h *SSEHub) NotifyInbox(_ context.Context, userID uint, eventType string) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for sub := range h.subs[userID] {
		select {
		case sub.Events <- eventType:
		default:
			// 缓冲已满，丢弃本次在线提示，不阻塞分发。
		}
	}
}
