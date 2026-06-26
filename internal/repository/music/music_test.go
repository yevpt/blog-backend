package music_test

import (
	"database/sql"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/model"
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
	mock.ExpectQuery("SELECT \\* FROM `music` WHERE is_public = \\? AND `music`.`deleted_at` IS NULL ORDER BY seq ASC,id ASC").
		WithArgs(true).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "name", "singer", "artist_display_name",
			"album", "album_id", "album_track_no", "song_date", "audio_key", "audio_size",
			"audio_mime", "audio_hash", "cover_img_url", "description", "lyric", "duration", "seq", "is_public",
		}).AddRow(1, now, now, nil, "Song", "Singer", "Singer", "Album", nil, 0, nil, "song.mp3", 0, "", "", nil, nil, nil, 240, 0, true))

	rows, err := repo.List()
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, uint(1), rows[0].ID)
	assert.Equal(t, "Song", rows[0].Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMusicRepository_ListPublicSongs_FiltersPublicAndOrders(t *testing.T) {
	db, mock, sqlDB := newMusicMockDB(t)
	defer sqlDB.Close()
	repo := music.NewMusicRepository(db)

	now := time.Now()
	mock.ExpectQuery("SELECT \\* FROM `music` WHERE is_public = \\? AND `music`.`deleted_at` IS NULL ORDER BY seq ASC,id ASC").
		WithArgs(true).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "name", "singer", "artist_display_name",
			"album", "album_id", "album_track_no", "song_date", "audio_key", "audio_size",
			"audio_mime", "audio_hash", "cover_img_url", "description", "lyric", "duration", "seq", "is_public",
		}).AddRow(1, now, now, nil, "Song", "Singer", "Singer", "Album", nil, 0, nil, "music/audio/1/a.mp3", 12, "audio/mpeg", "hash", nil, nil, nil, 240, 0, true))

	rows, err := repo.ListPublicSongs()

	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "Song", rows[0].Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMusicRepository_MusicArtistRelations_LoadsArtists(t *testing.T) {
	db, mock, sqlDB := newMusicMockDB(t)
	defer sqlDB.Close()
	repo := music.NewMusicRepository(db)

	now := time.Now()
	mock.ExpectQuery("SELECT music_artist_relation.music_id, music_artist.\\* FROM `music_artist_relation` JOIN music_artist ON music_artist.id = music_artist_relation.artist_id AND music_artist.deleted_at IS NULL").
		WillReturnRows(sqlmock.NewRows([]string{
			"music_id", "id", "created_at", "updated_at", "deleted_at", "name", "name_zh", "avatar_key", "description",
		}).AddRow(1, 2, now, now, nil, "Aimer", nil, nil, nil))

	rows, err := repo.MusicArtistRelations([]uint{1})

	require.NoError(t, err)
	require.Len(t, rows[1], 1)
	assert.Equal(t, "Aimer", rows[1][0].Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMusicRepository_SaveMusic_ReplacesArtistRelations(t *testing.T) {
	db, mock, sqlDB := newMusicMockDB(t)
	defer sqlDB.Close()
	repo := music.NewMusicRepository(db)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT `id` FROM `music`").
		WithArgs(uint(3), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(3))
	mock.ExpectExec("UPDATE `music`").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec("DELETE FROM `music_artist_relation` WHERE music_id = \\?").WithArgs(uint(3)).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec("INSERT INTO `music_artist_relation`").WillReturnResult(sqlmock.NewResult(1, 2))
	mock.ExpectCommit()

	err := repo.SaveMusic(music.MusicSaveData{
		Music: model.Music{Base: model.Base{ID: 3}, Name: "Song"},
		ArtistRelations: []model.MusicArtistRelation{
			{MusicID: 3, ArtistID: 1, Role: "primary", Seq: 0},
			{MusicID: 3, ArtistID: 2, Role: "primary", Seq: 1},
		},
	})

	require.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMusicRepository_SaveArtist_RestoresSoftDeletedArtist(t *testing.T) {
	db, mock, sqlDB := newMusicMockDB(t)
	defer sqlDB.Close()
	repo := music.NewMusicRepository(db)

	now := time.Now()
	rows := sqlmock.NewRows([]string{
		"id", "created_at", "updated_at", "deleted_at", "name", "name_zh", "avatar_key", "description",
	}).AddRow(9, now, now, now.Add(-time.Hour), "Aimer", nil, nil, nil)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `music_artist`").
		WithArgs("Aimer", 1).
		WillReturnRows(rows)
	mock.ExpectExec("UPDATE `music_artist` SET .*deleted_at").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT \\* FROM `music_artist`").
		WithArgs(uint(9), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "name", "name_zh", "avatar_key", "description",
		}).AddRow(9, now, now, nil, "Aimer", nil, nil, "restored"))
	mock.ExpectCommit()

	saved, err := repo.SaveArtist(music.MusicArtistSaveData{
		Artist: model.MusicArtist{Name: "Aimer", Description: strPtr("restored")},
	})

	require.NoError(t, err)
	require.NotNil(t, saved)
	assert.Equal(t, uint(9), saved.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMusicRepository_SaveAlbum_RestoresSoftDeletedAlbum(t *testing.T) {
	db, mock, sqlDB := newMusicMockDB(t)
	defer sqlDB.Close()
	repo := music.NewMusicRepository(db)

	now := time.Now()
	artistID := uint(3)
	rows := sqlmock.NewRows([]string{
		"id", "created_at", "updated_at", "deleted_at", "name", "artist_id", "cover_key", "release_date", "description",
	}).AddRow(15, now, now, now.Add(-time.Hour), "Dawn", artistID, nil, nil, nil)

	mock.ExpectBegin()
	mock.ExpectQuery("SELECT \\* FROM `music_album`").
		WithArgs("Dawn", artistID, 1).
		WillReturnRows(rows)
	mock.ExpectExec("UPDATE `music_album` SET .*deleted_at").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT \\* FROM `music_album`").
		WithArgs(uint(15), 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "created_at", "updated_at", "deleted_at", "name", "artist_id", "cover_key", "release_date", "description",
		}).AddRow(15, now, now, nil, "Dawn", artistID, nil, nil, "restored"))
	mock.ExpectCommit()

	saved, err := repo.SaveAlbum(music.MusicAlbumSaveData{
		Album: model.MusicAlbum{Name: "Dawn", ArtistID: &artistID, Description: strPtr("restored")},
	})

	require.NoError(t, err)
	require.NotNil(t, saved)
	assert.Equal(t, uint(15), saved.ID)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func strPtr(s string) *string {
	return &s
}
