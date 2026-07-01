package moderation_test

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderation "github.com/vpt/blog-backend/internal/repository/moderation"
)

func TestListReviewRecordsReturnsPendingRevisionAndMomentOptions(t *testing.T) {
	repository, mock := newRepository(t)
	mock.ExpectQuery("SELECT count\\(\\*\\) " + currentRevisionJoinPattern).
		WithArgs(moderation.ReviewPending).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .* "+currentRevisionJoinPattern+".*ORDER BY moderation_revision.created_at DESC,moderation_revision.id DESC LIMIT \\?").
		WithArgs(moderation.ReviewPending, 20).
		WillReturnRows(reviewRecordRows().AddRow(
			uint64(10), "moment", uint64(7), uint64(42), uint64(3), "active", "placeholder",
			nil, nil, uint64(20), nil, nil, nil, nil, uint64(20), uint64(1), "待审原文", "待审正文",
			"medium", "pre_review", "pending", uint8(1), uint8(0), nil, nil, nil, nil, fixedTime,
		))

	page, err := repository.ListReviewRecords(context.Background(), moderation.ReviewFilter{
		Page: 1, PageSize: 20, ReviewStatus: reviewStatusPtr(moderation.ReviewPending),
	})

	require.NoError(t, err)
	assert.Equal(t, int64(1), page.Total)
	require.Len(t, page.Items, 1)
	item := page.Items[0]
	assert.Equal(t, moderation.SubjectRef{Type: moderation.SubjectMoment, ID: 7}, item.Subject)
	assert.Equal(t, uint64(20), item.RevisionID)
	require.NotNil(t, item.MomentOptions)
	assert.Equal(t, moderation.MomentOptions{Status: 1, CommentStatus: 0}, *item.MomentOptions)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestListReviewRecordsReturnsCurrentRevision(t *testing.T) {
	repository, mock := newRepository(t)
	mock.ExpectQuery("SELECT count\\(\\*\\) " + currentRevisionJoinPattern).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT .* " + currentRevisionJoinPattern + ".*ORDER BY moderation_revision.created_at DESC,moderation_revision.id DESC LIMIT \\?").
		WithArgs(20).
		WillReturnRows(reviewRecordRows().AddRow(
			uint64(10), "moment", uint64(7), uint64(42), uint64(3), "active", "visible",
			uint64(11), uint64(11), uint64(13), nil, nil, nil, nil, uint64(13), uint64(3), "当前待审原文", "当前待审正文",
			"medium", "pre_review", "pending", uint8(1), uint8(1), nil, nil, nil, nil, fixedTime,
		))

	page, err := repository.ListReviewRecords(context.Background(), moderation.ReviewFilter{Page: 1, PageSize: 20})

	require.NoError(t, err)
	assert.Equal(t, int64(1), page.Total)
	require.Len(t, page.Items, 1)
	assert.Equal(t, uint64(13), page.Items[0].RevisionID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadReviewRecordReturnsCanonicalCommentRelation(t *testing.T) {
	repository, mock := newRepository(t)
	mock.ExpectQuery("SELECT .* FROM `moderation_revision` JOIN moderation_item.*WHERE .*moderation_item.id = \\?.*moderation_revision.id = \\?.*LIMIT \\?").
		WithArgs(uint64(10), uint64(20), 1).
		WillReturnRows(reviewRecordRows().AddRow(
			uint64(10), "article_comment", uint64(7), uint64(42), uint64(3), "active", "visible",
			uint64(19), uint64(19), uint64(20), nil, nil, nil, nil, uint64(20), uint64(2), "待审原文", "待审正文",
			"medium", "pre_review", "pending", nil, nil, nil, nil, nil, nil, fixedTime,
		))
	mock.ExpectQuery("SELECT .* FROM `article_comment` WHERE .*`id` = \\?.*LIMIT \\?").
		WithArgs(uint64(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "article_id", "user_id", "content"}).
			AddRow(7, 3, 42, "当前公开正文"))

	record, err := repository.LoadReviewRecord(context.Background(), 10, 20)

	require.NoError(t, err)
	assert.Equal(t, moderation.SubjectRef{Type: moderation.SubjectArticleComment, ID: 7, RootID: 3}, record.Subject)
	assert.Equal(t, uint64(42), record.AuthorID)
	assert.Equal(t, moderation.ExistingRevision(19), record.State.Approved)
	assert.Equal(t, moderation.ExistingRevision(20), record.State.Pending)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadCurrentReviewRecordPrefersPendingRevision(t *testing.T) {
	repository, mock := newRepository(t)
	mock.ExpectQuery("SELECT `pending_revision_id` FROM `moderation_item` WHERE id = \\?.*LIMIT \\?").
		WithArgs(uint64(10), 1).
		WillReturnRows(sqlmock.NewRows([]string{"pending_revision_id"}).AddRow(uint64(20)))
	mock.ExpectQuery("SELECT .* FROM `moderation_revision` JOIN moderation_item.*WHERE .*moderation_item.id = \\?.*moderation_revision.id = \\?.*LIMIT \\?").
		WithArgs(uint64(10), uint64(20), 1).
		WillReturnRows(reviewRecordRows().AddRow(
			uint64(10), "moment", uint64(7), uint64(42), uint64(3), "active", "placeholder",
			nil, nil, uint64(20), nil, nil, nil, nil, uint64(20), uint64(1), "原文", "正文",
			"medium", "pre_review", "pending", uint8(1), uint8(1), nil, nil, nil, nil, fixedTime,
		))
	mock.ExpectQuery("SELECT .* FROM `moment` WHERE `moment`.`id` = \\?.*LIMIT \\?").
		WithArgs(uint64(7), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "user_id", "content"}).AddRow(7, 42, ""))

	record, err := repository.LoadCurrentReviewRecord(context.Background(), 10)

	require.NoError(t, err)
	assert.Equal(t, uint64(20), record.RevisionID)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadReviewNotificationContextResolvesInteractionRecipient(t *testing.T) {
	tests := []struct {
		name            string
		ref             moderation.SubjectRef
		wantRecipientID uint64
		wantQuote       bool
		expectQueries   func(sqlmock.Sqlmock)
	}{
		{
			name:            "article comment uses article author",
			ref:             moderation.SubjectRef{Type: moderation.SubjectArticleComment, RootID: 11},
			wantRecipientID: 101,
			expectQueries: func(mock sqlmock.Sqlmock) {
				expectRecipient(mock, "article", "user_id", 11, 101)
				mock.ExpectQuery("SELECT .* FROM `article` WHERE `article`.`id` = \\?.*LIMIT \\?").
					WithArgs(uint64(11), 1).
					WillReturnRows(sqlmock.NewRows([]string{"id", "title", "short_content", "content"}).
						AddRow(11, "文章标题", "文章摘要", "文章正文"))
			},
		},
		{
			name:            "moment comment uses moment author",
			ref:             moderation.SubjectRef{Type: moderation.SubjectMomentComment, RootID: 12},
			wantRecipientID: 102,
			expectQueries: func(mock sqlmock.Sqlmock) {
				expectRecipient(mock, "moment", "user_id", 12, 102)
				mock.ExpectQuery("SELECT .* FROM `moment` WHERE `moment`.`id` = \\?.*LIMIT \\?").
					WithArgs(uint64(12), 1).
					WillReturnRows(sqlmock.NewRows([]string{"id", "content"}).AddRow(12, "碎语正文"))
			},
		},
		{
			name:            "guestbook uses owner from root id",
			ref:             moderation.SubjectRef{Type: moderation.SubjectGuestbook, ID: 13, RootID: 103},
			wantRecipientID: 103,
			expectQueries: func(mock sqlmock.Sqlmock) {
				mock.ExpectQuery("SELECT .* FROM `guestbook` WHERE `guestbook`.`id` = \\?.*LIMIT \\?").
					WithArgs(uint64(13), 1).
					WillReturnRows(sqlmock.NewRows([]string{"id", "content"}).AddRow(13, "留言正文"))
			},
		},
		{
			name:            "materialized article reply uses to user",
			ref:             moderation.SubjectRef{Type: moderation.SubjectArticleCommentReply, ID: 21, RootID: 14, ParentID: uint64Ptr(20)},
			wantRecipientID: 104,
			wantQuote:       true,
			expectQueries: func(mock sqlmock.Sqlmock) {
				expectMaterializedReplyRecipient(mock, "article_comment_reply", 21, 104)
				expectArticleReplyContext(mock, 14, 31, 20)
			},
		},
		{
			name:            "new article reply to comment uses comment author",
			ref:             moderation.SubjectRef{Type: moderation.SubjectArticleCommentReply, RootID: 15, ParentID: uint64Ptr(0)},
			wantRecipientID: 105,
			wantQuote:       true,
			expectQueries: func(mock sqlmock.Sqlmock) {
				expectRecipient(mock, "article_comment", "user_id", 15, 105)
				expectArticleReplyRoot(mock, 15, 32)
				expectArticleRoot(mock, 32)
			},
		},
		{
			name:            "new moment reply to parent uses parent author",
			ref:             moderation.SubjectRef{Type: moderation.SubjectMomentCommentReply, RootID: 16, ParentID: uint64Ptr(22)},
			wantRecipientID: 106,
			wantQuote:       true,
			expectQueries: func(mock sqlmock.Sqlmock) {
				expectNestedReplyRecipient(mock, "moment_comment_reply", 22, 16, 106)
				expectMomentReplyRoot(mock, 16, 33)
				expectMomentRoot(mock, 33)
				expectParentReplyContent(mock, "moment_comment_reply", 22)
			},
		},
		{
			name:            "materialized moment reply uses to user",
			ref:             moderation.SubjectRef{Type: moderation.SubjectMomentCommentReply, ID: 23, RootID: 17, ParentID: uint64Ptr(0)},
			wantRecipientID: 107,
			wantQuote:       true,
			expectQueries: func(mock sqlmock.Sqlmock) {
				expectMaterializedReplyRecipient(mock, "moment_comment_reply", 23, 107)
				expectMomentReplyRoot(mock, 17, 34)
				expectMomentRoot(mock, 34)
			},
		},
		{
			name:            "new guestbook reply to message uses message author",
			ref:             moderation.SubjectRef{Type: moderation.SubjectGuestbookReply, RootID: 18, ParentID: uint64Ptr(0)},
			wantRecipientID: 108,
			wantQuote:       true,
			expectQueries: func(mock sqlmock.Sqlmock) {
				expectRecipient(mock, "guestbook", "from_user_id", 18, 108)
				expectGuestbookReplyContext(mock, 18)
			},
		},
		{
			name:            "new guestbook reply to parent uses parent author",
			ref:             moderation.SubjectRef{Type: moderation.SubjectGuestbookReply, RootID: 19, ParentID: uint64Ptr(24)},
			wantRecipientID: 109,
			wantQuote:       true,
			expectQueries: func(mock sqlmock.Sqlmock) {
				expectNestedReplyRecipient(mock, "guestbook_reply", 24, 19, 109)
				expectGuestbookReplyContext(mock, 19)
				expectParentReplyContent(mock, "guestbook_reply", 24)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repository, mock := newRepository(t)
			tt.expectQueries(mock)

			got, err := repository.LoadReviewNotificationContext(context.Background(), tt.ref)

			require.NoError(t, err)
			assert.Equal(t, tt.wantRecipientID, got.InteractionRecipientUserID)
			assert.NotNil(t, got.RootSnapshot)
			if tt.wantQuote {
				assert.NotNil(t, got.QuoteSnapshot)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestLoadReviewNotificationContextRecipientLookupFailureAborts(t *testing.T) {
	repository, mock := newRepository(t)
	mock.ExpectQuery("SELECT `user_id` FROM `article` WHERE .*id = \\?.*LIMIT \\?").
		WithArgs(uint64(99), 1).
		WillReturnError(assert.AnError)

	_, err := repository.LoadReviewNotificationContext(context.Background(), moderation.SubjectRef{
		Type:   moderation.SubjectArticleComment,
		RootID: 99,
	})

	require.ErrorIs(t, err, assert.AnError)
	require.NoError(t, mock.ExpectationsWereMet())
}

func expectMaterializedReplyRecipient(mock sqlmock.Sqlmock, table string, replyID, recipientID uint64) {
	mock.ExpectQuery("SELECT `to_user_id` FROM `"+table+"` WHERE id = \\?.*LIMIT \\?").
		WithArgs(replyID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"to_user_id"}).AddRow(recipientID))
}

func expectArticleReplyContext(mock sqlmock.Sqlmock, commentID, articleID, parentID uint64) {
	expectArticleReplyRoot(mock, commentID, articleID)
	expectArticleRoot(mock, articleID)
	expectParentReplyContent(mock, "article_comment_reply", parentID)
}

func expectArticleReplyRoot(mock sqlmock.Sqlmock, commentID, articleID uint64) {
	mock.ExpectQuery("SELECT .* FROM `article_comment` WHERE `article_comment`.`id` = \\?.*LIMIT \\?").
		WithArgs(commentID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "article_id", "content"}).AddRow(commentID, articleID, "文章评论"))
}

func expectMomentReplyRoot(mock sqlmock.Sqlmock, commentID, momentID uint64) {
	mock.ExpectQuery("SELECT .* FROM `moment_comment` WHERE `moment_comment`.`id` = \\?.*LIMIT \\?").
		WithArgs(commentID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "moment_id", "content"}).AddRow(commentID, momentID, "碎语评论"))
}

func expectGuestbookReplyContext(mock sqlmock.Sqlmock, messageID uint64) {
	mock.ExpectQuery("SELECT .* FROM `guestbook` WHERE `guestbook`.`id` = \\?.*LIMIT \\?").
		WithArgs(messageID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "content"}).AddRow(messageID, "留言正文"))
	mock.ExpectQuery("SELECT .* FROM `guestbook` WHERE `guestbook`.`id` = \\?.*LIMIT \\?").
		WithArgs(messageID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"content"}).AddRow("留言正文"))
}

