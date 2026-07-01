package upload_test

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vpt/blog-backend/pkg/imagefile"
	uploadservice "github.com/vpt/blog-backend/internal/service/upload"
)

type fakeObjectStore struct {
	exists         bool
	existsErr      error
	putErr         error
	urlErr         error
	puts           []putRecord
	objectURLByKey map[string]string
}

type putRecord struct {
	key         string
	data        []byte
	contentType string
}

type publishingGateStub struct {
	allowed bool
}

func (s publishingGateStub) PublishingAllowed(context.Context, uint64) (bool, error) {
	return s.allowed, nil
}

func (s *fakeObjectStore) ObjectURL(ctx context.Context, objectName string) (string, error) {
	if s.urlErr != nil {
		return "", s.urlErr
	}
	if s.objectURLByKey == nil {
		return "https://cdn.example.com/blog/" + objectName, nil
	}
	if url, ok := s.objectURLByKey[objectName]; ok {
		return url, nil
	}
	return "https://cdn.example.com/blog/" + objectName, nil
}

func (s *fakeObjectStore) MoveObject(ctx context.Context, sourceName string, targetName string) error {
	return nil
}

func (s *fakeObjectStore) CopyObject(ctx context.Context, sourceName string, targetName string) error {
	return nil
}

func (s *fakeObjectStore) ObjectKey(value string) (string, error) {
	return value, nil
}

func (s *fakeObjectStore) ObjectExists(ctx context.Context, objectName string) (bool, error) {
	return s.exists, s.existsErr
}

func (s *fakeObjectStore) PutObject(ctx context.Context, objectName string, data []byte, contentType string) error {
	if s.putErr != nil {
		return s.putErr
	}
	s.puts = append(s.puts, putRecord{
		key:         objectName,
		data:        append([]byte(nil), data...),
		contentType: contentType,
	})
	return nil
}

func (s *fakeObjectStore) DeleteObject(ctx context.Context, objectName string) error {
	return nil
}

func TestService_UploadTempImage_StoresUserScopedKey(t *testing.T) {
	store := &fakeObjectStore{objectURLByKey: map[string]string{}}
	svc := uploadservice.NewService(store)
	file := smallPNG(t)
	sum := md5.Sum(file)
	expectedMD5 := hex.EncodeToString(sum[:])

	resp, err := svc.UploadTempImage(context.Background(), uploadservice.TempImageInput{
		UserID: 7,
		Dir:    "images",
		Name:   "cat.png",
		Data:   file,
	})

	require.NoError(t, err)
	assert.Equal(t, "temp/articles/7/images/"+expectedMD5+".png", resp.Key)
	assert.Equal(t, "https://cdn.example.com/blog/"+resp.Key, resp.URL)
	require.Len(t, store.puts, 1)
	assert.Equal(t, resp.Key, store.puts[0].key)
	assert.Equal(t, "image/png", store.puts[0].contentType)
	assert.Equal(t, file, store.puts[0].data)
}

func TestService_UploadTempImage_MobileCoversDir(t *testing.T) {
	store := &fakeObjectStore{objectURLByKey: map[string]string{}}
	svc := uploadservice.NewService(store)
	file := smallPNG(t)
	sum := md5.Sum(file)
	expectedMD5 := hex.EncodeToString(sum[:])

	resp, err := svc.UploadTempImage(context.Background(), uploadservice.TempImageInput{
		UserID: 7,
		Dir:    "mobile-covers",
		Name:   "mobile.png",
		Data:   file,
	})

	require.NoError(t, err)
	assert.Equal(t, "temp/articles/7/mobile-covers/"+expectedMD5+".png", resp.Key)
}

func TestService_UploadTempImage_RejectsInvalidDir(t *testing.T) {
	svc := uploadservice.NewService(&fakeObjectStore{})

	_, err := svc.UploadTempImage(context.Background(), uploadservice.TempImageInput{
		UserID: 7,
		Dir:    "../images",
		Name:   "cat.png",
		Data:   smallPNG(t),
	})

	require.ErrorIs(t, err, uploadservice.ErrUploadDirInvalid)
}

func TestServiceUploadCommentTempImageRejectsSanctionedUser(t *testing.T) {
	store := &fakeObjectStore{}
	svc := uploadservice.NewService(store, publishingGateStub{allowed: false})

	_, err := svc.UploadTempImage(context.Background(), uploadservice.TempImageInput{
		UserID: 7, Scene: "comment", Dir: "comments", Name: "cat.png", Data: smallPNG(t),
	})

	require.ErrorIs(t, err, uploadservice.ErrUploadForbidden)
	assert.Empty(t, store.puts)
}

