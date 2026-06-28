package moderationmedia_test

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"image/png"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/internal/service/moderationmedia"
	"github.com/vpt/blog-backend/pkg/config"
	"github.com/vpt/blog-backend/pkg/storage"
)

type mediaStore struct {
	objects map[string][]byte
	puts    []string
	deletes []string
}

func (s *mediaStore) ObjectURL(context.Context, string) (string, error) { return "", nil }
func (s *mediaStore) ObjectExists(_ context.Context, key string) (bool, error) {
	_, ok := s.objects[key]
	return ok, nil
}
func (s *mediaStore) PutObject(_ context.Context, key string, data []byte, _ string) error {
	s.objects[key] = append([]byte(nil), data...)
	s.puts = append(s.puts, key)
	return nil
}
func (s *mediaStore) GetObject(_ context.Context, key string) ([]byte, error) {
	data, ok := s.objects[key]
	if !ok {
		return nil, errors.New("missing")
	}
	return append([]byte(nil), data...), nil
}
func (s *mediaStore) DeleteObject(_ context.Context, key string) error {
	delete(s.objects, key)
	s.deletes = append(s.deletes, key)
	return nil
}
func (s *mediaStore) MoveObject(context.Context, string, string) error { return nil }
func (s *mediaStore) CopyObject(context.Context, string, string) error { return nil }
func (s *mediaStore) ObjectKey(value string) (string, error) {
	if value == "https://external.example/a.png" {
		return "", storage.ErrExternalObjectURL
	}
	return value, nil
}

type mediaRegistry struct {
	approved  bool
	useErr    error
	upsertErr error
	usedAt    time.Time
	pending   []moderationmedia.PendingImage
}

func (r *mediaRegistry) UseApprovedImage(_ context.Context, _ moderationmedia.Fingerprint, usedAt time.Time) (bool, error) {
	r.usedAt = usedAt
	return r.approved, r.useErr
}
func (r *mediaRegistry) UpsertPendingImage(_ context.Context, image moderationmedia.PendingImage) error {
	r.pending = append(r.pending, image)
	return r.upsertErr
}

func TestPrepareBuildsFingerprintAndStaticPreview(t *testing.T) {
	original := testPNG(t, 120, 80)
	store := &mediaStore{objects: map[string][]byte{"temp/comments/7/images/a.png": original}}
	registry := &mediaRegistry{}
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	svc := moderationmedia.NewService(store, registry, mediaConfig(), func() time.Time { return now })

	got, err := svc.Prepare(context.Background(), 7, []string{"temp/comments/7/images/a.png"})

	require.NoError(t, err)
	require.Len(t, got.Images, 1)
	assert.Len(t, got.Images[0].SHA256, 64)
	assert.Len(t, got.Images[0].MD5, 32)
	assert.Equal(t, uint64(len(original)), got.Images[0].Size)
	assert.False(t, got.Images[0].Approved)
	assert.NotEmpty(t, got.Images[0].PreviewObjectKey)
	require.Len(t, registry.pending, 1)
	preview := store.objects[got.Images[0].PreviewObjectKey]
	previewConfig, _, err := image.DecodeConfig(bytes.NewReader(preview))
	require.NoError(t, err)
	assert.LessOrEqual(t, previewConfig.Width, 48)
	assert.LessOrEqual(t, previewConfig.Height, 48)
}

func TestPrepareReusesApprovedImageAndTouchesAccessTime(t *testing.T) {
	store := &mediaStore{objects: map[string][]byte{"moments/7/a.png": testPNG(t, 20, 20)}}
	registry := &mediaRegistry{approved: true}
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	svc := moderationmedia.NewService(store, registry, mediaConfig(), func() time.Time { return now })

	got, err := svc.Prepare(context.Background(), 7, []string{"moments/7/a.png"})

	require.NoError(t, err)
	require.Len(t, got.Images, 1)
	assert.True(t, got.Images[0].Approved)
	assert.Empty(t, got.Images[0].PreviewObjectKey)
	assert.Equal(t, now, registry.usedAt)
	assert.Empty(t, store.puts)
}

func TestPrepareDeletesCreatedPreviewWhenRegistryFails(t *testing.T) {
	store := &mediaStore{objects: map[string][]byte{"temp/comments/7/images/a.png": testPNG(t, 20, 20)}}
	registry := &mediaRegistry{upsertErr: errors.New("db down")}
	svc := moderationmedia.NewService(store, registry, mediaConfig(), time.Now)

	_, err := svc.Prepare(context.Background(), 7, []string{"temp/comments/7/images/a.png"})

	require.Error(t, err)
	require.Len(t, store.puts, 1)
	assert.Equal(t, store.puts, store.deletes)
}

func TestPrepareUsesGIFPlaceholderWithoutCreatingPreview(t *testing.T) {
	store := &mediaStore{objects: map[string][]byte{"temp/comments/7/images/a.gif": testGIF(t)}}
	registry := &mediaRegistry{}
	svc := moderationmedia.NewService(store, registry, mediaConfig(), time.Now)

	got, err := svc.Prepare(context.Background(), 7, []string{"temp/comments/7/images/a.gif"})

	require.NoError(t, err)
	require.Len(t, got.Images, 1)
	assert.True(t, got.Images[0].IsGIF)
	assert.Equal(t, "system/moderation/gif-review.jpg", got.Images[0].PreviewObjectKey)
	assert.Empty(t, store.puts)
}

func TestPreparePreservesDuplicateOrderButRegistersFingerprintOnce(t *testing.T) {
	key := "temp/comments/7/images/a.png"
	store := &mediaStore{objects: map[string][]byte{key: testPNG(t, 20, 20)}}
	registry := &mediaRegistry{}
	svc := moderationmedia.NewService(store, registry, mediaConfig(), time.Now)

	got, err := svc.Prepare(context.Background(), 7, []string{key, key})

	require.NoError(t, err)
	require.Len(t, got.Images, 2)
	assert.Equal(t, got.Images[0], got.Images[1])
	assert.Len(t, registry.pending, 1)
}

func TestPrepareRejectsExternalAndOversizedPixelImages(t *testing.T) {
	store := &mediaStore{objects: map[string][]byte{"moments/7/a.png": testPNG(t, 20, 20)}}
	registry := &mediaRegistry{}
	cfg := mediaConfig()
	cfg.MaxPixels = 100
	svc := moderationmedia.NewService(store, registry, cfg, time.Now)

	_, externalErr := svc.Prepare(context.Background(), 7, []string{"https://external.example/a.png"})
	_, pixelsErr := svc.Prepare(context.Background(), 7, []string{"moments/7/a.png"})

	assert.ErrorIs(t, externalErr, moderationmedia.ErrInvalidImage)
	assert.ErrorIs(t, pixelsErr, moderationmedia.ErrInvalidImage)
}

func mediaConfig() config.ModerationImageConfig {
	return config.ModerationImageConfig{
		MaxStoredBytes: 1024 * 1024, MaxPixels: 1_000_000, PreviewMaxEdge: 48,
		StaticPlaceholderKey: "system/moderation/image-review.jpg",
		GIFPlaceholderKey:    "system/moderation/gif-review.jpg",
	}
}

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func testGIF(t *testing.T) []byte {
	t.Helper()
	img := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.Black, color.White})
	var buf bytes.Buffer
	require.NoError(t, gif.Encode(&buf, img, nil))
	return buf.Bytes()
}
