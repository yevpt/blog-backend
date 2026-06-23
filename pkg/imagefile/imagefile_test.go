package imagefile_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/pkg/imagefile"
)

func TestValidate_PreservesPNGAndComputesMD5(t *testing.T) {
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	require.NoError(t, png.Encode(&buf, img))

	result, err := imagefile.Validate("cat.png", buf.Bytes(), 10*1024*1024)

	require.NoError(t, err)
	assert.Equal(t, ".png", result.Ext)
	assert.Equal(t, "image/png", result.ContentType)
	assert.Len(t, result.MD5, 32)
	assert.Equal(t, buf.Bytes(), result.Data)
}

func TestValidate_RejectsInvalidImage(t *testing.T) {
	_, err := imagefile.Validate("cat.png", []byte("not-image"), 10*1024*1024)

	require.ErrorIs(t, err, imagefile.ErrInvalidImage)
}

func TestValidate_RejectsTooLarge(t *testing.T) {
	_, err := imagefile.Validate("cat.png", []byte("abc"), 2)

	require.ErrorIs(t, err, imagefile.ErrImageTooLarge)
}
