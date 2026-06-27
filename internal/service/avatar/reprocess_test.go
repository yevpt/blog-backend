package avatar_test

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	avatarservice "github.com/vpt/blog-backend/internal/service/avatar"
)

func TestIsCompliantAvatarData_AcceptsSmallJPEG(t *testing.T) {
	data := testJPEG(t, 80, 80, 80)

	ok, err := avatarservice.IsCompliantAvatarData(data)

	require.NoError(t, err)
	assert.True(t, ok)
}

func TestIsCompliantAvatarData_RejectsOversizedDimensions(t *testing.T) {
	data := testJPEG(t, 200, 120, 80)

	ok, err := avatarservice.IsCompliantAvatarData(data)

	require.NoError(t, err)
	assert.False(t, ok)
}

func TestIsCompliantAvatarData_RejectsPNG(t *testing.T) {
	data := testPNGBytes(t, 80, 80)

	ok, err := avatarservice.IsCompliantAvatarData(data)

	require.NoError(t, err)
	assert.False(t, ok)
}

func TestService_LoadStoredAvatar_ReadsObjectDirectly(t *testing.T) {
	payload := testPNGBytes(t, 120, 120)
	store := &fakeObjectStore{
		exists:   true,
		getBody:  payload,
		useGet:   true,
	}
	svc := avatarservice.NewService(store, avatarservice.Options{Timeout: 2 * time.Second})

	data, err := svc.LoadStoredAvatar(context.Background(), "avatar/user/old.png")

	require.NoError(t, err)
	assert.Equal(t, payload, data)
	assert.Equal(t, "avatar/user/old.png", store.objectName)
	assert.True(t, store.getCalled)
}

func TestService_ReprocessData_CompressesToJPEG(t *testing.T) {
	store := &fakeObjectStore{}
	svc := avatarservice.NewService(store, avatarservice.Options{Timeout: 2 * time.Second})

	result, err := svc.ReprocessData(context.Background(), testPNGBytes(t, 240, 200))

	require.NoError(t, err)
	assert.Contains(t, result.ObjectKey, "avatar/user/")
	assert.Contains(t, result.ObjectKey, ".jpg")
	assert.True(t, store.putCalled)
	assert.Equal(t, "image/jpeg", store.contentTyp)
	assert.LessOrEqual(t, len(store.content), 20*1024)
}

func TestService_ReprocessStoredAvatar_OverwritesTargetKey(t *testing.T) {
	store := &fakeObjectStore{}
	svc := avatarservice.NewService(store, avatarservice.Options{Timeout: 2 * time.Second})

	target := "avatar/user/legacy.png"
	result, err := svc.ReprocessStoredAvatar(context.Background(), testPNGBytes(t, 240, 200), target)

	require.NoError(t, err)
	assert.Equal(t, "avatar/user/legacy.jpg", result.ObjectKey)
	assert.True(t, store.putCalled)
	assert.Equal(t, "avatar/user/legacy.jpg", store.objectName)
	assert.LessOrEqual(t, len(store.content), 20*1024)
}

func TestService_ReprocessStoredAvatar_ConvertsGIFToJPEG(t *testing.T) {
	store := &fakeObjectStore{}
	svc := avatarservice.NewService(store, avatarservice.Options{Timeout: 2 * time.Second})

	target := "avatar/user/animated.gif"
	result, err := svc.ReprocessStoredAvatar(context.Background(), testGIF(t), target)

	require.NoError(t, err)
	assert.Equal(t, "avatar/user/animated.jpg", result.ObjectKey)
	assert.True(t, store.putCalled)
	assert.Equal(t, "image/jpeg", store.contentTyp)
	assert.LessOrEqual(t, len(store.content), 20*1024)
}

func TestService_ReprocessDataForNormalize_AlwaysUploads(t *testing.T) {
	store := &fakeObjectStore{exists: true}
	svc := avatarservice.NewService(store, avatarservice.Options{Timeout: 2 * time.Second})

	_, err := svc.ReprocessData(context.Background(), testPNGBytes(t, 240, 200))
	require.NoError(t, err)
	assert.False(t, store.putCalled, "普通重处理应复用已存在对象")

	store.putCalled = false
	result, err := svc.ReprocessDataForNormalize(context.Background(), testPNGBytes(t, 240, 200))
	require.NoError(t, err)
	assert.True(t, store.putCalled, "归一化应始终上传")
	assert.Contains(t, result.ObjectKey, "avatar/user/")
	assert.LessOrEqual(t, len(store.content), 20*1024)
}

func testPNGBytes(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 100, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func testJPEG(t *testing.T, width, height, quality int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 100, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}))
	return buf.Bytes()
}