func expectArticleRoot(mock sqlmock.Sqlmock, articleID uint64) {
	mock.ExpectQuery("SELECT .* FROM `article` WHERE `article`.`id` = \\?.*LIMIT \\?").
		WithArgs(articleID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "title", "short_content", "content"}).
			AddRow(articleID, "文章标题", "文章摘要", "文章正文"))
}

func expectMomentRoot(mock sqlmock.Sqlmock, momentID uint64) {
	mock.ExpectQuery("SELECT .* FROM `moment` WHERE `moment`.`id` = \\?.*LIMIT \\?").
		WithArgs(momentID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "content"}).AddRow(momentID, "碎语正文"))
}

func expectRecipient(mock sqlmock.Sqlmock, table, column string, rootID, recipientID uint64) {
	mock.ExpectQuery("SELECT `"+column+"` FROM `"+table+"` WHERE .*id = \\?.*LIMIT \\?").
		WithArgs(rootID, 1).
		WillReturnRows(sqlmock.NewRows([]string{column}).AddRow(recipientID))
}

func expectNestedReplyRecipient(mock sqlmock.Sqlmock, table string, replyID, rootID, recipientID uint64) {
	mock.ExpectQuery("SELECT `from_user_id` FROM `"+table+"` WHERE .*id = \\?.*comment_id = \\?.*LIMIT \\?").
		WithArgs(replyID, rootID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"from_user_id"}).AddRow(recipientID))
}

