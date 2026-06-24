package upload

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/vpt/blog-backend/internal/dto"
	"github.com/vpt/blog-backend/pkg/imagefile"
	"github.com/vpt/blog-backend/pkg/imageutil"
	"github.com/vpt/blog-backend/pkg/storage"
)

const MaxTempImageBytes = 10 * 1024 * 1024

const (
	MaxCommentTempImageBytes       = 1024 * 1024
	MaxCommentTempGIFBytes         = 300 * 1024
	MaxCommentTempImageStoredBytes = 500 * 1024
)

var (
	ErrUploadInvalid         = errors.New("上传图片无效")
	ErrUploadTooLarge        = errors.New("图片不能超过 10MB")
	ErrUploadCommentTooLarge = errors.New("图片不能超过 1MB")
	ErrUploadCommentGIFLarge = errors.New("GIF 图片过大，暂不支持压缩该格式，请上传 300KB 以内的 GIF。")
	ErrUploadCompressedLarge = errors.New("图片过大，压缩后仍超过 500KB，请换一张更小的图片")
	ErrUploadDirInvalid      = errors.New("上传目录无效")
	ErrUploadSceneInvalid    = errors.New("上传场景无效")
	ErrUploadUnavailable     = errors.New("对象存储不可用")
)

const (
	sceneArticle = "article"
	sceneComment = "comment"
)

type Service interface {
	UploadTempImage(ctx context.Context, input TempImageInput) (*dto.TempUploadResp, error)
}

type TempImageInput struct {
	UserID uint
	Scene  string
	Dir    string
	Name   string
	Data   []byte
}

type service struct {
	store storage.ObjectStore
}

func NewService(store storage.ObjectStore) Service {
	return &service{store: store}
}

func (s *service) UploadTempImage(ctx context.Context, input TempImageInput) (*dto.TempUploadResp, error) {
	if s == nil || s.store == nil {
		return nil, ErrUploadUnavailable
	}

	scene, err := normalizeTempScene(input.Scene)
	if err != nil {
		return nil, err
	}

	var key string
	var data []byte
	var contentType string
	switch scene {
	case sceneArticle:
		key, data, contentType, err = s.prepareArticleTempImage(input)
	case sceneComment:
		key, data, contentType, err = s.prepareCommentTempImage(input)
	default:
		err = ErrUploadSceneInvalid
	}
	if err != nil {
		return nil, err
	}
	exists, err := s.store.ObjectExists(ctx, key)
	if err != nil {
		return nil, ErrUploadUnavailable
	}
	if !exists {
		if err := s.store.PutObject(ctx, key, data, contentType); err != nil {
			return nil, ErrUploadUnavailable
		}
	}
	url, err := s.store.ObjectURL(ctx, key)
	if err != nil {
		return nil, ErrUploadUnavailable
	}

	return &dto.TempUploadResp{
		Key: key,
		URL: url,
	}, nil
}

func (s *service) prepareArticleTempImage(input TempImageInput) (string, []byte, string, error) {
	dir, err := normalizeArticleTempDir(input.Dir)
	if err != nil {
		return "", nil, "", err
	}

	result, err := imagefile.Validate(input.Name, input.Data, MaxTempImageBytes)
	if err != nil {
		return "", nil, "", mapArticleValidateErr(err)
	}
	key := fmt.Sprintf("temp/articles/%d/%s/%s%s", input.UserID, dir, result.MD5, result.Ext)
	return key, result.Data, result.ContentType, nil
}

func (s *service) prepareCommentTempImage(input TempImageInput) (string, []byte, string, error) {
	dir, err := normalizeCommentTempDir(input.Dir)
	if err != nil {
		return "", nil, "", err
	}

	result, err := imagefile.Validate(input.Name, input.Data, MaxCommentTempImageBytes)
	if err != nil {
		return "", nil, "", mapCommentValidateErr(err)
	}
	if result.ContentType == "image/gif" {
		if len(result.Data) > MaxCommentTempGIFBytes {
			return "", nil, "", ErrUploadCommentGIFLarge
		}
		key := fmt.Sprintf("temp/comments/%d/%s/%s%s", input.UserID, dir, result.MD5, result.Ext)
		return key, result.Data, result.ContentType, nil
	}

	processed, err := processCommentImage(input.Data, result.Ext)
	if err != nil {
		return "", nil, "", err
	}
	key := fmt.Sprintf("temp/comments/%d/%s/%s%s", input.UserID, dir, processed.md5, processed.ext)
	return key, processed.data, processed.contentType, nil
}

