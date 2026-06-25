package music_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/internal/model"
	musicrepo "github.com/vpt/blog-backend/internal/repository/music"
	"github.com/vpt/blog-backend/internal/service/music"
	"github.com/vpt/blog-backend/pkg/storage"
)

type stubMusicRepository struct {
	rows    []model.Music
	artists []model.MusicArtist
	err     error
}

func (s *stubMusicRepository) List() ([]model.Music, error) {
	return s.ListPublicSongs()
}

func (s *stubMusicRepository) ListPublicSongs() ([]model.Music, error) {
	return s.rows, s.err
}

func (s *stubMusicRepository) FindMusic(uint) (*model.Music, error) {
	return nil, nil
}

func (s *stubMusicRepository) MusicArtistRelations([]uint) (map[uint][]model.MusicArtist, error) {
	return map[uint][]model.MusicArtist{}, nil
}

func (s *stubMusicRepository) ListArtists(string) ([]model.MusicArtist, error) {
	return nil, nil
}

func (s *stubMusicRepository) FindArtists([]uint) ([]model.MusicArtist, error) {
	return s.artists, nil
}

func (s *stubMusicRepository) FindArtist(uint) (*model.MusicArtist, error) {
	if len(s.artists) == 0 {
		return nil, nil
	}
	return &s.artists[0], nil
}

func (s *stubMusicRepository) ListAlbums(string) ([]model.MusicAlbum, error) {
	return nil, nil
}

func (s *stubMusicRepository) FindAlbum(uint) (*model.MusicAlbum, error) {
	return nil, nil
}

func (s *stubMusicRepository) SaveMusic(musicrepo.MusicSaveData) error {
	return nil
}

func (s *stubMusicRepository) DeleteMusic(uint) error {
	return nil
}

func (s *stubMusicRepository) SaveArtist(musicrepo.MusicArtistSaveData) (*model.MusicArtist, error) {
	return nil, nil
}

func (s *stubMusicRepository) DeleteArtist(uint) error {
	return nil
}

func (s *stubMusicRepository) SaveAlbum(musicrepo.MusicAlbumSaveData) (*model.MusicAlbum, error) {
	return nil, nil
}

func (s *stubMusicRepository) DeleteAlbum(uint) error {
	return nil
}

func (s *stubMusicRepository) ListAdminSongs(string, int, int) ([]model.Music, int64, error) {
	return s.rows, int64(len(s.rows)), s.err
}

var _ musicrepo.MusicRepository = (*stubMusicRepository)(nil)

type stubMusicObjectStore struct{}

func (stubMusicObjectStore) ObjectURL(_ context.Context, objectName string) (string, error) {
	return "https://cdn.example.com/" + objectName, nil
}

func (stubMusicObjectStore) ObjectExists(_ context.Context, key string) (bool, error) {
	return strings.TrimSpace(key) != "", nil
}

func (stubMusicObjectStore) PutObject(context.Context, string, []byte, string) error {
	return nil
}

func (stubMusicObjectStore) DeleteObject(context.Context, string) error {
	return nil
}

func (stubMusicObjectStore) MoveObject(context.Context, string, string) error {
	return nil
}

func (stubMusicObjectStore) CopyObject(context.Context, string, string) error {
	return nil
}

func (stubMusicObjectStore) ObjectKey(string) (string, error) {
	return "", nil
}

var _ storage.ObjectStore = (*stubMusicObjectStore)(nil)

func TestMusicService_List_MapsAndResolvesURLs(t *testing.T) {
	audioKey := "music/song.mp3"
	cover := "music/cover.jpg"
	svc := music.NewMusicService(&stubMusicRepository{
		rows: []model.Music{
			{
				Base:              model.Base{ID: 1},
				Name:              "Song",
				ArtistDisplayName: "Singer",
				AudioKey:          &audioKey,
				CoverImgUrl:       &cover,
				Duration:          240,
				Seq:               2,
				IsPublic:          true,
			},
		},
	}, stubMusicObjectStore{})

	resp, err := svc.List()
	require.NoError(t, err)
	require.Len(t, resp.List, 1)

	item := resp.List[0]
	assert.Equal(t, uint(1), item.ID)
	assert.Equal(t, "Song", item.Name)
	assert.Equal(t, "Singer", item.ArtistDisplayName)
	assert.Equal(t, "https://cdn.example.com/music/song.mp3", *item.AudioURL)
	assert.Equal(t, "https://cdn.example.com/music/cover.jpg", *item.CoverURL)
	assert.Equal(t, uint16(240), item.Duration)
	assert.Equal(t, uint(2), item.Seq)
}

func TestMusicService_List_PropagatesRepoError(t *testing.T) {
	dbErr := errors.New("db error")
	svc := music.NewMusicService(&stubMusicRepository{err: dbErr}, nil)

	_, err := svc.List()
	require.ErrorIs(t, err, dbErr)
}

