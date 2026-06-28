package moderation

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestMomentMaterializePreservesVisibilityAndCommentSwitches(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	gdb, err := gorm.Open(mysql.New(mysql.Config{Conn: db, SkipInitializeWithVersion: true}), &gorm.Config{SkipDefaultTransaction: true})
	require.NoError(t, err)
	mock.ExpectExec("UPDATE `moment` SET .*comment_status.*content.*status.*WHERE .*id = .*user_id =").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = (&momentAdapter{}).Materialize(context.Background(), gdb, MaterializeCommand{
		Ref: SubjectRef{Type: SubjectMoment, ID: 9}, AuthorID: 7, Content: "隐藏碎语",
		MomentOptions: &MomentOptions{Status: 0, CommentStatus: 0},
	})

	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