type processedCommentImage struct {
	data        []byte
	contentType string
	ext         string
	md5         string
}

func processCommentImage(data []byte, ext string) (processedCommentImage, error) {
	opts := imageutil.Options{Format: imageFormatFromExt(ext)}
	if len(data) > MaxCommentTempImageStoredBytes {
		opts.Format = imageutil.FormatJPEG
		opts.MaxBytes = MaxCommentTempImageStoredBytes
		opts.JPEGQuality = 85
		opts.MinJPEGQuality = 35
	}
	result, err := imageutil.Process(bytes.NewReader(data), opts)
	if err != nil {
		return processedCommentImage{}, mapImageProcessErr(err)
	}
	if len(result.Bytes) > MaxCommentTempImageStoredBytes {
		result, err = imageutil.Process(bytes.NewReader(data), imageutil.Options{
			Format:         imageutil.FormatJPEG,
			MaxBytes:       MaxCommentTempImageStoredBytes,
			JPEGQuality:    85,
			MinJPEGQuality: 35,
		})
		if err != nil {
			return processedCommentImage{}, mapImageProcessErr(err)
		}
	}
	sum := md5.Sum(result.Bytes)
	return processedCommentImage{
		data:        result.Bytes,
		contentType: result.ContentType,
		ext:         result.Ext,
		md5:         hex.EncodeToString(sum[:]),
	}, nil
}

func imageFormatFromExt(ext string) imageutil.Format {
	if strings.EqualFold(ext, ".png") {
		return imageutil.FormatPNG
	}
	return imageutil.FormatJPEG
}

func normalizeTempScene(value string) (string, error) {
	scene := strings.TrimSpace(value)
	if scene == "" {
		return sceneArticle, nil
	}
	switch scene {
	case sceneArticle, sceneComment:
		return scene, nil
	default:
		return "", ErrUploadSceneInvalid
	}
}

func MaxTempUploadReadBytes(scene string) int {
	if strings.TrimSpace(scene) == sceneComment {
		return MaxCommentTempImageBytes
	}
	return MaxTempImageBytes
}

func normalizeTempDir(value string) (string, error) {
	return normalizeArticleTempDir(value)
}

func normalizeArticleTempDir(value string) (string, error) {
	dir := strings.TrimSpace(value)
	switch dir {
	case "images", "covers":
		return dir, nil
	default:
		return "", ErrUploadDirInvalid
	}
}

func normalizeCommentTempDir(value string) (string, error) {
	dir := strings.TrimSpace(value)
	if dir == "" {
		dir = "images"
	}
	switch dir {
	case "images":
		return dir, nil
	default:
		return "", ErrUploadDirInvalid
	}
}

func mapArticleValidateErr(err error) error {
	switch {
	case errors.Is(err, imagefile.ErrImageTooLarge):
		return ErrUploadTooLarge
	case errors.Is(err, imagefile.ErrInvalidImage):
		return ErrUploadInvalid
	default:
		return ErrUploadInvalid
	}
}

func mapCommentValidateErr(err error) error {
	switch {
	case errors.Is(err, imagefile.ErrImageTooLarge):
		return ErrUploadCommentTooLarge
	case errors.Is(err, imagefile.ErrInvalidImage):
		return ErrUploadInvalid
	default:
		return ErrUploadInvalid
	}
}

func mapImageProcessErr(err error) error {
	switch {
	case errors.Is(err, imageutil.ErrInvalidImage), errors.Is(err, imageutil.ErrUnsupportedFormat):
		return ErrUploadInvalid
	case errors.Is(err, imageutil.ErrImageTooLarge):
		return ErrUploadCompressedLarge
	default:
		return err
	}
}
