package imageutil_test

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/pkg/imageutil"
)

func TestProcess_CompressesToWebPWithinBounds(t *testing.T) {
	input := noisyPNG(t, 300, 220)

	result, err := imageutil.Process(bytes.NewReader(input), imageutil.Options{
		MaxWidth:       120,
		MaxHeight:      120,
		MaxBytes:       10 * 1024,
		WebPQuality:    95,
		MinWebPQuality: 50,
	})

	require.NoError(t, err)
	assert.LessOrEqual(t, result.Width, 120)
	assert.LessOrEqual(t, result.Height, 120)
	assert.LessOrEqual(t, len(result.Bytes), 10*1024)
	assert.Equal(t, imageutil.FormatWebP, result.Format)
	assert.Equal(t, "image/webp", result.ContentType)
	assert.Equal(t, ".webp", result.Ext)
	assert.Len(t, result.MD5, 32)
}

func TestProcess_DefaultsToWebP(t *testing.T) {
	input := noisyPNG(t, 40, 40)

	result, err := imageutil.Process(bytes.NewReader(input), imageutil.Options{})

	require.NoError(t, err)
	assert.Equal(t, imageutil.FormatWebP, result.Format)
	assert.Equal(t, "image/webp", result.ContentType)
	assert.Equal(t, ".webp", result.Ext)
}

func TestProcess_RejectsNonImage(t *testing.T) {
	_, err := imageutil.Process(bytes.NewReader([]byte("not image")), imageutil.Options{
		MaxWidth:  120,
		MaxHeight: 120,
	})

	assert.ErrorIs(t, err, imageutil.ErrInvalidImage)
}

func TestProcess_OutputsPNGWhenRequested(t *testing.T) {
	input := noisyPNG(t, 80, 60)

	result, err := imageutil.Process(bytes.NewReader(input), imageutil.Options{
		MaxWidth:  120,
		MaxHeight: 120,
		Format:    imageutil.FormatPNG,
	})

	require.NoError(t, err)
	assert.Equal(t, imageutil.FormatPNG, result.Format)
	assert.Equal(t, "image/png", result.ContentType)
	assert.Equal(t, ".png", result.Ext)
	assert.Equal(t, 80, result.Width)
	assert.Equal(t, 60, result.Height)
}

func TestProcess_DecodesWebP(t *testing.T) {
	input := smallWebP(t)

	result, err := imageutil.Process(bytes.NewReader(input), imageutil.Options{
		Format: imageutil.FormatWebP,
	})

	require.NoError(t, err)
	assert.Equal(t, imageutil.FormatWebP, result.Format)
	assert.Equal(t, "image/webp", result.ContentType)
	assert.Equal(t, ".webp", result.Ext)
	assert.NotEmpty(t, result.Bytes)
}

func TestProcess_CompressesDecodedWebPWithinBounds(t *testing.T) {
	input := noisyPNG(t, 300, 220)

	result, err := imageutil.Process(bytes.NewReader(input), imageutil.Options{
		MaxWidth:       120,
		MaxHeight:      120,
		MaxBytes:       10 * 1024,
		Format:         imageutil.FormatWebP,
		WebPQuality:    95,
		MinWebPQuality: 50,
	})

	require.NoError(t, err)
	assert.LessOrEqual(t, result.Width, 120)
	assert.LessOrEqual(t, result.Height, 120)
	assert.LessOrEqual(t, len(result.Bytes), 10*1024)
	assert.Equal(t, imageutil.FormatWebP, result.Format)
	assert.Equal(t, "image/webp", result.ContentType)
	assert.Equal(t, ".webp", result.Ext)
}

func noisyPNG(t *testing.T, width, height int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((x*17 + y*31) % 256),
				G: uint8((x*29 + y*11) % 256),
				B: uint8((x*7 + y*19) % 256),
				A: 255,
			})
		}
	}

	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func smallWebP(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	require.NoError(t, err)
	return data
}