func TestService_UploadTempImage_SkipsUploadWhenObjectExists(t *testing.T) {
	file := smallPNG(t)
	sum := md5.Sum(file)
	expectedMD5 := hex.EncodeToString(sum[:])
	key := "temp/articles/7/covers/" + expectedMD5 + ".png"
	store := &fakeObjectStore{
		exists: true,
		objectURLByKey: map[string]string{
			key: "https://cdn.example.com/blog/" + key,
		},
	}
	svc := uploadservice.NewService(store)

	resp, err := svc.UploadTempImage(context.Background(), uploadservice.TempImageInput{
		UserID: 7,
		Dir:    "covers",
		Name:   "cover.png",
		Data:   file,
	})

	require.NoError(t, err)
	assert.NotEmpty(t, resp.URL)
	assert.Len(t, store.puts, 0)
}

func TestService_UploadTempImage_CommentSceneCompressesAndStoresCommentKey(t *testing.T) {
	store := &fakeObjectStore{}
	svc := uploadservice.NewService(store)

	resp, err := svc.UploadTempImage(context.Background(), uploadservice.TempImageInput{
		UserID: 7,
		Scene:  "comment",
		Dir:    "images",
		Name:   "cat.png",
		Data:   noisyPNG(t, 420, 420),
	})

	require.NoError(t, err)
	assert.Regexp(t, `^temp/comments/7/images/[a-f0-9]{32}\.webp$`, resp.Key)
	assert.Equal(t, "https://cdn.example.com/blog/"+resp.Key, resp.URL)
	require.Len(t, store.puts, 1)
	assert.Equal(t, resp.Key, store.puts[0].key)
	assert.Equal(t, "image/webp", store.puts[0].contentType)
	assert.LessOrEqual(t, len(store.puts[0].data), 500*1024)
}

func TestService_UploadTempImage_ArticleSceneCompressesOversizedToWebP(t *testing.T) {
	store := &fakeObjectStore{}
	svc := uploadservice.NewService(store)

	resp, err := svc.UploadTempImage(context.Background(), uploadservice.TempImageInput{
		UserID: 7,
		Dir:    "images",
		Name:   "big.png",
		Data:   noisyPNG(t, 1100, 1100),
	})

	require.NoError(t, err)
	assert.Regexp(t, `^temp/articles/7/images/[a-f0-9]{32}\.webp$`, resp.Key)
	require.Len(t, store.puts, 1)
	assert.Equal(t, "image/webp", store.puts[0].contentType)
	assert.LessOrEqual(t, len(store.puts[0].data), 3*1024*1024)
}

func TestService_UploadTempImage_CommentSceneRejectsTooManyPixels(t *testing.T) {
	svc := uploadservice.NewService(&fakeObjectStore{})

	_, err := svc.UploadTempImage(context.Background(), uploadservice.TempImageInput{
		UserID: 7,
		Scene:  "comment",
		Dir:    "images",
		Name:   "huge.png",
		Data:   pngWithDeclaredDimensions(t, 4032, 3024),
	})

	require.ErrorIs(t, err, uploadservice.ErrUploadImageTooManyPixels)
	assert.Equal(t, imagefile.ErrImageTooManyPixels.Error(), err.Error())
}

func TestService_UploadTempImage_ReturnsUnavailableWhenStoreFails(t *testing.T) {
	store := &fakeObjectStore{putErr: errors.New("s3 down")}
	svc := uploadservice.NewService(store)

	_, err := svc.UploadTempImage(context.Background(), uploadservice.TempImageInput{
		UserID: 7,
		Dir:    "images",
		Name:   "cat.png",
		Data:   smallPNG(t),
	})

	require.ErrorIs(t, err, uploadservice.ErrUploadUnavailable)
}

func smallPNG(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func noisyPNG(t *testing.T, width int, height int) []byte {
	t.Helper()

	var buf bytes.Buffer
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
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func pngWithDeclaredDimensions(t *testing.T, width, height int) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.Write([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A})
	ihdr := make([]byte, 13)
	putUint32BE(ihdr[0:4], uint32(width))
	putUint32BE(ihdr[4:8], uint32(height))
	ihdr[8] = 8
	ihdr[9] = 2
	writePNGChunk(&buf, "IHDR", ihdr)
	writePNGChunk(&buf, "IEND", nil)
	return buf.Bytes()
}

func putUint32BE(dst []byte, value uint32) {
	dst[0] = byte(value >> 24)
	dst[1] = byte(value >> 16)
	dst[2] = byte(value >> 8)
	dst[3] = byte(value)
}

func writePNGChunk(buf *bytes.Buffer, chunkType string, data []byte) {
	_ = binary.Write(buf, binary.BigEndian, uint32(len(data)))
	buf.WriteString(chunkType)
	buf.Write(data)
	crc := crc32.ChecksumIEEE(append([]byte(chunkType), data...))
	_ = binary.Write(buf, binary.BigEndian, crc)
}
