package imagefile_test

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
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

func TestValidate_RejectsTruncatedPNG(t *testing.T) {
	_, err := imagefile.Validate("cat.png", truncatedPNG(t), 10*1024*1024)

	require.ErrorIs(t, err, imagefile.ErrInvalidImage)
}

func TestValidate_RejectsDeclaredDimensionsAbovePixelLimit(t *testing.T) {
	_, err := imagefile.Validate("bomb.png", pngWithDeclaredDimensions(t, 5000, 5000), 10*1024*1024)

	require.ErrorIs(t, err, imagefile.ErrImageTooLarge)
}

func truncatedPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	require.NoError(t, png.Encode(&buf, img))
	data := buf.Bytes()
	require.Greater(t, len(data), 8)
	return data[:len(data)-8]
}

func pngWithDeclaredDimensions(t *testing.T, width, height int) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
	ihdr := make([]byte, 13)
	binary.BigEndian.PutUint32(ihdr[0:4], uint32(width))
	binary.BigEndian.PutUint32(ihdr[4:8], uint32(height))
	ihdr[8] = 8
	ihdr[9] = 2
	writePNGChunk(&buf, "IHDR", ihdr)
	writePNGChunk(&buf, "IEND", nil)
	return buf.Bytes()
}

func writePNGChunk(buf *bytes.Buffer, chunkType string, data []byte) {
	_ = binary.Write(buf, binary.BigEndian, uint32(len(data)))
	buf.WriteString(chunkType)
	buf.Write(data)
	crc := crc32.ChecksumIEEE(append([]byte(chunkType), data...))
	_ = binary.Write(buf, binary.BigEndian, crc)
}

func TestValidate_AcceptsRealGIF(t *testing.T) {
	data := testGIF(t)

	result, err := imagefile.Validate("motion.gif", data, 10*1024*1024)

	require.NoError(t, err)
	assert.Equal(t, "image/gif", result.ContentType)
	assert.Equal(t, ".gif", result.Ext)
	assert.Equal(t, data, result.Data)
}

func TestValidate_RejectsFakeGIFHeader(t *testing.T) {
	fake := append([]byte("GIF89a"), bytes.Repeat([]byte{0}, 32)...)

	_, err := imagefile.Validate("motion.gif", fake, 10*1024*1024)

	require.ErrorIs(t, err, imagefile.ErrInvalidImage)
}

func testGIF(t *testing.T) []byte {
	t.Helper()
	img := image.NewPaletted(image.Rect(0, 0, 1, 1), color.Palette{color.White})
	var buf bytes.Buffer
	require.NoError(t, gif.Encode(&buf, img, nil))
	return buf.Bytes()
}
