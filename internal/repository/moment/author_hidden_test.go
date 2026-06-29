package moment_test

import (
	"database/sql/driver"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	momentrepo "github.com/vpt/blog-backend/internal/repository/moment"
)

func TestMomentRepositoryListIncludesAuthorHiddenRejectedMoment(t *testing.T) {
	db, mock, sqlDB := newMomentMockDB(t)
	defer sqlDB.Close()
	repo := momentrepo.NewMomentRepository(db, true)

	now := time.Now()
	ownerID := uint(7)
	args := append(publicMomentVisibilityArgsWithAuthorHidden(ownerID), ownerID, 10)
	mock.ExpectQuery("SELECT count\\(\\*\\) FROM `moment` LEFT JOIN moderation_item AS public_moderation").
		WithArgs(args[:len(args)-1]...).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery("SELECT moment\\.\\* FROM `moment` LEFT JOIN moderation_item AS public_moderation").
		WithArgs(args...).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "user_id", "content", "status",
			"comment_status", "read_count", "is_top",
		}).AddRow(9, now, now, nil, ownerID, "被驳回的碎语", 0, 1, 0, false))
	expectEmptyRelations(mock, now, uint(9), ownerID)
	mock.ExpectQuery("SELECT `target_id` FROM `user_like`").
		WithArgs(momentrepo.MomentLikeType, ownerID, uint(9)).
		WillReturnRows(sqlmock.NewRows([]string{"target_id"}))

	page, err := repo.List(momentrepo.ListFilter{UserID: &ownerID, Page: 1, PageSize: 10}, &ownerID)

	require.NoError(t, err)
	require.Len(t, page.Moments, 1)
	assert.Equal(t, "被驳回的碎语", page.Moments[0].Moment.Content)
	require.NoError(t, mock.ExpectationsWereMet())
}

func publicMomentVisibilityArgsWithAuthorHidden(userID uint) []driver.Value {
	return append(publicMomentVisibilityArgs(), "hidden", userID)
}