func expectParentReplyContent(mock sqlmock.Sqlmock, table string, replyID uint64) {
	mock.ExpectQuery("SELECT `content` FROM `"+table+"` WHERE `"+table+"`.`id` = \\?.*LIMIT \\?").
		WithArgs(replyID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"content"}).AddRow("上级回复"))
}

func uint64Ptr(value uint64) *uint64 {
	return &value
}

func reviewRecordRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"item_id", "content_type", "content_id", "author_id", "lock_version", "lifecycle_state", "public_state",
		"materialized_revision_id", "approved_revision_id", "pending_revision_id",
		"state_before_emergency", "emergency_hidden_reason", "emergency_hidden_at", "deleted_at",
		"revision_id", "revision_version", "submitted_content", "published_content", "risk_level",
		"policy_action", "review_status", "moment_status", "moment_comment_status",
		"decision_type", "decision_reason", "reviewer_id", "reviewed_at", "created_at",
	})
}

func reviewStatusPtr(status moderation.ReviewStatus) *moderation.ReviewStatus {
	return &status
}

const currentRevisionJoinPattern = "FROM `moderation_item` JOIN moderation_revision ON moderation_revision.id = COALESCE\\(.*moderation_item.pending_revision_id,.*\\(SELECT latest.id FROM moderation_revision AS latest WHERE latest.item_id = moderation_item.id ORDER BY latest.version DESC LIMIT 1\\).*\\)"
