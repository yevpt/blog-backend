package notification_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	notificationservice "github.com/vpt/blog-backend/internal/service/notification"
)

// hub 注册与注销连接：注销后通道关闭。
func TestSSEHub_RegisterAndUnregister(t *testing.T) {
	hub := notificationservice.NewSSEHub()

	sub := hub.Subscribe(7)
	require.NotNil(t, sub)

	hub.Unsubscribe(7, sub)

	// 注销后通道应被关闭。
	_, alive := <-sub.Events
	assert.False(t, alive)
}

// 向某用户推送不会发给其他用户。
func TestSSEHub_PublishOnlyToTargetUser(t *testing.T) {
	hub := notificationservice.NewSSEHub()
	subA := hub.Subscribe(7)
	subB := hub.Subscribe(8)
	defer hub.Unsubscribe(7, subA)
	defer hub.Unsubscribe(8, subB)

	hub.NotifyInbox(context.Background(), 7, "comment_created")

	// 用户 7 收到。
	select {
	case msg := <-subA.Events:
		assert.Equal(t, "comment_created", msg)
	case <-time.After(time.Second):
		t.Fatal("用户 7 未收到推送")
	}

	// 用户 8 不应收到。
	select {
	case <-subB.Events:
		t.Fatal("用户 8 不应收到其他用户的推送")
	case <-time.After(50 * time.Millisecond):
	}
}
