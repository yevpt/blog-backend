package main

import (
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	msg := legacyMessage{
		ID:         7,
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
	msg := legacyMessage{ID: 8, Type: "guestBook", TypeID: 12, FromUserID: 4}

	event, err := buildLegacyEvent(msg)

	require.NoError(t, err)
	assert.Equal(t, "guestbook_created", event.Type)
	assert.Equal(t, "legacy", event.RootType)
	assert.Equal(t, uint(12), event.RootID)
}

// 已读的旧 user_messages 映射为已读收件箱并带已读时间。
func TestBuildLegacyInbox_MapsReadState(t *testing.T) {
	createdAt := time.Now()
	um := legacyUserMessage{ID: 5, UserID: 9, MessageID: 7, IsRead: true, CreatedAt: createdAt, UpdatedAt: createdAt}

	inbox := buildLegacyInbox(um, 100)

	assert.Equal(t, uint(100), inbox.EventID)
	assert.Equal(t, uint(9), inbox.RecipientUserID)
	assert.True(t, inbox.IsRead)
	assert.NotNil(t, inbox.ReadAt)
}

func TestListLegacyMessages_ReadsSourceMessageTable(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	createdAt := time.Now()
	mock.ExpectQuery("SELECT ID, title, content, type, type_id, from_id, post_id, comment_id, date_create\\s+FROM message ORDER BY ID").
		WillReturnRows(sqlmock.NewRows([]string{"ID", "title", "content", "type", "type_id", "from_id", "post_id", "comment_id", "date_create"}).
			AddRow(uint(7), "文章点赞", "有人点赞", "post_like", uint(50), uint(2), int64(3), int64(99), createdAt))

	messages, err := listLegacyMessages(db)

	require.NoError(t, err)
	require.Len(t, messages, 1)
	assert.Equal(t, uint(7), messages[0].ID)
	assert.Equal(t, "post_like", messages[0].Type)
	require.NotNil(t, messages[0].ArticleID)
	assert.Equal(t, uint(3), *messages[0].ArticleID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListLegacyUserMessages_FiltersByExistingSourceMessage(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	createdAt := time.Now()
	mock.ExpectQuery("FROM user_messages um\\s+JOIN message m ON m.ID = um.message_id").
		WillReturnRows(sqlmock.NewRows([]string{"ID", "user_id", "message_id", "read_status", "date_create"}).
			AddRow(uint(5), uint(9), uint(7), "01", createdAt))

	userMessages, err := listLegacyUserMessages(db)

	require.NoError(t, err)
	require.Len(t, userMessages, 1)
	assert.Equal(t, uint(9), userMessages[0].UserID)
	assert.Equal(t, uint(7), userMessages[0].MessageID)
	assert.True(t, userMessages[0].IsRead)
	require.NoError(t, mock.ExpectationsWereMet())
}