func TestMusicService_ListPublic_ResolvesAudioAndArtistDisplay(t *testing.T) {
	audioKey := "music/audio/1/hash.mp3"
	repo := &stubMusicRepository{
		rows: []model.Music{{
			Base:              model.Base{ID: 1},
			Name:              "Song",
			ArtistDisplayName: "문성남 (文胜南)",
			AudioKey:          &audioKey,
			Duration:          180,
			IsPublic:          true,
		}},
	}
	svc := music.NewMusicService(repo, stubMusicObjectStore{})

	resp, err := svc.ListPublic()

	require.NoError(t, err)
	require.Len(t, resp.List, 1)
	assert.Equal(t, "https://cdn.example.com/music/audio/1/hash.mp3", *resp.List[0].AudioURL)
	assert.Equal(t, "문성남 (文胜南)", resp.List[0].ArtistDisplayName)
}

func TestMusicService_SaveMusic_RejectsMissingArtists(t *testing.T) {
	repo := &stubMusicRepository{
		artists: []model.MusicArtist{{Base: model.Base{ID: 1}, Name: "Aimer"}},
	}
	svc := music.NewMusicService(repo, nil)

	err := svc.SaveMusic(context.Background(), 1, dto.MusicSaveReq{
		Name:      "Song",
		ArtistIDs: []uint{1, 2},
		AudioKey:  "music/audio/1/a.mp3",
		IsPublic:  true,
	})

	require.ErrorIs(t, err, music.ErrMusicArtistNotFound)
}

func TestMusicService_GetPublicArtist_ReturnsArtist(t *testing.T) {
	svc := music.NewMusicService(&stubMusicRepository{
		artists: []model.MusicArtist{{Base: model.Base{ID: 2}, Name: "Aimer"}},
	}, stubMusicObjectStore{})

	resp, err := svc.GetPublicArtist(2)

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, uint(2), resp.ID)
	assert.Equal(t, "Aimer", resp.Name)
}

func TestMusicService_GetPublicArtist_NotFound(t *testing.T) {
	svc := music.NewMusicService(&stubMusicRepository{}, stubMusicObjectStore{})

	_, err := svc.GetPublicArtist(99)

	require.ErrorIs(t, err, music.ErrMusicArtistNotFound)
}

func TestMusicService_SaveMusic_PersistsAudioMetadata(t *testing.T) {
	repo := &captureMusicRepository{
		stubMusicRepository: stubMusicRepository{
			artists: []model.MusicArtist{{Base: model.Base{ID: 1}, Name: "Aimer"}},
		},
	}
	svc := music.NewMusicService(repo, stubMusicObjectStore{})

	err := svc.SaveMusic(context.Background(), 1, dto.MusicSaveReq{
		Name:      "Song",
		ArtistIDs: []uint{1},
		AudioKey:  "legacy/audio.mp3",
		AudioSize: 123456,
		AudioMime: "audio/mpeg",
		AudioHash: "abc123",
		IsPublic:  true,
	})

	require.NoError(t, err)
	assert.Equal(t, uint64(123456), repo.saved.Music.AudioSize)
	assert.Equal(t, "audio/mpeg", repo.saved.Music.AudioMime)
	assert.Equal(t, "abc123", repo.saved.Music.AudioHash)
}

type captureMusicRepository struct {
	stubMusicRepository
	saved musicrepo.MusicSaveData
}

func (s *captureMusicRepository) SaveMusic(data musicrepo.MusicSaveData) error {
	s.saved = data
	return s.stubMusicRepository.SaveMusic(data)
}

func TestSplitArtistDisplayName_WithChineseTranslation(t *testing.T) {
	name, nameZh := music.SplitArtistDisplayName("문성남 (文胜南)")

	assert.Equal(t, "문성남", name)
	require.NotNil(t, nameZh)
	assert.Equal(t, "文胜南", *nameZh)
}

func TestSplitArtistDisplayName_WithFullWidthParentheses(t *testing.T) {
	name, nameZh := music.SplitArtistDisplayName("문성남（文胜南）")

	assert.Equal(t, "문성남", name)
	require.NotNil(t, nameZh)
	assert.Equal(t, "文胜南", *nameZh)
}

func TestSplitArtistDisplayName_WithoutChineseTranslation(t *testing.T) {
	name, nameZh := music.SplitArtistDisplayName("Aimer")

	assert.Equal(t, "Aimer", name)
	assert.Nil(t, nameZh)
}

func TestSplitArtistTokens_KeepsOrderAndDeduplicates(t *testing.T) {
	names := music.SplitArtistTokens("Aimer / milet feat. 幾田りら / Aimer")

	assert.Equal(t, []string{"Aimer", "milet", "幾田りら"}, names)
}

func TestArtistDisplayName_UsesChineseTranslation(t *testing.T) {
	nameZh := "文胜南"

	assert.Equal(t, "문성남 (文胜南)", music.ArtistDisplayName("문성남", &nameZh))
}
