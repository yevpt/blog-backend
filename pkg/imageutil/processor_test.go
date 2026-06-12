package imageutil_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vpt/blog-backend/pkg/imageutil"
)

func TestProcess_CompressesToJPEGWithinBounds(t *testing.T) {
	input := noisyPNG(t, 300, 220)

	result, err := imageutil.Process(bytes.NewReader(input), imageutil.Options{
		MaxWidth:       120,
		MaxHeight:      120,
		MaxBytes:       10 * 1024,
		Format:         imageutil.FormatJPEG,
		JPEGQuality:    85,
		MinJPEGQuality: 35,
	})

	require.NoError(t, err)
	assert.LessOrEqual(t, result.Width, 120)
	assert.LessOrEqual(t, result.Height, 120)
	assert.LessOrEqual(t, len(result.Bytes), 10*1024)
	assert.Equal(t, imageutil.FormatJPEG, result.Format)
	assert.Equal(t, "image/jpeg", result.ContentType)
	assert.Equal(t, ".jpg", result.Ext)
	assert.Len(t, result.MD5, 32)
}

func TestProcess_RejectsNonImage(t *testing.T) {
	_, err := imageutil.Process(bytes.NewReader([]byte("not image")), imageutil.Options{
		MaxWidth:  120,
		MaxHeight: 120,
		Format:    imageutil.FormatJPEG,
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
