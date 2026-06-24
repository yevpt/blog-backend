package music_test

import (
	"database/sql"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/repository/music"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newMusicMockDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock, *sql.DB) {
	t.Helper()

	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)

	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{})
	require.NoError(t, err)

	return gormDB, mock, sqlDB
}

func TestMusicRepository_List_OrdersBySeqAndID(t *testing.T) {
	db, mock, sqlDB := newMusicMockDB(t)
	defer sqlDB.Close()
	repo := music.NewMusicRepository(db)

	now := time.Now()
	mock.ExpectQuery("SELECT \\* FROM `music` WHERE `music`.`deleted_at` IS NULL ORDER BY seq ASC,id ASC").
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "name", "singer", "album",
			"song_date", "url", "cover_img_url", "description", "lyric", "duration", "seq",
		}).AddRow(1, now, now, nil, "Song", "Singer", "Album", nil, "song.mp3", nil, nil, nil, 240, 0))

	rows, err := repo.List()
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, uint(1), rows[0].ID)
	assert.Equal(t, "Song", rows[0].Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}
