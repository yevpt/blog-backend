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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	avatarservice "github.com/vpt/blog-backend/internal/service/avatar"
)

type fakeObjectStore struct {
	exists     bool
	existsErr  error
	putErr     error
	putCalled  bool
	objectName string
	content    []byte
	contentTyp string
}

func (s *fakeObjectStore) ObjectURL(ctx context.Context, objectName string) (string, error) {
	return "", nil
}

func (s *fakeObjectStore) ObjectExists(ctx context.Context, objectName string) (bool, error) {
	s.objectName = objectName
	return s.exists, s.existsErr
}

func (s *fakeObjectStore) PutObject(ctx context.Context, objectName string, data []byte, contentType string) error {
	s.putCalled = true
	s.objectName = objectName
	s.content = append([]byte(nil), data...)
	s.contentTyp = contentType
	return s.putErr
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
	assert.Contains(t, objectName, ".jpg")
	assert.True(t, store.putCalled)
	assert.Equal(t, objectName, store.objectName)
	assert.Equal(t, "image/jpeg", store.contentTyp)
	assert.LessOrEqual(t, len(store.content), 10*1024)
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
