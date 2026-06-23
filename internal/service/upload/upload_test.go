package upload_test

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	uploadservice "github.com/vpt/blog-backend/internal/service/upload"
)

type fakeObjectStore struct {
	exists         bool
	existsErr      error
	putErr         error
	urlErr         error
	puts           []putRecord
	objectURLByKey map[string]string
}

type putRecord struct {
	key         string
	data        []byte
	contentType string
}

func (s *fakeObjectStore) ObjectURL(ctx context.Context, objectName string) (string, error) {
	if s.urlErr != nil {
		return "", s.urlErr
	}
	if s.objectURLByKey == nil {
		return "https://cdn.example.com/blog/" + objectName, nil
	}
	if url, ok := s.objectURLByKey[objectName]; ok {
		return url, nil
	}
	return "https://cdn.example.com/blog/" + objectName, nil
}

func (s *fakeObjectStore) MoveObject(ctx context.Context, sourceName string, targetName string) error {
	return nil
}

func (s *fakeObjectStore) CopyObject(ctx context.Context, sourceName string, targetName string) error {
	return nil
}

func (s *fakeObjectStore) ObjectKey(value string) (string, error) {
	return value, nil
}

func (s *fakeObjectStore) ObjectExists(ctx context.Context, objectName string) (bool, error) {
	return s.exists, s.existsErr
}

func (s *fakeObjectStore) PutObject(ctx context.Context, objectName string, data []byte, contentType string) error {
	if s.putErr != nil {
		return s.putErr
	}
	s.puts = append(s.puts, putRecord{
		key:         objectName,
		data:        append([]byte(nil), data...),
		contentType: contentType,
	})
	return nil
}

func (s *fakeObjectStore) DeleteObject(ctx context.Context, objectName string) error {
	return nil
}

func TestService_UploadTempImage_StoresUserScopedKey(t *testing.T) {
	store := &fakeObjectStore{objectURLByKey: map[string]string{}}
	svc := uploadservice.NewService(store)
	file := smallPNG(t)
	sum := md5.Sum(file)
	expectedMD5 := hex.EncodeToString(sum[:])

	resp, err := svc.UploadTempImage(context.Background(), uploadservice.TempImageInput{
		UserID: 7,
		Dir:    "images",
		Name:   "cat.png",
		Data:   file,
	})

	require.NoError(t, err)
	assert.Equal(t, "temp/articles/7/images/"+expectedMD5+".png", resp.Key)
	assert.Equal(t, "https://cdn.example.com/blog/"+resp.Key, resp.URL)
	require.Len(t, store.puts, 1)
	assert.Equal(t, resp.Key, store.puts[0].key)
	assert.Equal(t, "image/png", store.puts[0].contentType)
	assert.Equal(t, file, store.puts[0].data)
}

func TestService_UploadTempImage_RejectsInvalidDir(t *testing.T) {
	svc := uploadservice.NewService(&fakeObjectStore{})

	_, err := svc.UploadTempImage(context.Background(), uploadservice.TempImageInput{
		UserID: 7,
		Dir:    "../images",
		Name:   "cat.png",
		Data:   smallPNG(t),
	})

	require.ErrorIs(t, err, uploadservice.ErrUploadDirInvalid)
}

func TestService_UploadTempImage_SkipsUploadWhenObjectExists(t *testing.T) {
	store := &fakeObjectStore{
		exists: true,
		objectURLByKey: map[string]string{
			"temp/articles/7/covers/45eb8f0de73f0680e67d356d2f2eb2cc.png": "https://cdn.example.com/blog/temp/articles/7/covers/45eb8f0de73f0680e67d356d2f2eb2cc.png",
		},
	}
	svc := uploadservice.NewService(store)

	resp, err := svc.UploadTempImage(context.Background(), uploadservice.TempImageInput{
		UserID: 7,
		Dir:    "covers",
		Name:   "cover.png",
		Data:   []byte{137, 80, 78, 71, 13, 10, 26, 10, 0, 0, 0, 13, 73, 72, 68, 82, 0, 0, 0, 1, 0, 0, 0, 1, 8, 2, 0, 0, 0, 144, 119, 83, 222, 0, 0, 0, 12, 73, 68, 65, 84, 8, 153, 99, 248, 255, 255, 63, 0, 5, 254, 2, 254, 167, 141, 163, 75, 0, 0, 0, 0, 73, 69, 78, 68, 174, 66, 96, 130},
	})

	require.NoError(t, err)
	assert.NotEmpty(t, resp.URL)
	assert.Len(t, store.puts, 0)
}

func TestService_UploadTempImage_ReturnsUnavailableWhenStoreFails(t *testing.T) {
	store := &fakeObjectStore{putErr: errors.New("s3 down")}
	svc := uploadservice.NewService(store)

	_, err := svc.UploadTempImage(context.Background(), uploadservice.TempImageInput{
		UserID: 7,
		Dir:    "images",
		Name:   "cat.png",
		Data:   smallPNG(t),
	})

	require.ErrorIs(t, err, uploadservice.ErrUploadUnavailable)
}

func smallPNG(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}
