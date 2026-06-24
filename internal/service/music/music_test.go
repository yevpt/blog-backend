package music_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/model"
	musicrepo "github.com/vpt/blog-backend/internal/repository/music"
	"github.com/vpt/blog-backend/internal/service/music"
)

type stubMusicRepository struct {
	rows []model.Music
	err  error
}

func (s *stubMusicRepository) List() ([]model.Music, error) {
	return s.rows, s.err
}

var _ musicrepo.MusicRepository = (*stubMusicRepository)(nil)

type stubMusicResolver struct{}

func (stubMusicResolver) ObjectURL(_ context.Context, objectName string) (string, error) {
	return "https://cdn.example.com/" + objectName, nil
}

func TestMusicService_List_MapsAndResolvesURLs(t *testing.T) {
	url := "music/song.mp3"
	cover := "music/cover.jpg"
	svc := music.NewMusicService(&stubMusicRepository{
		rows: []model.Music{
			{
				Base:        model.Base{ID: 1},
				Name:        "Song",
				Singer:      "Singer",
				Album:       "Album",
				URL:         &url,
				CoverImgUrl: &cover,
				Duration:    240,
				Seq:         2,
			},
		},
	}, stubMusicResolver{})

	resp, err := svc.List()
	require.NoError(t, err)
	require.Len(t, resp.List, 1)

	item := resp.List[0]
	assert.Equal(t, uint(1), item.ID)
	assert.Equal(t, "Song", item.Name)
	assert.Equal(t, "Singer", item.Singer)
	assert.Equal(t, "Album", item.Album)
	assert.Equal(t, "https://cdn.example.com/music/song.mp3", *item.URL)
	assert.Equal(t, "https://cdn.example.com/music/cover.jpg", *item.CoverImgUrl)
	assert.Equal(t, uint16(240), item.Duration)
	assert.Equal(t, uint(2), item.Seq)
}

func TestMusicService_List_PropagatesRepoError(t *testing.T) {
	dbErr := errors.New("db error")
	svc := music.NewMusicService(&stubMusicRepository{err: dbErr}, nil)

	_, err := svc.List()
	require.ErrorIs(t, err, dbErr)
}
