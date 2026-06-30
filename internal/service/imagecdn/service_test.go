package imagecdn_test

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appconfig "github.com/vpt/blog-backend/pkg/config"
	"github.com/vpt/blog-backend/internal/service/imagecdn"
	"github.com/vpt/blog-backend/pkg/storage"
)

type fakeObjectStore struct {
	data map[string][]byte
}

func (f *fakeObjectStore) GetImageObject(_ context.Context, objectName string) ([]byte, error) {
	data, ok := f.data[objectName]
	if !ok {
		return nil, assert.AnError
	}
	return data, nil
}

func testJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}))
	return buf.Bytes()
}

func TestService_ServeObject_TransformSetsCacheControl(t *testing.T) {
	store := &fakeObjectStore{data: map[string][]byte{"a.jpg": testJPEG(t, 120, 80)}}
	cfg := appconfig.ImageConfig{ResponseCacheMaxAge: 123, DefaultQuality: 75, MaxWidth: 3840}
	svc := imagecdn.NewService(store, cfg)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, svc.ServeObject(rr, req, "a.jpg", 32, 75, true))

	assert.Contains(t, rr.Header().Get("Cache-Control"), "max-age=123")
	assert.Equal(t, "image/webp", rr.Header().Get("Content-Type"))
	assert.NotEmpty(t, rr.Body.Bytes())
}

func TestService_ServeObject_PassthroughWithoutTransform(t *testing.T) {
	raw := testJPEG(t, 40, 30)
	store := &fakeObjectStore{data: map[string][]byte{"a.jpg": raw}}
	cfg := appconfig.ImageConfig{ResponseCacheMaxAge: 60}
	svc := imagecdn.NewService(store, cfg)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	require.NoError(t, svc.ServeObject(rr, req, "a.jpg", 0, 0, false))

	assert.Equal(t, raw, rr.Body.Bytes())
	assert.Contains(t, rr.Header().Get("Cache-Control"), "max-age=60")
}

func TestService_ServeObject_MapsOversizedSourceToErrSourceTooLarge(t *testing.T) {
	svc := imagecdn.NewService(oversizedObjectStore{}, appconfig.ImageConfig{ResponseCacheMaxAge: 60})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	err := svc.ServeObject(rr, req, "a.jpg", 0, 0, false)

	require.Error(t, err)
	assert.ErrorIs(t, err, imagecdn.ErrSourceTooLarge)
}

type oversizedObjectStore struct{}

func (oversizedObjectStore) GetImageObject(_ context.Context, _ string) ([]byte, error) {
	return nil, storage.ErrObjectTooLarge
}
