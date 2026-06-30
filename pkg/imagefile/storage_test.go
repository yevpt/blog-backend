package imagefile_test

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"testing"

	"github.com/chai2010/webp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/pkg/imagefile"
)

func TestPrepareForStorage_PassthroughWhenWithinLimit(t *testing.T) {
	input := smallWebP(t)

	result, err := imagefile.PrepareForStorage("photo.webp", input, imagefile.PrepareOptions{
		MaxStoredBytes: 500 * 1024,
	})

	require.NoError(t, err)
	assert.Equal(t, input, result.Data)
	assert.Equal(t, "image/webp", result.ContentType)
	assert.Equal(t, ".webp", result.Ext)
}

func TestPrepareForStorage_PassthroughWhenWithinDimensions(t *testing.T) {
	input := smallWebPImage(t, 80, 80)

	result, err := imagefile.PrepareForStorage("avatar.webp", input, imagefile.PrepareOptions{
		MaxStoredBytes: 20 * 1024,
		MaxWidth:       120,
		MaxHeight:      120,
	})

	require.NoError(t, err)
	assert.Equal(t, input, result.Data)
}

func TestPrepareForStorage_RecompressesWhenDimensionsExceeded(t *testing.T) {
	input := smallWebPImage(t, 200, 200)

	result, err := imagefile.PrepareForStorage("avatar.webp", input, imagefile.PrepareOptions{
		MaxStoredBytes: 20 * 1024,
		MaxWidth:       120,
		MaxHeight:      120,
	})

	require.NoError(t, err)
	assert.NotEqual(t, input, result.Data)
	assert.Equal(t, "image/webp", result.ContentType)
	assert.LessOrEqual(t, len(result.Data), 20*1024)
}

func TestPrepareForStorage_CompressesOversizedToWebP(t *testing.T) {
	input := largeNoisyPNG(t, 420, 420)

	result, err := imagefile.PrepareForStorage("large.png", input, imagefile.PrepareOptions{
		MaxStoredBytes: 500 * 1024,
	})

	require.NoError(t, err)
	assert.LessOrEqual(t, len(result.Data), 500*1024)
	assert.Equal(t, "image/webp", result.ContentType)
	assert.Equal(t, ".webp", result.Ext)
	assert.NotEqual(t, input, result.Data)
}

func smallWebPImage(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 100, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, webp.Encode(&buf, img, &webp.Options{Lossless: false, Quality: 80}))
	require.LessOrEqual(t, buf.Len(), 20*1024)
	return buf.Bytes()
}

func largeNoisyPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	rng := rand.New(rand.NewSource(1))
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8(rng.Intn(256)),
				G: uint8(rng.Intn(256)),
				B: uint8(rng.Intn(256)),
				A: 255,
			})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	require.Greater(t, buf.Len(), 500*1024)
	return buf.Bytes()
}

func smallWebP(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	require.NoError(t, err)
	return data
}
