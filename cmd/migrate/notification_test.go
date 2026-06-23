package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/internal/model"
)

// 旧类型按设计映射规范化，未知类型回退 legacy_notice。
func TestLegacyEventType_Mapping(t *testing.T) {
	cases := map[string]string{
		"post_like":       "article_liked",
		"article_like":    "article_liked",
		"say_like":        "moment_liked",
		"moment_like":     "moment_liked",
		"comment":         "comment_created",
		"moment_comment":  "comment_created",
		"say":             "comment_created",
		"comment_reply":   "reply_created",
		"guestBook":       "guestbook_created",
		"guestBook_reply": "reply_created",
		"weird_unknown":   "legacy_notice",
		"":                "legacy_notice",
	}
	for old, want := range cases {
		assert.Equalf(t, want, legacyEventType(old), "type %q", old)
	}
}

// 旧 message 映射为事件：类型规范化、原始信息保留到 metadata、文章作为根对象。
func TestBuildLegacyEvent_MapsAndPreservesMetadata(t *testing.T) {
	title := "文章点赞"
	content := "有人点赞"
	articleID := uint(3)
	commentID := uint(99)
	msg := model.Message{
		Base:       model.Base{ID: 7},
		Title:      &title,
		Content:    &content,
		Type:       "post_like",
		TypeID:     50,
		FromUserID: 2,
		ArticleID:  &articleID,
		CommentID:  &commentID,
	}

	event, err := buildLegacyEvent(msg)

	require.NoError(t, err)
	assert.Equal(t, "article_liked", event.Type)
	require.NotNil(t, event.ActorUserID)
	assert.Equal(t, uint(2), *event.ActorUserID)
	assert.Equal(t, "article", event.RootType)
	assert.Equal(t, uint(3), event.RootID)
	assert.Equal(t, "文章点赞", event.Title)
	assert.Equal(t, "done", event.DispatchStatus)
	require.NotNil(t, event.MetadataJSON)
	// 旧字段可追溯。
	assert.True(t, strings.Contains(*event.MetadataJSON, "legacy_message_id"))
	assert.True(t, strings.Contains(*event.MetadataJSON, "post_like"))
	assert.True(t, strings.Contains(*event.MetadataJSON, "legacy_comment_id"))
}

// 无文章关联的旧消息回退到 legacy 根对象。
func TestBuildLegacyEvent_FallsBackToLegacyRoot(t *testing.T) {
	msg := model.Message{Base: model.Base{ID: 8}, Type: "guestBook", TypeID: 12, FromUserID: 4}

	event, err := buildLegacyEvent(msg)

	require.NoError(t, err)
	assert.Equal(t, "guestbook_created", event.Type)
	assert.Equal(t, "legacy", event.RootType)
	assert.Equal(t, uint(12), event.RootID)
}

// 已读的旧 user_message 映射为已读收件箱并带已读时间。
func TestBuildLegacyInbox_MapsReadState(t *testing.T) {
	um := model.UserMessage{Base: model.Base{ID: 5}, UserID: 9, MessageID: 7, IsRead: true}

	inbox := buildLegacyInbox(um, 100)

	assert.Equal(t, uint(100), inbox.EventID)
	assert.Equal(t, uint(9), inbox.RecipientUserID)
	assert.True(t, inbox.IsRead)
	assert.NotNil(t, inbox.ReadAt)
}
