package avatar_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/chai2010/webp"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	avatarservice "github.com/vpt/blog-backend/internal/service/avatar"
)

type fakeObjectStore struct {
	exists     bool
	existsErr  error
	putErr     error
	putCalled  bool
	getCalled  bool
	useGet     bool
	getBody    []byte
	objectName string
	content    []byte
	contentTyp string
	url        string
}

func (s *fakeObjectStore) ObjectURL(ctx context.Context, objectName string) (string, error) {
	if s.url != "" {
		return s.url, nil
	}
	return "", nil
}

func (s *fakeObjectStore) ObjectExists(ctx context.Context, objectName string) (bool, error) {
	s.objectName = objectName
	return s.exists, s.existsErr
}

func (s *fakeObjectStore) GetObject(ctx context.Context, objectName string) ([]byte, error) {
	s.getCalled = true
	s.objectName = objectName
	return append([]byte(nil), s.getBody...), nil
}

func (s *fakeObjectStore) PutObject(ctx context.Context, objectName string, data []byte, contentType string) error {
	s.putCalled = true
	s.objectName = objectName
	s.content = append([]byte(nil), data...)
	s.contentTyp = contentType
	return s.putErr
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

func (s *fakeObjectStore) DeleteObject(ctx context.Context, objectName string) error {
	return nil
}

func TestService_SaveRemoteAvatar_CompressesAndUploads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(testPNG(t, 240, 200))
	}))
	t.Cleanup(server.Close)

	store := &fakeObjectStore{}
	svc := avatarservice.NewService(store, avatarservice.Options{
		Timeout:  2 * time.Second,
		MaxBytes: 2 << 20,
	})

	objectName, err := svc.SaveRemoteAvatar(context.Background(), server.URL)

	require.NoError(t, err)
	assert.NotEmpty(t, objectName)
	assert.Contains(t, objectName, "avatar/user/")
	assert.Contains(t, objectName, ".webp")
	assert.True(t, store.putCalled)
	assert.Equal(t, objectName, store.objectName)
	assert.Equal(t, "image/webp", store.contentTyp)
	assert.LessOrEqual(t, len(store.content), 20*1024)
}

func TestService_SaveRemoteAvatar_ReusesExistingObject(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(testPNG(t, 120, 120))
	}))
	t.Cleanup(server.Close)

	store := &fakeObjectStore{exists: true}
	svc := avatarservice.NewService(store, avatarservice.Options{Timeout: 2 * time.Second})

	objectName, err := svc.SaveRemoteAvatar(context.Background(), server.URL)

	require.NoError(t, err)
	assert.NotEmpty(t, objectName)
	assert.False(t, store.putCalled)
}

func TestService_SaveRemoteAvatar_UploadsWhenExistsCheckFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(testPNG(t, 120, 120))
	}))
	t.Cleanup(server.Close)

	store := &fakeObjectStore{existsErr: assert.AnError}
	svc := avatarservice.NewService(store, avatarservice.Options{Timeout: 2 * time.Second})

	objectName, err := svc.SaveRemoteAvatar(context.Background(), server.URL)

	require.NoError(t, err)
	assert.NotEmpty(t, objectName)
	assert.True(t, store.putCalled)
}

func TestService_SaveRemoteAvatar_RejectsNonImageContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("not image"))
	}))
	t.Cleanup(server.Close)

	svc := avatarservice.NewService(&fakeObjectStore{}, avatarservice.Options{Timeout: 2 * time.Second})

	_, err := svc.SaveRemoteAvatar(context.Background(), server.URL)

	assert.ErrorIs(t, err, avatarservice.ErrRemoteAvatarInvalid)
}

func TestService_SaveRemoteAvatar_RespectsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		_, _ = w.Write(testPNG(t, 10, 10))
	}))
	t.Cleanup(server.Close)

	svc := avatarservice.NewService(&fakeObjectStore{}, avatarservice.Options{Timeout: time.Millisecond})

	_, err := svc.SaveRemoteAvatar(context.Background(), server.URL)

	assert.Error(t, err)
}

func TestService_SaveUploadedAvatar_PassthroughCompliantWebP(t *testing.T) {
	store := &fakeObjectStore{}
	svc := avatarservice.NewService(store, avatarservice.Options{})
	input := testWebPAvatar(t, 80, 80)

	result, err := svc.SaveUploadedAvatar(context.Background(), "avatar.webp", input)

	require.NoError(t, err)
	assert.True(t, result.Created)
	assert.Contains(t, result.ObjectKey, ".webp")
	assert.Equal(t, input, store.content)
	assert.Equal(t, "image/webp", store.contentTyp)
}

func TestService_SaveUploadedAvatar_CompressesAndUploads(t *testing.T) {
	store := &fakeObjectStore{}
	svc := avatarservice.NewService(store, avatarservice.Options{})

	objectName, err := svc.SaveUploadedAvatar(context.Background(), "avatar.png", testPNG(t, 240, 200))

	require.NoError(t, err)
	assert.NotEmpty(t, objectName.ObjectKey)
	assert.Contains(t, objectName.ObjectKey, "avatar/user/")
	assert.True(t, objectName.Created)
	assert.Equal(t, "image/webp", store.contentTyp)
	assert.LessOrEqual(t, len(store.content), 20*1024)
}

func TestService_SaveUploadedAvatar_RejectsGIF(t *testing.T) {
	svc := avatarservice.NewService(&fakeObjectStore{}, avatarservice.Options{})
	gif := testGIF(t)

	_, err := svc.SaveUploadedAvatar(context.Background(), "avatar.gif", gif)

	assert.ErrorIs(t, err, avatarservice.ErrAvatarGIFNotAllowed)
}

func testGIF(t *testing.T) []byte {
	t.Helper()
	// 最小合法 GIF89a 1x1 像素。
	return []byte{
		0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00, 0x00,
		0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00, 0x00, 0x00,
		0x2c, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02, 0x44, 0x01, 0x00, 0x3b,
	}
}

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: uint8((x + y) % 255), A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func testWebPAvatar(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: uint8((x + y) % 255), A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, webp.Encode(&buf, img, &webp.Options{Lossless: false, Quality: 80}))
	require.LessOrEqual(t, buf.Len(), 20*1024)
	return buf.Bytes()
}
