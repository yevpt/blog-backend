package music

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/pkg/storage"
)

type trackingMusicObjectStore struct {
	keys   map[string]bool
	copies [][2]string
}

func (s *trackingMusicObjectStore) ObjectURL(context.Context, string) (string, error) {
	return "", nil
}

func (s *trackingMusicObjectStore) ObjectExists(_ context.Context, key string) (bool, error) {
	return s.keys[key], nil
}

func (s *trackingMusicObjectStore) PutObject(context.Context, string, []byte, string) error {
	return nil
}

func (s *trackingMusicObjectStore) DeleteObject(context.Context, string) error {
	return nil
}

func (s *trackingMusicObjectStore) MoveObject(context.Context, string, string) error {
	return nil
}

func (s *trackingMusicObjectStore) CopyObject(_ context.Context, sourceName, targetName string) error {
	s.copies = append(s.copies, [2]string{sourceName, targetName})
	s.keys[targetName] = true
	return nil
}

func (s *trackingMusicObjectStore) ObjectKey(value string) (string, error) {
	return value, nil
}

var _ storage.ObjectStore = (*trackingMusicObjectStore)(nil)

func TestNormalizeMusicAudioKey_PromotesTempToFormal(t *testing.T) {
	store := &trackingMusicObjectStore{
		keys: map[string]bool{
			"temp/music/7/audio/abc.mp3": true,
		},
	}
	result, err := normalizeMusicAudioKey(
		context.Background(),
		store,
		7,
		12,
		"temp/music/7/audio/abc.mp3",
		nil,
	)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "music/audio/12/abc.mp3", result.Key)
	assert.Equal(t, "temp/music/7/audio/abc.mp3", result.TempKey)
	assert.Equal(t, [][2]string{{"temp/music/7/audio/abc.mp3", "music/audio/12/abc.mp3"}}, store.copies)
}

func TestNormalizeMusicAudioKey_RejectsOtherUserTemp(t *testing.T) {
	store := &trackingMusicObjectStore{
		keys: map[string]bool{
			"temp/music/9/audio/abc.mp3": true,
		},
	}
	_, err := normalizeMusicAudioKey(
		context.Background(),
		store,
		7,
		12,
		"temp/music/9/audio/abc.mp3",
		nil,
	)

	require.ErrorIs(t, err, ErrMusicAssetInvalid)
}
