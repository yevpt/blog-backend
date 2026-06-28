package moderation

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestReplyDescendantsUseTypedRecursiveQueriesAndStableOrder(t *testing.T) {
	tests := []struct {
		name        string
		subjectType SubjectType
		table       string
		adapter     subjectAdapter
	}{
		{name: "article", subjectType: SubjectArticleCommentReply, table: "article_comment_reply", adapter: &commentAdapter{}},
		{name: "moment", subjectType: SubjectMomentCommentReply, table: "moment_comment_reply", adapter: &commentAdapter{}},
		{name: "guestbook", subjectType: SubjectGuestbookReply, table: "guestbook_reply", adapter: &guestbookAdapter{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			require.NoError(t, err)
			gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{})
			require.NoError(t, err)
			mock.ExpectQuery("WITH RECURSIVE reply_tree AS .*`"+tt.table+"`.*UNION ALL.*`"+tt.table+"`.*ORDER BY id ASC").
				WithArgs(uint64(5), uint64(7), uint64(7)).
				WillReturnRows(sqlmock.NewRows([]string{"id", "comment_id", "parent_reply_id"}).
					AddRow(11, 7, 8).
					AddRow(8, 7, 5))

			got, err := tt.adapter.Descendants(context.Background(), gdb, SubjectRef{
				Type: tt.subjectType, ID: 5, RootID: 7, ParentID: uint64Pointer(0),
			})

			require.NoError(t, err)
			require.Len(t, got, 2)
			assert.Equal(t, uint64(8), got[0].ID)
			assert.Equal(t, uint64(11), got[1].ID)
			assert.Equal(t, uint64(5), *got[0].ParentID)
			assert.Equal(t, uint64(8), *got[1].ParentID)
			require.NoError(t, mock.ExpectationsWereMet())
		})
	}
}
