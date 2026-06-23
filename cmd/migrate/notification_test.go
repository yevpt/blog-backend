package main

import (
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/model"
)

// 旧类型按设计映射规范化，未知类型回退 legacy_notice。
func TestLegacyEventType_Mapping(t *testing.T) {
	cases := map[string]string{
		"post_like":            "article_liked",
		"article_like":         "article_liked",
		"say_like":             "moment_liked",
		"moment_like":          "moment_liked",
		"comment":              "comment_created",
		"moment_comment":       "comment_created",
		"say":                  "comment_created",
		"comment_like":         "comment_liked",
		"comment_reply":        "reply_created",
		"comment_reply_like":   "reply_liked",
		"guestBook":            "guestbook_created",
		"guestBook_like":       "guestbook_liked",
		"guestBook_reply":      "reply_created",
		"guestBook_reply_like": "reply_liked",
		"system":               "system_notice",
		"weird_unknown":        "legacy_notice",
		"":                     "legacy_notice",
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

	event, err := buildLegacyEvent(msg, legacyNotificationRefs{}, nil)

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

func TestBuildLegacyEvent_MapsLegacySemanticRefs(t *testing.T) {
	refs := legacyNotificationRefs{
		comments: map[uint]legacyCommentRef{
			10: {sourceType: "comment", sourceID: 10, rootType: "article", rootID: 3},
			11: {sourceType: "comment", sourceID: 11, rootType: "moment", rootID: 4},
			12: {sourceType: "guestbook", sourceID: 12, rootType: "guestbook", rootID: 12},
		},
		replies: map[uint]legacyReplyRef{
			20: {sourceType: "reply", sourceID: 20, rootType: "article", rootID: 10},
			21: {sourceType: "reply", sourceID: 21, rootType: "moment", rootID: 11},
			22: {sourceType: "reply", sourceID: 22, rootType: "guestbook", rootID: 12},
		},
	}

	cases := []struct {
		name       string
		msg        legacyMessage
		sourceType string
		sourceID   uint
		rootType   string
		rootID     uint
	}{
		{
			name:       "碎语点赞",
			msg:        legacyMessage{ID: 8, Type: "say_like", TypeID: 30, FromUserID: 4},
			sourceType: "moment",
			sourceID:   30,
			rootType:   "moment",
			rootID:     30,
		},
		{
			name:       "文章评论",
			msg:        legacyMessage{ID: 9, Type: "comment", TypeID: 10, FromUserID: 4},
			sourceType: "comment",
			sourceID:   10,
			rootType:   "article",
			rootID:     3,
		},
		{
			name:       "碎语评论",
			msg:        legacyMessage{ID: 10, Type: "moment_comment", TypeID: 11, FromUserID: 4},
			sourceType: "comment",
			sourceID:   11,
			rootType:   "moment",
			rootID:     4,
		},
		{
			name:       "留言板新留言",
			msg:        legacyMessage{ID: 11, Type: "guestBook", TypeID: 12, FromUserID: 4},
			sourceType: "guestbook",
			sourceID:   12,
			rootType:   "guestbook",
			rootID:     12,
		},
		{
			name:       "文章回复",
			msg:        legacyMessage{ID: 12, Type: "comment_reply", TypeID: 20, FromUserID: 4},
			sourceType: "reply",
			sourceID:   20,
			rootType:   "article",
			rootID:     10,
		},
		{
			name:       "碎语回复",
			msg:        legacyMessage{ID: 13, Type: "comment_reply", TypeID: 21, FromUserID: 4},
			sourceType: "reply",
			sourceID:   21,
			rootType:   "moment",
			rootID:     11,
		},
		{
			name:       "留言回复",
			msg:        legacyMessage{ID: 14, Type: "guestBook_reply", TypeID: 22, FromUserID: 4},
			sourceType: "reply",
			sourceID:   22,
			rootType:   "guestbook",
			rootID:     12,
		},
		{
			name:       "文章评论点赞",
			msg:        legacyMessage{ID: 15, Type: "comment_like", TypeID: 10, FromUserID: 4},
			sourceType: "comment",
			sourceID:   10,
			rootType:   "article",
			rootID:     3,
		},
		{
			name:       "碎语评论点赞",
			msg:        legacyMessage{ID: 16, Type: "comment_like", TypeID: 11, FromUserID: 4},
			sourceType: "comment",
			sourceID:   11,
			rootType:   "moment",
			rootID:     4,
		},
		{
			name:       "留言板点赞",
			msg:        legacyMessage{ID: 17, Type: "comment_like", TypeID: 12, FromUserID: 4},
			sourceType: "guestbook",
			sourceID:   12,
			rootType:   "guestbook",
			rootID:     12,
		},
		{
			name:       "文章回复点赞",
			msg:        legacyMessage{ID: 18, Type: "comment_reply_like", TypeID: 20, FromUserID: 4},
			sourceType: "reply",
			sourceID:   20,
			rootType:   "article",
			rootID:     10,
		},
		{
			name:       "碎语回复点赞",
			msg:        legacyMessage{ID: 19, Type: "comment_reply_like", TypeID: 21, FromUserID: 4},
			sourceType: "reply",
			sourceID:   21,
			rootType:   "moment",
			rootID:     11,
		},
		{
			name:       "留言回复点赞",
			msg:        legacyMessage{ID: 20, Type: "comment_reply_like", TypeID: 22, FromUserID: 4},
			sourceType: "reply",
			sourceID:   22,
			rootType:   "guestbook",
			rootID:     12,
		},
		{
			name:       "旧留言板点赞",
			msg:        legacyMessage{ID: 21, Type: "guestBook_like", TypeID: 12, FromUserID: 4},
			sourceType: "guestbook",
			sourceID:   12,
			rootType:   "guestbook",
			rootID:     12,
		},
		{
			name:       "旧留言回复点赞",
			msg:        legacyMessage{ID: 22, Type: "guestBook_reply_like", TypeID: 22, FromUserID: 4},
			sourceType: "reply",
			sourceID:   22,
			rootType:   "guestbook",
			rootID:     12,
		},
		{
			name:       "系统通知",
			msg:        legacyMessage{ID: 23, Type: "system", TypeID: 0},
			sourceType: "system",
			sourceID:   0,
			rootType:   "system",
			rootID:     0,
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			event, err := buildLegacyEvent(tt.msg, refs, nil)

			require.NoError(t, err)
			assert.Equal(t, tt.sourceType, event.SourceType)
			assert.Equal(t, tt.sourceID, event.SourceID)
			assert.Equal(t, tt.rootType, event.RootType)
			assert.Equal(t, tt.rootID, event.RootID)
		})
	}
}

func TestBuildLegacyEvent_MapsLegacyLikeEventTypes(t *testing.T) {
	refs := legacyNotificationRefs{
		comments: map[uint]legacyCommentRef{
			10: {sourceType: "comment", sourceID: 10, rootType: "article", rootID: 3},
			11: {sourceType: "comment", sourceID: 11, rootType: "moment", rootID: 4},
			12: {sourceType: "guestbook", sourceID: 12, rootType: "guestbook", rootID: 12},
		},
		replies: map[uint]legacyReplyRef{
			20: {sourceType: "reply", sourceID: 20, rootType: "article", rootID: 10},
			21: {sourceType: "reply", sourceID: 21, rootType: "moment", rootID: 11},
			22: {sourceType: "reply", sourceID: 22, rootType: "guestbook", rootID: 12},
		},
	}

	cases := []struct {
		name string
		msg  legacyMessage
		want string
	}{
		{name: "文章评论点赞", msg: legacyMessage{ID: 15, Type: "comment_like", TypeID: 10, FromUserID: 4}, want: "comment_liked"},
		{name: "碎语评论点赞", msg: legacyMessage{ID: 16, Type: "comment_like", TypeID: 11, FromUserID: 4}, want: "comment_liked"},
		{name: "留言点赞", msg: legacyMessage{ID: 17, Type: "comment_like", TypeID: 12, FromUserID: 4}, want: "guestbook_liked"},
		{name: "文章回复点赞", msg: legacyMessage{ID: 18, Type: "comment_reply_like", TypeID: 20, FromUserID: 4}, want: "reply_liked"},
		{name: "碎语回复点赞", msg: legacyMessage{ID: 19, Type: "comment_reply_like", TypeID: 21, FromUserID: 4}, want: "reply_liked"},
		{name: "留言回复点赞", msg: legacyMessage{ID: 20, Type: "comment_reply_like", TypeID: 22, FromUserID: 4}, want: "reply_liked"},
		{name: "旧留言点赞", msg: legacyMessage{ID: 21, Type: "guestBook_like", TypeID: 12, FromUserID: 4}, want: "guestbook_liked"},
		{name: "旧留言回复点赞", msg: legacyMessage{ID: 22, Type: "guestBook_reply_like", TypeID: 22, FromUserID: 4}, want: "reply_liked"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			event, err := buildLegacyEvent(tt.msg, refs, nil)

			require.NoError(t, err)
			assert.Equal(t, tt.want, event.Type)
		})
	}
}

