package audiofile_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/pkg/audiofile"
)

func TestValidateMP3ByID3Header(t *testing.T) {
	data := append([]byte("ID3\x04\x00\x00\x00\x00\x00\x00"), []byte("payload")...)

	result, err := audiofile.Validate("song.mp3", data, 1024)

	require.NoError(t, err)
	assert.Equal(t, "audio/mpeg", result.ContentType)
	assert.Equal(t, ".mp3", result.Ext)
	assert.NotEmpty(t, result.SHA256)
	assert.Equal(t, uint64(len(data)), result.Size)
}

func TestValidateRejectsInvalidAudio(t *testing.T) {
	_, err := audiofile.Validate("song.txt", []byte("not audio"), 1024)

	require.ErrorIs(t, err, audiofile.ErrInvalidAudio)
}

func TestValidateRejectsTooLarge(t *testing.T) {
	_, err := audiofile.Validate("song.mp3", append([]byte("ID3"), make([]byte, 20)...), 4)

	require.ErrorIs(t, err, audiofile.ErrAudioTooLarge)
}
