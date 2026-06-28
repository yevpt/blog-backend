package imagecdn_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	imagecdnhandler "github.com/vpt/blog-backend/internal/handler/imagecdn"
	imagecdnservice "github.com/vpt/blog-backend/internal/service/imagecdn"
	"github.com/vpt/blog-backend/internal/middleware"
	appconfig "github.com/vpt/blog-backend/pkg/config"
)

type fakeStore struct {
	data map[string][]byte
}

func (f *fakeStore) GetImageObject(_ context.Context, objectName string) ([]byte, error) {
	data, ok := f.data[objectName]
	if !ok {
		return nil, errors.New("missing")
	}
	return data, nil
}

func sampleJPEG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}))
	return buf.Bytes()
}

func TestImageCDNHandler_RequiresOriginAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &appconfig.Config{
		Garage: appconfig.GarageConfig{Bucket: "blog"},
		Image:  appconfig.ImageConfig{OriginAuthSecret: "secret", ResponseCacheMaxAge: 60},
	}
	store := &fakeStore{data: map[string][]byte{"articles/a.jpg": sampleJPEG(t, 40, 30)}}
	svc := imagecdnservice.NewService(store, cfg.Image)
	h := imagecdnhandler.NewHandler(svc, cfg)

	r := gin.New()
	r.GET("/blog/*filepath", middleware.OriginAuth("secret"), h.Serve)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/blog/articles/a.jpg?w=32", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestImageCDNHandler_ServesTransformedImage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &appconfig.Config{
		Garage: appconfig.GarageConfig{Bucket: "blog"},
		Image:  appconfig.ImageConfig{ResponseCacheMaxAge: 99, DefaultQuality: 75, MaxWidth: 3840},
	}
	store := &fakeStore{data: map[string][]byte{"articles/a.jpg": sampleJPEG(t, 80, 60)}}
	svc := imagecdnservice.NewService(store, cfg.Image)
	h := imagecdnhandler.NewHandler(svc, cfg)

	r := gin.New()
	r.GET("/blog/*filepath", h.Serve)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/blog/articles/a.jpg?w=32&q=75", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Cache-Control"), "max-age=99")
}
