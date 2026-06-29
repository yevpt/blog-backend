package moderation_test

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	moderation "github.com/vpt/blog-backend/internal/repository/moderation"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newMigrationRepository(t *testing.T) (moderation.MigrationRepository, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{NowFunc: func() time.Time { return fixedTime }})
	require.NoError(t, err)
	return moderation.NewMigrationRepository(gdb), mock
}

func TestListLegacyRecordsSupportsAllSevenTypesAndSoftDeletedRows(t *testing.T) {
	tests := []struct {
		subjectType moderation.SubjectType
		table       string
		rootID      uint64
		parentID    any
	}{
		{moderation.SubjectMoment, "moment", 0, nil},
		{moderation.SubjectArticleComment, "article_comment", 9, nil},
		{moderation.SubjectMomentComment, "moment_comment", 9, nil},
		{moderation.SubjectGuestbook, "guestbook", 9, nil},
		{moderation.SubjectArticleCommentReply, "article_comment_reply", 9, uint64(3)},
		{moderation.SubjectMomentCommentReply, "moment_comment_reply", 9, uint64(3)},
		{moderation.SubjectGuestbookReply, "guestbook_reply", 9, uint64(3)},
	}
	deletedAt := fixedTime.Add(-time.Hour)
	for _, tt := range tests {
		t.Run(string(tt.subjectType), func(t *testing.T) {
			repository, mock := newMigrationRepository(t)
			momentStatus, commentStatus := any(nil), any(nil)
			visible := true
			if tt.subjectType == moderation.SubjectMoment {
				momentStatus, commentStatus = uint8(0), uint8(0)
				visible = false
			}
			mock.ExpectQuery("SELECT .* FROM "+regexp.QuoteMeta(tt.table)+" WHERE id > \\? ORDER BY id ASC LIMIT \\?").
				WithArgs(uint64(5), 20).
				WillReturnRows(sqlmock.NewRows([]string{
					"id", "root_id", "parent_id", "author_id", "content", "visible",
					"moment_status", "moment_comment_status", "created_at", "updated_at", "deleted_at",
				}).AddRow(7, tt.rootID, tt.parentID, 42, "旧内容", visible, momentStatus, commentStatus, fixedTime, fixedTime, deletedAt))
			if tt.subjectType == moderation.SubjectMoment {
				mock.ExpectQuery("SELECT moment_id,url FROM `moment_media` WHERE .*type = \\?.*status = \\?.*deleted_at IS NULL.*ORDER BY moment_id ASC, seq ASC, id ASC").
					WithArgs(uint64(7), uint8(0), uint8(1)).
					WillReturnRows(sqlmock.NewRows([]string{"moment_id", "url"}).AddRow(7, "moments/a.jpg"))
			}

			got, err := repository.ListLegacyRecords(context.Background(), tt.subjectType, 5, 20)

			require.NoError(t, err)
			require.Len(t, got, 1)
			assert.Equal(t, uint64(7), got[0].Subject.ID)
			assert.Equal(t, &deletedAt, got[0].DeletedAt)
			if tt.subjectType == moderation.SubjectMoment {
				assert.Equal(t, []string{"moments/a.jpg"}, got[0].ImageKeys)
				assert.False(t, got[0].Visible)
			}
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestPersistLegacyRecordsCreatesApprovedVersionAndFingerprintSnapshot(t *testing.T) {
	repository, mock := newMigrationRepository(t)
	parentID := uint64(0)
	record := moderation.LegacyRecord{
		Subject:  moderation.SubjectRef{Type: moderation.SubjectArticleCommentReply, ID: 7, RootID: 3, ParentID: &parentID},
		AuthorID: 42, Content: "旧回复", Visible: true, CreatedAt: fixedTime, UpdatedAt: fixedTime,
		Images: []moderation.LegacyImage{{
			Seq: 1, ObjectKey: "comments/a.jpg", SHA256: "sha", MD5: "md5", Size: 10, MediaType: "image/jpeg",
		}},
	}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `moderation_item`").WillReturnResult(sqlmock.NewResult(10, 1))
	mock.ExpectQuery("SELECT .* FROM `moderation_item` WHERE .*content_type = \\? AND content_id = \\?.*LIMIT \\?").
		WillReturnRows(activeItemRows(1, nil))
	mock.ExpectQuery("SELECT .* FROM `moderation_revision` WHERE item_id = \\? AND version = \\?").
		WillReturnError(gorm.ErrRecordNotFound)
	mock.ExpectExec("INSERT INTO `moderation_revision`").WillReturnResult(sqlmock.NewResult(20, 1))
	mock.ExpectExec("INSERT INTO `moderation_image`").WillReturnResult(sqlmock.NewResult(30, 1))
	mock.ExpectExec("INSERT INTO `moderation_revision_image`").WillReturnResult(sqlmock.NewResult(40, 1))
	mock.ExpectExec("INSERT INTO `moderation_visible_image`").WillReturnResult(sqlmock.NewResult(50, 1))
	mock.ExpectExec("UPDATE `moderation_item` SET ").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err := repository.PersistLegacyRecords(context.Background(), []moderation.LegacyRecord{record})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPersistLegacyRecordsIdempotentRerunDoesNotCreateSecondRevision(t *testing.T) {
	repository, mock := newMigrationRepository(t)
	record := moderation.LegacyRecord{
		Subject:  moderation.SubjectRef{Type: moderation.SubjectArticleComment, ID: 7, RootID: 3},
		AuthorID: 42, Content: "旧评论", Visible: true, CreatedAt: fixedTime, UpdatedAt: fixedTime,
	}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT INTO `moderation_item`").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery("SELECT .* FROM `moderation_item` WHERE .*content_type = \\? AND content_id = \\?.*LIMIT \\?").
		WillReturnRows(activeItemRows(1, nil))
	mock.ExpectQuery("SELECT .* FROM `moderation_revision` WHERE item_id = \\? AND version = \\?").
		WillReturnRows(sqlmock.NewRows([]string{"id", "item_id", "version"}).AddRow(20, 10, 1))
	mock.ExpectCommit()

	err := repository.PersistLegacyRecords(context.Background(), []moderation.LegacyRecord{record})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVerifyLegacyChecksEveryContentTypeProfilesFingerprintsAndControl(t *testing.T) {
	repository, mock := newMigrationRepository(t)
	tables := []struct {
		table       string
		subjectType moderation.SubjectType
	}{
		{"moment", moderation.SubjectMoment},
		{"article_comment", moderation.SubjectArticleComment},
		{"moment_comment", moderation.SubjectMomentComment},
		{"guestbook", moderation.SubjectGuestbook},
		{"article_comment_reply", moderation.SubjectArticleCommentReply},
		{"moment_comment_reply", moderation.SubjectMomentCommentReply},
		{"guestbook_reply", moderation.SubjectGuestbookReply},
	}
	for _, item := range tables {
		mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM " + item.table + ".*moderation_item.*moderation_revision").
			WithArgs(item.subjectType).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	}
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM user AS source.*user_moderation_profile").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM moderation_revision_image.*moderation_image").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery("SELECT COUNT\\(\\*\\) = 0 FROM moderation_control").
		WillReturnRows(sqlmock.NewRows([]string{"missing"}).AddRow(0))

	got, err := repository.VerifyLegacy(context.Background())

	require.NoError(t, err)
	assert.Equal(t, int64(2), got.MissingImages)
	require.NoError(t, mock.ExpectationsWereMet())
}