func TestBuildLegacyEvent_FallsBackReplyRelationFromMessageColumns(t *testing.T) {
	articleID := uint(2)
	commentID := uint(139)
	msg := legacyMessage{
		ID:         182,
		Type:       "comment_reply",
		TypeID:     136,
		FromUserID: 1,
		ArticleID:  &articleID,
		CommentID:  &commentID,
	}

	event, err := buildLegacyEvent(msg, legacyNotificationRefs{}, nil)

	require.NoError(t, err)
	assert.Equal(t, "reply_created", event.Type)
	assert.Equal(t, "reply", event.SourceType)
	assert.Equal(t, uint(136), event.SourceID)
	assert.Equal(t, "article", event.RootType)
	assert.Equal(t, uint(139), event.RootID)
}

// 未知旧消息保留 legacy 根对象，避免丢失无法解释的历史数据。
func TestBuildLegacyEvent_FallsBackToLegacyRoot(t *testing.T) {
	msg := legacyMessage{ID: 8, Type: "weird_unknown", TypeID: 12, FromUserID: 4}

	event, err := buildLegacyEvent(msg, legacyNotificationRefs{}, nil)

	require.NoError(t, err)
	assert.Equal(t, "legacy_notice", event.Type)
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

func TestBuildLegacyNotificationRefs_ReadsCommentAndReplySemantics(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery("SELECT ID, type, owner_id FROM comment ORDER BY ID").
		WillReturnRows(sqlmock.NewRows([]string{"ID", "type", "owner_id"}).
			AddRow(uint(10), "post", uint(3)).
			AddRow(uint(11), "say", uint(4)).
			AddRow(uint(12), "guestBook", uint(9)))
	mock.ExpectQuery("FROM comment_reply cr\\s+JOIN comment c ON c.ID = cr.comment_id").
		WillReturnRows(sqlmock.NewRows([]string{"ID", "comment_id", "type"}).
			AddRow(uint(20), uint(10), "post").
			AddRow(uint(21), uint(11), "say").
			AddRow(uint(22), uint(12), "guestBook"))

	refs, err := buildLegacyNotificationRefs(db)

	require.NoError(t, err)
	assert.Equal(t, legacyCommentRef{sourceType: "comment", sourceID: 10, rootType: "article", rootID: 3}, refs.comments[10])
	assert.Equal(t, legacyCommentRef{sourceType: "comment", sourceID: 11, rootType: "moment", rootID: 4}, refs.comments[11])
	assert.Equal(t, legacyCommentRef{sourceType: "guestbook", sourceID: 12, rootType: "guestbook", rootID: 12}, refs.comments[12])
	assert.Equal(t, legacyReplyRef{sourceType: "reply", sourceID: 20, rootType: "article", rootID: 10}, refs.replies[20])
	assert.Equal(t, legacyReplyRef{sourceType: "reply", sourceID: 21, rootType: "moment", rootID: 11}, refs.replies[21])
	assert.Equal(t, legacyReplyRef{sourceType: "reply", sourceID: 22, rootType: "guestbook", rootID: 12}, refs.replies[22])
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLegacyNotificationTargetValid_SkipsDeletedArticleLike(t *testing.T) {
	articleID := uint(3)
	msg := legacyMessage{
		ID:        7,
		Type:      "post_like",
		TypeID:    50,
		ArticleID: &articleID,
	}
	existence := notificationExistence{
		articles: map[uint]struct{}{99: {}},
	}

	assert.False(t, legacyNotificationTargetValid(msg, legacyNotificationRefs{}, existence))
}

func TestLegacyNotificationTargetValid_KeepsUnknownLegacyType(t *testing.T) {
	msg := legacyMessage{ID: 8, Type: "weird_unknown", TypeID: 12}

	assert.True(t, legacyNotificationTargetValid(msg, legacyNotificationRefs{}, notificationExistence{}))
}

func TestLegacyNotificationTargetValid_RequiresCommentAndArticle(t *testing.T) {
	refs := legacyNotificationRefs{
		comments: map[uint]legacyCommentRef{
			10: {sourceType: "comment", sourceID: 10, rootType: "article", rootID: 3},
		},
	}
	msg := legacyMessage{ID: 9, Type: "comment", TypeID: 10}
	existence := notificationExistence{
		articleComments: map[uint]struct{}{10: {}},
		articles:        map[uint]struct{}{},
	}

	assert.False(t, legacyNotificationTargetValid(msg, refs, existence))
}

func TestNotificationEventTargetValid_RejectsLegacyFallbackForDeletedSource(t *testing.T) {
	meta := `{"legacy_message_id":7,"legacy_type":"post_like","legacy_type_id":50,"legacy_post_id":3}`
	event := model.NotificationEvent{
		Type:         "legacy_notice",
		SourceType:   "legacy",
		SourceID:     50,
		RootType:     "legacy",
		RootID:       50,
		MetadataJSON: &meta,
	}

	assert.False(t, notificationEventTargetValid(event, notificationExistence{}))
}

func TestBuildLegacyEvent_ReplyCreatedWritesQuotedExcerpt(t *testing.T) {
	msg := legacyMessage{ID: 12, Type: "comment_reply", TypeID: 20, FromUserID: 4}
	refs := legacyNotificationRefs{
		replies: map[uint]legacyReplyRef{
			20: {sourceType: "reply", sourceID: 20, rootType: "article", rootID: 10},
		},
	}
	replyQuotes := map[uint]legacyReplyQuote{
		20: {toUserID: 8, commentID: 10, quotedExcerpt: "被回复的评论"},
	}

	event, err := buildLegacyEvent(msg, refs, replyQuotes)

	require.NoError(t, err)
	assert.Equal(t, "reply_created", event.Type)
	require.NotNil(t, event.MetadataJSON)
	assert.Contains(t, *event.MetadataJSON, `"quoted_excerpt":"被回复的评论"`)
	assert.Contains(t, *event.MetadataJSON, `"recipient_user_ids":[8]`)
	assert.Contains(t, *event.MetadataJSON, `"comment_id":10`)
}

func TestBuildLegacyEvent_ReplyLikedWritesCommentID(t *testing.T) {
	msg := legacyMessage{ID: 18, Type: "comment_reply_like", TypeID: 20, FromUserID: 4}
	replyQuotes := map[uint]legacyReplyQuote{
		20: {toUserID: 8, commentID: 10},
	}

	event, err := buildLegacyEvent(msg, legacyNotificationRefs{}, replyQuotes)

	require.NoError(t, err)
	assert.Equal(t, "reply_liked", event.Type)
	require.NotNil(t, event.MetadataJSON)
	assert.Contains(t, *event.MetadataJSON, `"comment_id":10`)
	assert.Contains(t, *event.MetadataJSON, `"recipient_user_ids":[8]`)
}

func TestMergeLegacyReplyMetadata_PrefersParentReplyContent(t *testing.T) {
	meta := map[string]any{"legacy_message_id": uint(12)}
	replyQuotes := map[uint]legacyReplyQuote{
		21: {toUserID: 9, commentID: 10, quotedExcerpt: "上一层回复"},
	}

	mergeLegacyReplyMetadata(meta, 21, replyQuotes)

	assert.Equal(t, []uint{uint(9)}, meta["recipient_user_ids"])
	assert.Equal(t, uint(10), meta["comment_id"])
	assert.Equal(t, "上一层回复", meta["quoted_excerpt"])
}
