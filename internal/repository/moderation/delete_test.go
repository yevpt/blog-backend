package moderation

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestTombstoneDescendantSupersedesPendingAndMarksTerminal(t *testing.T) {
	now := time.Date(2026, time.June, 28, 12, 0, 0, 0, time.UTC)
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{
		NowFunc: func() time.Time { return now }, SkipDefaultTransaction: true,
	})
	require.NoError(t, err)
	ref := SubjectRef{Type: SubjectArticleCommentReply, ID: 9, RootID: 7, ParentID: uint64Pointer(0)}
	mock.ExpectQuery("SELECT .* FROM `moderation_item`.*FOR UPDATE").
		WithArgs(SubjectArticleCommentReply, uint64(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "content_type", "content_id", "author_id", "lifecycle_state", "public_state",
			"materialized_revision_id", "approved_revision_id", "pending_revision_id",
			"state_before_emergency", "emergency_hidden_reason", "emergency_hidden_at", "deleted_at",
			"lock_version", "created_at", "updated_at",
		}).AddRow(12, SubjectArticleCommentReply, 9, 42, LifecycleActive, PublicVisible,
			81, 80, 81, nil, nil, nil, nil, 4, now, now))
	mock.ExpectExec("UPDATE `moderation_revision` SET .*review_status.*updated_at.*WHERE id = .*item_id = .*review_status =").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("UPDATE `moderation_item` SET .* WHERE id = .*lock_version =").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = tombstoneDescendant(context.Background(), gdb, ref)

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteSubjectLikesUsesStableBusinessTypeMapping(t *testing.T) {
	tests := []struct {
		subjectType SubjectType
		likeType    uint8
	}{
		{SubjectArticleComment, articleCommentLikeType},
		{SubjectArticleCommentReply, articleCommentReplyLikeType},
		{SubjectMoment, momentLikeType},
		{SubjectGuestbook, guestbookLikeType},
		{SubjectMomentComment, momentCommentLikeType},
		{SubjectMomentCommentReply, momentCommentReplyLikeType},
		{SubjectGuestbookReply, guestbookReplyLikeType},
	}
	for _, test := range tests {
		t.Run(string(test.subjectType), func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{SkipDefaultTransaction: true})
			require.NoError(t, err)
			mock.ExpectExec("DELETE FROM `user_like` WHERE target_id = .*type =").
				WithArgs(uint64(9), test.likeType).
				WillReturnResult(sqlmock.NewResult(0, 1))

			err = deleteSubjectLikes(context.Background(), gdb, SubjectRef{Type: test.subjectType, ID: 9})

			require.NoError(t, err)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
